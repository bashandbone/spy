// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package loader

import (
	"bufio"
	"errors"
	"io"
	"sync"

	"github.com/knitli/spy/internal/source"
)

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
	mu               sync.Mutex
	lines            []source.Line // index 0 == first resident line
	startLine        int64         // 1-based line number of lines[0]
	totalLines       int64         // last seen total; -1 while streaming
	residentBytes    int64
	maxResidentBytes int64
	windowSize       int

	src        source.Source
	windowed   bool
	warnedSeek bool

	// warningCh is best-effort sink for streaming warnings. After the
	// producer closes it, post-streaming warnings still accumulate in
	// `warnings` for callers to inspect via [LineBuffer.Warnings].
	warningCh chan<- error
	warnings  []error
}

// NewLineBuffer constructs an empty buffer. `src` is retained so
// windowed-mode re-seeks can call Reopen(). When `maxResidentBytes` is
// zero, the buffer never flips to windowed mode.
func NewLineBuffer(maxResidentBytes int64, windowSize int, src source.Source) *LineBuffer {
	if windowSize <= 0 {
		windowSize = defaultWindowSize
	}
	return &LineBuffer{
		startLine:        1,
		totalLines:       -1,
		maxResidentBytes: maxResidentBytes,
		windowSize:       windowSize,
		src:              src,
	}
}

// Append records new lines emitted by the streamer. When the buffer
// exceeds `maxResidentBytes`, it flips into windowed mode and trims
// older lines to keep memory under the cap.
func (b *LineBuffer) Append(in []source.Line) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, l := range in {
		b.lines = append(b.lines, l)
		b.residentBytes += int64(len(l.Raw))
	}
	if b.maxResidentBytes > 0 && b.residentBytes > b.maxResidentBytes {
		b.windowed = true
		b.evictLocked()
	}
}

// MarkComplete tells the buffer streaming has finished and pins the
// final total. Renderers typically poll Total() after EOF.
func (b *LineBuffer) MarkComplete(total int64) {
	b.mu.Lock()
	b.totalLines = total
	b.mu.Unlock()
}

// Total returns the total line count once known, or -1 while streaming.
func (b *LineBuffer) Total() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
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

// Slice returns lines numbered in [start, end) using 1-based numbering.
// Returns whatever is currently resident if the range is partially
// outside; in windowed mode, falls back to a re-seek via [source.Source]
// .Reopen() when a range lies entirely outside the window.
//
// `start` and `end` are 0-based line indices in the source. We accept
// either convention as long as callers are consistent — the contract in
// internal-apis.md uses 0-based indices for Slice.
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
		out := make([]source.Line, to-from)
		copy(out, b.lines[from:to])
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
		out := make([]source.Line, toIdx-fromIdx)
		copy(out, b.lines[fromIdx:toIdx])
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

	out, err := readWindow(src, wantStart, wantEnd)
	if err != nil {
		if errors.Is(err, source.ErrNotSeekable) {
			b.mu.Lock()
			if !warned {
				b.warnedSeek = true
				b.warnings = append(b.warnings, ErrStdinNonSeekable)
				if warningCh != nil {
					sendWarning(warningCh, ErrStdinNonSeekable)
				}
			}
			b.mu.Unlock()
		}
		return nil
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

// sendWarning attempts to push a warning onto the (potentially closed)
// channel without panicking. The recover path is the only way to avoid
// a panic on send-to-closed without coordinating producer/consumer
// shutdown — see the LineBuffer doc comment for the broader design.
func sendWarning(ch chan<- error, err error) {
	defer func() { _ = recover() }()
	select {
	case ch <- err:
	default:
	}
}

// SetWarningSink installs a side-channel for windowed-mode warnings.
// The loader's [Open] hooks the buffer to Stream.Errs so the UI can
// surface "scroll-back disabled past resident window" advisories.
func (b *LineBuffer) SetWarningSink(ch chan<- error) {
	b.mu.Lock()
	b.warningCh = ch
	b.mu.Unlock()
}

// evictLocked drops oldest lines until residentBytes fits the cap or
// only the configured window remains. Caller must hold b.mu.
func (b *LineBuffer) evictLocked() {
	target := b.maxResidentBytes
	if target <= 0 {
		return
	}
	for b.residentBytes > target && len(b.lines) > b.windowSize {
		dropped := b.lines[0]
		b.residentBytes -= int64(len(dropped.Raw))
		b.lines = b.lines[1:]
		b.startLine++
	}
}

// readWindow re-opens the source and skips ahead to `start`, copying
// lines until `end`. Used by windowed-mode Slice() when the requested
// range falls outside the resident window.
func readWindow(src source.Source, start, end int64) ([]source.Line, error) {
	rs, err := src.Reopen()
	if err != nil {
		return nil, err
	}
	if c, ok := rs.(io.Closer); ok {
		defer c.Close()
	}
	scanner := bufio.NewScanner(rs)
	scanner.Buffer(make([]byte, scannerBufferBytes), scannerMaxBytes)

	var out []source.Line
	var lineNo int64 = 1
	for scanner.Scan() {
		if lineNo >= end {
			break
		}
		if lineNo >= start {
			raw := scanner.Bytes()
			cp := make([]byte, len(raw))
			copy(cp, raw)
			out = append(out, source.Line{Number: lineNo, Raw: string(cp)})
		}
		lineNo++
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	return out, nil
}
