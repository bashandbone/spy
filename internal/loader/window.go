// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package loader

import (
	"bufio"
	"errors"
	"io"
	"sync"
	"unsafe"

	"github.com/knitli/spy/internal/source"
)

// storedLine is the compact internal representation kept in b.lines.
// Storing only Number and Raw (24 bytes) instead of the full
// source.Line (72 bytes, including two nil-slice headers for Tokens and
// Wrapped) cuts per-line struct overhead by two thirds. For a 200 MiB
// file with 256-byte lines this saves ~38 MiB of backing-array memory.
//
// Tokens and Wrapped are stored in the LineBuffer's side-channel maps
// (b.tokens, b.wrapped) keyed by line number; they are nil for the vast
// majority of lines (only rendered/highlighted lines populate them).
// Slice() merges the maps back into source.Line on read.
type storedLine struct {
	Number int64
	Raw    string
}

// storedLineOverheadBytes is the per-line fixed memory cost of storedLine
// itself (struct layout: int64 + string header). Added to residentBytes
// accounting so windowing triggers on actual bytes-in-use, not just
// len(Raw). Computed via unsafe.Sizeof so it stays correct across
// architectures if the struct layout ever changes.
var storedLineOverheadBytes = int64(unsafe.Sizeof(storedLine{}))

// LineBuffer is the resident hot region the renderer slices into. Below
// `maxResidentBytes` it holds every line; above it, the buffer flips to
// windowed mode and re-seeks via `src.Reopen()` for line ranges that
// fall outside the current window.
//
// LineBuffer is goroutine-safe: the loader producer Append()s while the
// UI consumer Slice()s. Mutations are guarded by a single sync.Mutex —
// renderer access is bursty (one frame at a time), so contention is
// negligible compared to scan throughput.
type LineBuffer struct {
	mu         sync.Mutex
	lines      []storedLine // index 0 == first resident line; compact repr.
	// tokens and wrapped are side-channel maps keyed by 1-based line
	// number. They are nil (zero-allocation) when no line has been
	// highlighted or word-wrapped, which is the common case during
	// initial load. Storing them here (rather than in storedLine) is
	// what lets storedLine stay at 24 bytes.
	tokens  map[int64][]source.Token
	wrapped map[int64][]string
	startLine  int64         // 1-based line number of lines[0]
	totalLines int64         // last seen total; -1 while streaming
	// streamStarted flips true on the first [LineBuffer.Append] or
	// [LineBuffer.MarkComplete] call. While false [LineBuffer.Total]
	// returns -1 (the "unknown / streaming hasn't begun" sentinel) so
	// callers can distinguish "loader hasn't fed me anything yet" from
	// "loader confirmed zero lines" — important for search and the
	// status-bar footer (Copilot review acceptance M5).
	streamStarted    bool
	residentBytes    int64
	maxResidentBytes int64
	windowSize       int
	// maxLineBytes is the per-line truncation cap propagated from
	// [Config]. Used by the windowed-mode re-seek path so streaming
	// and reseek apply the same cap (Copilot review PR#7 #20).
	maxLineBytes int64

	src        source.Source
	windowed   bool
	warnedSeek bool

	// warningCh is best-effort sink for streaming warnings. After the
	// producer closes it, post-streaming warnings still accumulate in
	// `warnings` for callers to inspect via [LineBuffer.Warnings].
	//
	// warningSinkClosed coordinates the lifecycle (acceptance review
	// LOW-4): the loader's producer goroutine calls
	// [LineBuffer.CloseWarningSink] BEFORE it closes the underlying
	// channel, flipping this flag under b.mu. Subsequent
	// [sendWarning] calls observe the flag (also under b.mu) and skip
	// the send entirely, so the previously-needed `defer recover()`
	// in sendWarning is no longer the only thing keeping a
	// send-on-closed-channel panic at bay. The defer recover() stays
	// as a belt-and-suspenders for the unlikely path where a future
	// caller invokes sendWarning without holding the mutex, but the
	// happy path no longer relies on it.
	warningCh         chan<- error
	warningSinkClosed bool
	warnings          []error
}

// NewLineBuffer constructs an empty buffer. `src` is retained so
// windowed-mode re-seeks can call Reopen(). When `maxResidentBytes` is
// zero, the buffer never flips to windowed mode.
//
// maxLineBytes <= 0 means "use defaultMaxLineBytes" — callers that
// don't care can pass 0; [Open] supplies the same value it gives the
// streaming readChunk so both paths apply the same cap.
func NewLineBuffer(maxResidentBytes int64, windowSize int, src source.Source) *LineBuffer {
	return newLineBuffer(maxResidentBytes, windowSize, defaultMaxLineBytes, src)
}

// newLineBuffer is the full-arity constructor used by [Open] when the
// caller needs to plumb a custom MaxLineBytes through to windowed-mode
// re-seeks.
func newLineBuffer(maxResidentBytes int64, windowSize int, maxLineBytes int64, src source.Source) *LineBuffer {
	if windowSize <= 0 {
		windowSize = defaultWindowSize
	}
	if maxLineBytes <= 0 {
		maxLineBytes = defaultMaxLineBytes
	}
	return &LineBuffer{
		startLine:        1,
		totalLines:       -1,
		maxResidentBytes: maxResidentBytes,
		windowSize:       windowSize,
		maxLineBytes:     maxLineBytes,
		src:              src,
	}
}

// Append records new lines emitted by the streamer. When the buffer
// exceeds `maxResidentBytes`, it flips into windowed mode and trims
// older lines to keep memory under the cap.
//
// The first Append (or [MarkComplete]) flips `streamStarted` so
// [Total] stops returning the -1 "unknown" sentinel — even an empty
// `in` slice counts as "the loader is producing", because EOF is
// reported via MarkComplete and the consumer needs Total() to switch
// to "0 confirmed" rather than "-1 still unknown".
//
// residentBytes now accounts for both the string content (len(l.Raw))
// AND the per-struct overhead (storedLineOverheadBytes bytes) so
// the windowing threshold reflects actual bytes-in-use rather than just
// raw content bytes. Previously only len(l.Raw) was tracked, causing the
// threshold to fire too late and the RSS to overshoot the budget.
func (b *LineBuffer) Append(in []source.Line) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.streamStarted = true
	for _, l := range in {
		b.lines = append(b.lines, storedLine{Number: l.Number, Raw: l.Raw})
		b.residentBytes += int64(len(l.Raw)) + storedLineOverheadBytes
		// Preserve any Tokens/Wrapped that the caller may have pre-set
		// (rare in practice; loader chunks always arrive with nil fields).
		if l.Tokens != nil {
			if b.tokens == nil {
				b.tokens = make(map[int64][]source.Token)
			}
			b.tokens[l.Number] = l.Tokens
		}
		if l.Wrapped != nil {
			if b.wrapped == nil {
				b.wrapped = make(map[int64][]string)
			}
			b.wrapped[l.Number] = l.Wrapped
		}
	}
	if b.maxResidentBytes > 0 && b.residentBytes > b.maxResidentBytes {
		b.windowed = true
		b.evictLocked()
	}
}

// ClearWrapCaches resets the word-wrap cache for every resident line.
// The UI's word-wrap toggle (Ctrl-W) and width-change handlers call this
// so any future per-line wrap caches the renderer stashes are invalidated
// when wrap mode or viewport width changes — without that contract, a
// toggle could surface stale visual rows on the next paint.
//
// Wrap data is stored in the b.wrapped side-channel map (not in the
// storedLine struct), so clearing it is simply zeroing the map.
//
// As of Phase 8 nothing in the renderer populates Line.Wrapped (the
// text/code renderers re-wrap from scratch on each frame), so calling
// ClearWrapCaches against the current build is effectively a no-op.
// The method exists ahead of that population work so the UI's toggle
// path is contract-correct from day one and a future renderer change
// to memoize wrapped rows doesn't have to retrofit invalidation
// across every caller (Copilot review PR#13 #2 — the documentation no
// longer claims the cache is wired up).
//
// Holds the same lock as [SetTokens] so concurrent loader appends and
// renderer slices stay consistent.
func (b *LineBuffer) ClearWrapCaches() {
	b.mu.Lock()
	b.wrapped = nil
	b.mu.Unlock()
}

// SetTokens propagates highlighter Tokens from `lines` into the
// matching resident lines (matched by [source.Line.Number]) under the
// buffer's mutex. Lines whose number falls outside the resident hot
// region (typically because windowed-mode eviction has already pruned
// them) are silently skipped — the renderer can re-highlight on
// demand for evicted ranges.
//
// Callers (currently `internal/ui`) highlight a freshly-arrived chunk
// then immediately invoke SetTokens so the buffer's stored copies pick
// up the tokens. Without this, the buffer's stored lines retain
// `Tokens == nil` and the renderer re-lexes on every frame — burning
// the highlighter's byte cap on each repaint (Copilot review PR#8 #1
// + #3).
//
// Tokens are kept in the b.tokens side-channel map (not in the
// compact storedLine struct) so line storage stays at 24 bytes/line.
func (b *LineBuffer) SetTokens(lines []source.Line) {
	if len(lines) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lines) == 0 {
		return
	}
	residentEnd := b.startLine + int64(len(b.lines))
	for _, in := range lines {
		if in.Number < b.startLine || in.Number >= residentEnd {
			continue
		}
		if in.Tokens == nil {
			if b.tokens != nil {
				delete(b.tokens, in.Number)
			}
		} else {
			if b.tokens == nil {
				b.tokens = make(map[int64][]source.Token)
			}
			b.tokens[in.Number] = in.Tokens
		}
	}
}

// MarkComplete tells the buffer streaming has finished and pins the
// final total. Renderers typically poll Total() after EOF.
//
// MarkComplete also flips `streamStarted` so a zero-line source still
// transitions [Total] from -1 (unknown) to 0 (confirmed empty) — a
// genuinely empty file must not look like "loader hasn't run yet".
func (b *LineBuffer) MarkComplete(total int64) {
	b.mu.Lock()
	b.streamStarted = true
	b.totalLines = total
	b.mu.Unlock()
}

// Total returns the line count of the full source. Three values are
// possible:
//
//   - `-1` while streaming hasn't started yet (no [Append], no
//     [MarkComplete]). Callers MUST treat this as "unknown total" — in
//     particular, a search scan that sees -1 should wait for the
//     streamer rather than declaring "no lines, nothing to scan"
//     (Copilot review acceptance M5).
//   - the running line count once at least one [Append] has landed
//     and before [MarkComplete].
//   - the pinned final total after [MarkComplete].
//
// A source that was confirmed-empty (loader read EOF on the first
// chunk) returns 0, distinguishable from -1.
func (b *LineBuffer) Total() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.streamStarted {
		return -1
	}
	if b.totalLines > 0 {
		return b.totalLines
	}
	return b.startLine - 1 + int64(len(b.lines))
}

// Windowed reports whether the buffer has flipped into windowed mode.
// Useful for tests; production code should not branch on this.
func (b *LineBuffer) Windowed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.windowed
}

// ResidentStartLine returns the 1-based source line number of the first
// line currently held in the resident hot region. While the buffer is
// not windowed this is always 1; once windowed, the value tracks the
// eviction frontier so callers (e.g. internal/ui's footer) can map a
// viewport row back to the absolute line number that the renderer
// printed in its gutter (Copilot review PR#7 #15).
func (b *LineBuffer) ResidentStartLine() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.startLine
}

// Slice returns the lines in the 0-based half-open range [start, end).
// The returned [source.Line] values retain their 1-based Number fields.
// Returns whatever is currently resident if the range is partially
// outside; in windowed mode, falls back to a re-seek via
// [source.Source.Reopen] when a range lies entirely outside the window.
//
// `start` and `end` are 0-based line indices in the source, matching the
// Slice contract in internal-apis.md.
//
// Internally, resident lines are stored as compact [storedLine] values
// (Number + Raw only). Slice reconstructs full [source.Line] values by
// merging in any Tokens/Wrapped from the side-channel maps.
func (b *LineBuffer) Slice(start, end int64) []source.Line {
	if start < 0 {
		start = 0
	}
	if end <= start {
		return nil
	}
	b.mu.Lock()

	// Convert 0-based slice indices to 1-based line numbers used by
	// source.Line.Number / b.startLine.
	wantStartNum := start + 1
	wantEndNum := end + 1

	residentStart := b.startLine
	residentEnd := residentStart + int64(len(b.lines))

	// Fast path: range entirely resident.
	if wantStartNum >= residentStart && wantEndNum <= residentEnd {
		from := int(wantStartNum - residentStart)
		to := int(wantEndNum - residentStart)
		if to > len(b.lines) {
			to = len(b.lines)
		}
		out := b.buildLines(from, to)
		b.mu.Unlock()
		return out
	}

	// Partial overlap with resident range.
	if wantStartNum < residentEnd && wantEndNum > residentStart {
		from := wantStartNum
		if from < residentStart {
			from = residentStart
		}
		to := wantEndNum
		if to > residentEnd {
			to = residentEnd
		}
		fromIdx := int(from - residentStart)
		toIdx := int(to - residentStart)
		out := b.buildLines(fromIdx, toIdx)
		b.mu.Unlock()
		return out
	}

	// Outside the window — windowed mode only.
	if !b.windowed {
		b.mu.Unlock()
		return nil
	}
	src := b.src
	wantStart := wantStartNum
	wantEnd := wantEndNum
	warned := b.warnedSeek
	warningCh := b.warningCh
	b.mu.Unlock()

	out, err := readWindow(src, wantStart, wantEnd, b.maxLineBytes)
	if err != nil {
		if errors.Is(err, source.ErrNotSeekable) {
			b.mu.Lock()
			if !warned {
				b.warnedSeek = true
				b.warnings = append(b.warnings, ErrStdinNonSeekable)
				// Only send if the sink is still live. The
				// warningSinkClosed flag is set by
				// [LineBuffer.CloseWarningSink] (called by the
				// producer goroutine before it closes the channel),
				// so checking under b.mu is race-free with the
				// producer's lifecycle (LOW-4).
				if warningCh != nil && !b.warningSinkClosed {
					sendWarning(warningCh, ErrStdinNonSeekable)
				}
			}
			b.mu.Unlock()
		}
		return nil
	}
	return out
}

// buildLines constructs a []source.Line from b.lines[from:to], merging
// any Tokens and Wrapped from the side-channel maps. Caller must hold b.mu.
func (b *LineBuffer) buildLines(from, to int) []source.Line {
	out := make([]source.Line, to-from)
	// Fast path: no tokens or wrapped data has been set yet (the common
	// case during initial load). Skip both map lookups entirely.
	if b.tokens == nil && b.wrapped == nil {
		for i, sl := range b.lines[from:to] {
			out[i] = source.Line{Number: sl.Number, Raw: sl.Raw}
		}
		return out
	}
	for i, sl := range b.lines[from:to] {
		out[i] = source.Line{
			Number:  sl.Number,
			Raw:     sl.Raw,
			Tokens:  b.tokens[sl.Number],  // nil when not in map
			Wrapped: b.wrapped[sl.Number], // nil when not in map
		}
	}
	return out
}

// Warnings returns a snapshot of all warnings the buffer has accumulated
// across its lifetime — including any that the producer-side errs
// channel could not deliver because it had already been closed. Tests
// and the UI's status-bar advisory pipeline (T023c) consume this when
// the live `errs` channel is no longer drainable.
func (b *LineBuffer) Warnings() []error {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]error, len(b.warnings))
	copy(out, b.warnings)
	return out
}

// sendWarning attempts to push a warning onto the channel without
// blocking. Callers MUST hold the LineBuffer mutex AND verify
// warningSinkClosed is false before invoking — that combination is
// what makes the send race-free with the producer's close.
//
// The defer recover() is retained as belt-and-suspenders for any
// future call site that fails to honour the contract; the only
// expected panic is "send on closed channel" and recovering from it
// is strictly safer than crashing the loader from a UI-driven
// Slice() call. Re-panic on anything else so unrelated runtime
// panics still surface.
func sendWarning(ch chan<- error, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		// Go panics with a runtime.plainError for "send on closed
		// channel"; its String() produces exactly that text. Compare
		// via fmt to avoid depending on the unexported runtime type.
		if e, ok := r.(error); ok && e.Error() == "send on closed channel" {
			return
		}
		// Some Go runtimes panic with a *runtime.PanicError or a
		// plain string — fall back to a string match.
		if s, ok := r.(string); ok && s == "send on closed channel" {
			return
		}
		panic(r) // unrelated panic — propagate
	}()
	select {
	case ch <- err:
	default:
	}
}

// SetWarningSink installs a side-channel for windowed-mode warnings.
// The loader's [Open] hooks the buffer to Stream.Errs so the UI can
// surface "scroll-back disabled past resident window" advisories.
//
// SetWarningSink also resets the warningSinkClosed flag so a fresh
// sink (rare in practice — the loader installs once at Open) starts
// from a live state.
func (b *LineBuffer) SetWarningSink(ch chan<- error) {
	b.mu.Lock()
	b.warningCh = ch
	b.warningSinkClosed = false
	b.mu.Unlock()
}

// CloseWarningSink marks the warning sink as no longer live. The
// producer goroutine calls this BEFORE closing the underlying
// channel so subsequent [LineBuffer.Slice] calls observing the flag
// (under b.mu) skip their send entirely and never reach the
// channel — eliminating the previously-fragile "rely on
// recover() to swallow send-on-closed" pattern (LOW-4).
//
// Idempotent: safe to call from the producer's defer chain even if
// the close has already been signalled (e.g. Open's error-path
// returns before launching the streaming goroutine).
func (b *LineBuffer) CloseWarningSink() {
	b.mu.Lock()
	b.warningSinkClosed = true
	b.mu.Unlock()
}

// evictLocked drops oldest lines until residentBytes fits the cap or
// only the configured window remains. Caller must hold b.mu.
//
// Each evicted storedLine is zeroed before the slice head advances so
// the Raw string pointer is released; the GC can then collect the
// string backing data even though the storedLine struct slot still
// occupies the backing array. Side-channel tokens/wrapped map entries
// for evicted line numbers are also removed.
func (b *LineBuffer) evictLocked() {
	target := b.maxResidentBytes
	if target <= 0 {
		return
	}
	for b.residentBytes > target && len(b.lines) > b.windowSize {
		dropped := b.lines[0]
		b.residentBytes -= int64(len(dropped.Raw)) + storedLineOverheadBytes
		// Zero out the slot so the Raw string can be GCed even
		// though the backing array element still exists.
		b.lines[0] = storedLine{}
		b.lines = b.lines[1:]
		if b.tokens != nil {
			delete(b.tokens, dropped.Number)
		}
		if b.wrapped != nil {
			delete(b.wrapped, dropped.Number)
		}
		b.startLine++
	}
}

// readWindow re-opens the source and skips ahead to `start`, copying
// lines until `end`. Used by windowed-mode Slice() when the requested
// range falls outside the resident window. The `maxLineBytes` cap
// matches the streaming path so both produce identical truncations.
func readWindow(src source.Source, start, end, maxLineBytes int64) ([]source.Line, error) {
	if maxLineBytes <= 0 {
		maxLineBytes = defaultMaxLineBytes
	}
	rs, err := src.Reopen()
	if err != nil {
		return nil, err
	}
	if c, ok := rs.(io.Closer); ok {
		defer c.Close()
	}
	reader := bufio.NewReaderSize(rs, readerBufferBytes)
	var out []source.Line
	var lineNo int64 = 1
	var lineBuf []byte
	for {
		if lineNo >= end {
			break
		}
		var raw []byte
		var err error
		raw, _, err = readLine(reader, maxLineBytes, lineBuf)
		lineBuf = raw
		if errors.Is(err, io.EOF) {
			if len(raw) > 0 && lineNo >= start {
				out = append(out, source.Line{Number: lineNo, Raw: string(raw)})
			}
			break
		}
		if err != nil {
			return out, err
		}
		if lineNo >= start {
			out = append(out, source.Line{Number: lineNo, Raw: string(raw)})
		}
		lineNo++
	}
	return out, nil
}
