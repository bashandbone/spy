// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/search"
	"github.com/knitli/spy/internal/source"
)

// Init sets up the initial command pipeline: subscribe to BOTH the
// loader's Updates channel (via [waitForChunk]) AND its Errs channel
// (via [waitForStreamErr]) so progressive content arrivals AND
// non-fatal warnings (line-truncated, stdin-non-seekable) surface
// in the UI as they happen.
//
// Pre-acceptance review the Errs channel was unconsumed in production
// — `loader.ErrLineTruncated` and `loader.ErrStdinNonSeekable`
// warnings reached the channel and were silently dropped, so users
// were never told content was clipped or that scroll-back was
// disabled (review C7).
func (m Model) Init() tea.Cmd {
	if m.stream == nil {
		return nil
	}
	return tea.Batch(waitForChunk(m.stream), waitForStreamErr(m.stream))
}

// Update routes Bubble Tea messages to the matching handler. The
// foundational set: window-size, key events (quit + scroll via the
// keymap), and chunk arrivals from the loader.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onResize(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	case chunkLoadedMsg:
		// Drop stale chunks from a stream that ActionReload swapped
		// out (Copilot review PR#8 #2).
		if msg.stream != nil && msg.stream != m.stream {
			return m, nil
		}
		return m.onChunk(msg)
	case streamDoneMsg:
		if msg.stream != nil && msg.stream != m.stream {
			// Stale done from an old stream; the new stream is still
			// alive — ignore.
			return m, nil
		}
		m.streaming = false
		if m.status == render.StatusStreaming {
			m.status = render.StatusIdle
		}
		return m, metaUpdatedCmd(m.stream)
	case streamErrMsg:
		return m.onStreamErr(msg)
	case reloadMsg:
		return m.onReload()
	case reloadResultMsg:
		return m.onReloadResult(msg)
	case openResultMsg:
		return m.onOpenResult(msg)
	case searchResultMsg:
		return m.onSearchResult(msg)
	case metaUpdatedMsg:
		// Cache the finalized line count on the model so the footer
		// reads it on subsequent paints without taking the buffer
		// mutex (Copilot review PR#13 round-2 #4 + #5 — TotalLines
		// has an observable consumer). No re-render is issued here:
		// the model state that drives the footer (m.streaming, the
		// buffer's pinned Total) has already been mutated by
		// [onChunk] / [streamDoneMsg] before metaUpdatedMsg arrives,
		// and Bubble Tea re-invokes [View] after every Update — so
		// the footer flips from "…" to the final count automatically
		// without doubling the EOF render work (Copilot review PR#13
		// round-1 #3).
		if msg.TotalLines > 0 {
			m.totalLines = msg.TotalLines
		}
		return m, nil
	}
	return m, nil
}

// onOpenResult swaps in the new source after a `:open <path>` command.
// On failure the prior session is retained and the error surfaces via
// statusAdvisory. On success the new stream becomes the active one and
// the renderer is rebuilt with the new source's kind / language.
func (m Model) onOpenResult(msg openResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusAdvisory = fmt.Sprintf("open: %v", msg.err)
		return m, nil
	}
	// Cancel any in-flight async search against the old buffer; its
	// result would be dropped via the gen guard anyway, but
	// cancelling lets the goroutine exit promptly rather than
	// scanning a buffer that's about to be GC'd.
	if m.searchCancel != nil {
		m.searchCancel()
		m.searchCancel = nil
	}
	// Bumping searchGen here defensively forces any in-flight result
	// to be dropped on arrival even if the cancel raced the scan
	// goroutine's send.
	m.searchGen++
	// Swap in the new source/stream/renderer. Clear the advisory and
	// last-error state so messages from the prior session ("search
	// wrapped", a previous reload error, etc.) don't bleed into the
	// new file's footer (Copilot review PR#9 round-2 #4).
	m.source = msg.src
	m.stream = msg.stream
	m.cancel = msg.cancel
	m.search = search.State{}
	m.statusAdvisory = ""
	m.lastError = nil
	m.viewport.GotoTop()
	kind := source.KindUnknown
	lang := ""
	if m.source != nil {
		kind = m.source.Kind()
		lang = m.source.Metadata().Language
	}
	if m.highlighter != nil && lang != "" {
		m.highlighter.SetLang(lang)
	}
	deps := render.Dependencies{
		Theme:        m.theme,
		Capabilities: m.caps,
		Highlighter:  m.highlighter,
		LineNumbers:  m.cfg != nil && m.cfg.LineNumbers,
		WordWrap:     m.cfg != nil && m.cfg.WordWrap,
		Language:     lang,
		Source:       m.source,
	}
	m.renderer = render.ForKind(kind, deps)
	m.page = 1       // reset PDF page cursor on source swap
	m.totalLines = 0 // clear the cached finalized count; new source restarts streaming
	m.streaming = true
	m.status = render.StatusStreaming
	if m.stream != nil && m.stream.First.EOF {
		m.streaming = false
		m.status = render.StatusIdle
	}
	if m.highlighter != nil && m.stream != nil && kind == source.KindCode {
		highlightLines(m.highlighter, lang, m.stream.First.Lines)
		if m.stream.Buffer != nil {
			m.stream.Buffer.SetTokens(m.stream.First.Lines)
		}
	}
	m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	if m.streaming {
		// Re-subscribe to BOTH chunks AND errs against the new
		// stream — without re-subscribing to errs, the swapped-in
		// source's truncation / stdin warnings would never reach
		// the UI (acceptance review C7).
		return m, tea.Batch(waitForChunk(m.stream), waitForStreamErr(m.stream))
	}
	// Even on EOF stream we should subscribe once so a buffered
	// warning from `loader.Open` (e.g. truncation in the first
	// chunk read synchronously) still surfaces.
	return m, waitForStreamErr(m.stream)
}

// onResize reflows the viewport to the new terminal size. The status
// bar (US6) reserves one row at the bottom; for now the viewport
// claims the full height minus a placeholder footer.
//
// SC-008 / quickstart.md step 12 require that the line previously at
// viewport row 0 stays at row 0 across a resize, so we preserve the
// existing YOffset rather than constructing a fresh viewport.Model
// (which would reset scroll state to top) — Copilot review PR#7 #22.
func (m Model) onResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	footerRows := 1
	if m.height < footerRows+1 {
		footerRows = 0
	}
	m.showFooter = footerRows > 0
	prevYOffset := m.viewport.YOffset
	first := m.viewport.Width == 0 && m.viewport.Height == 0
	if first {
		m.viewport = viewport.New(m.width, m.height-footerRows)
	} else {
		m.viewport.Width = m.width
		m.viewport.Height = m.height - footerRows
	}
	m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	if !first {
		m.viewport.SetYOffset(prevYOffset)
	}
	return m, nil
}

// onKey runs the active keymap and translates the matched [Action]
// into a tea.Cmd. The prompt state machine takes precedence: when a `:`
// / `/` / `?` prompt is open, every key is captured by the prompt
// editor until Enter (submit) or Esc (cancel) closes it.
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.commandLine.Active {
		return m.onPromptKey(msg)
	}
	// Vim "gg" sequence: a previous `g` is pending. Another `g`
	// resolves to ActionGoToTop; anything else cancels and falls
	// through to the regular dispatch path so the second key isn't
	// dropped.
	if m.vimPendingG {
		m.vimPendingG = false
		if msg.String() == "g" {
			m.viewport.GotoTop()
			return m, nil
		}
		// fall-through: the second key is dispatched normally below.
	}
	if matchAction(m.keyMap, keys.ActionQuit, msg) {
		if m.cancel != nil {
			m.cancel()
		}
		if m.searchCancel != nil {
			m.searchCancel()
			m.searchCancel = nil
		}
		return m, tea.Quit
	}
	if matchAction(m.keyMap, keys.ActionReload, msg) {
		return m, func() tea.Msg { return reloadMsg{} }
	}
	if matchAction(m.keyMap, keys.ActionToggleLineNumbers, msg) {
		return m.onToggleLineNumbers()
	}
	if matchAction(m.keyMap, keys.ActionToggleWordWrap, msg) {
		return m.onToggleWordWrap()
	}
	if matchAction(m.keyMap, keys.ActionOpenFile, msg) {
		return m.onOpenFilePrompt()
	}
	if matchAction(m.keyMap, keys.ActionSearchForward, msg) {
		m.openPrompt('/')
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionSearchBackward, msg) {
		m.openPrompt('?')
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionCommandOpen, msg) {
		m.openPrompt(':')
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionNextMatch, msg) {
		m.advanceMatch(+1)
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionPrevMatch, msg) {
		m.advanceMatch(-1)
		return m, nil
	}
	// Vim `gg` first-press detection: the keymap binds `g` to
	// ActionGoToTop (alongside Home), so we need to catch the literal
	// `g` press and arm the pending flag rather than firing the action
	// immediately.
	if m.vim && msg.String() == "g" {
		m.vimPendingG = true
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionScrollUp, msg) {
		m.viewport.LineUp(1)
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionScrollDown, msg) {
		m.viewport.LineDown(1)
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionPageUp, msg) {
		m.viewport.ViewUp()
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionPageDown, msg) {
		m.viewport.ViewDown()
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionHalfPageUp, msg) {
		m.viewport.HalfViewUp()
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionHalfPageDown, msg) {
		m.viewport.HalfViewDown()
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionGoToTop, msg) {
		m.viewport.GotoTop()
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionGoToBottom, msg) {
		m.viewport.GotoBottom()
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionNextPage, msg) {
		return m.onNextPage()
	}
	if matchAction(m.keyMap, keys.ActionPrevPage, msg) {
		return m.onPrevPage()
	}
	if matchAction(m.keyMap, keys.ActionBeginningOfLine, msg) {
		m.viewport.SetXOffset(0)
		return m, nil
	}
	if matchAction(m.keyMap, keys.ActionEndOfLine, msg) {
		// Best-effort end-of-line: jump to a very large horizontal
		// offset and let the viewport clamp it to the furthest valid
		// position for the currently visible content.
		m.viewport.SetXOffset(int(^uint(0) >> 1))
		return m, nil
	}
	return m, nil
}

// openPrompt enters command-line mode with the supplied prefix. The
// foundational nav keys are suppressed until the prompt closes via
// Enter or Esc.
func (m *Model) openPrompt(prefix rune) {
	m.commandLine.Active = true
	m.commandLine.Prefix = prefix
	m.commandLine.Buffer = ""
	m.commandLine.HistoryCursor = -1
}

// onPromptKey edits the active command-line buffer. Enter submits, Esc
// cancels, Up/Down recall history, Backspace deletes a rune, and any
// runeable key gets appended to the buffer.
func (m Model) onPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.submitPrompt()
	case tea.KeyEsc, tea.KeyCtrlC:
		m.commandLine.reset()
		return m, nil
	case tea.KeyBackspace, tea.KeyCtrlH:
		if m.commandLine.Buffer != "" {
			r := []rune(m.commandLine.Buffer)
			m.commandLine.Buffer = string(r[:len(r)-1])
		}
		return m, nil
	case tea.KeyUp:
		m.recallHistory(-1)
		return m, nil
	case tea.KeyDown:
		m.recallHistory(+1)
		return m, nil
	case tea.KeyRunes:
		m.commandLine.Buffer += string(msg.Runes)
		return m, nil
	case tea.KeySpace:
		m.commandLine.Buffer += " "
		return m, nil
	}
	return m, nil
}

// recallHistory steps through the active prefix's submission history.
// `direction == -1` walks toward older entries, `+1` toward newer.
// When the cursor reaches the end (newer than the newest), history
// navigation exits, HistoryCursor is reset to -1, and the buffer is
// cleared. (Storing the pre-recall buffer to restore on exit is a
// future polish — not implemented today.)
func (m *Model) recallHistory(direction int) {
	hist := m.commandLine.historyFor(m.commandLine.Prefix)
	if len(hist) == 0 {
		return
	}
	cur := m.commandLine.HistoryCursor
	if direction < 0 {
		if cur < 0 {
			cur = len(hist) - 1
		} else if cur > 0 {
			cur--
		}
	} else {
		if cur < 0 {
			return // already at "live" buffer
		}
		cur++
		if cur >= len(hist) {
			m.commandLine.HistoryCursor = -1
			m.commandLine.Buffer = ""
			return
		}
	}
	m.commandLine.HistoryCursor = cur
	m.commandLine.Buffer = hist[cur]
}

// submitPrompt closes the prompt and dispatches by prefix.
func (m Model) submitPrompt() (tea.Model, tea.Cmd) {
	prefix := m.commandLine.Prefix
	buf := m.commandLine.Buffer
	m.commandLine.pushHistory(prefix, buf)
	m.commandLine.reset()
	switch prefix {
	case '/':
		return m.runSearch(buf, search.DirForward)
	case '?':
		return m.runSearch(buf, search.DirBackward)
	case ':':
		return m.runCommand(buf)
	}
	return m, nil
}

// runSearch compiles the matcher and dispatches an asynchronous full
// scan against the resident buffer. Pre-acceptance review (H5) the
// scan ran synchronously on the Bubble Tea event-loop goroutine —
// against a 50 MB / 1 M-line resident buffer (`MaxResidentBytes == 0`)
// the unbuffered [search.Scan] channel forced the loop to block
// match-by-match for seconds, freezing key and resize handling. The
// async refactor moves the channel drain into a tea.Cmd-spawned
// goroutine and yields a [searchResultMsg] on completion.
//
// Concurrency invariants:
//
//   - Per-search context cancellation: a new search cancels the prior
//     one (m.searchCancel) so rapid-typing never piles up goroutines.
//   - Stale-result drop: each dispatch bumps m.searchGen and tags the
//     resulting [searchResultMsg]; [Model.Update] ignores any message
//     whose gen doesn't match the live counter so a slow first scan
//     can't clobber a fresh second scan's state.
//   - Pending flag set: search.State.Pending=true is observable to the
//     renderer / footer immediately so the user sees that a scan is
//     in flight.
//
// The prompt buffer can carry vim-style prefix toggles per
// contracts/keys.md ("Search" section): `\v` forces regex, `\V`
// forces literal, `\c` forces case-insensitive, and `\C` forces
// case-sensitive. Prefixes apply in the order typed and override the
// cfg defaults; the cleaned query is stored on SearchState.Query so
// the renderer's highlight overlay matches what the user actually
// searched for.
func (m Model) runSearch(query string, dir search.Direction) (tea.Model, tea.Cmd) {
	// Cancel any in-flight scan and invalidate its result BEFORE any
	// of the early-return guards. Every submission path — empty query,
	// missing buffer, all-prefix-only after stripping, compile error,
	// or the success path — must honor the "submit cancels prior"
	// contract. Without this top placement, hitting Enter on an empty
	// `/` prompt while a prior scan is Pending leaves the prior scan
	// running and Pending stuck true (PR#22 round-3 review — the
	// initial fix moved cancel above compile but missed the empty-query
	// guard above it).
	if m.searchCancel != nil {
		m.searchCancel()
		m.searchCancel = nil
	}
	m.searchGen++
	// Clear Pending now; the success path below re-installs Pending=true
	// with the new query, while early returns leave it false so the
	// renderer / footer don't lie about a scan in flight.
	m.search.Pending = false
	if query == "" || m.stream == nil || m.stream.Buffer == nil {
		return m, nil
	}
	regex := false
	caseMode := search.CaseSmart
	if m.cfg != nil {
		regex = m.cfg.RegexDefault
		caseMode = caseModeFromConfig(m.cfg.CaseMode, caseMode)
	}
	cleaned, regex, caseMode := stripSearchPrefixes(query, regex, caseMode)
	if cleaned == "" {
		// All-prefix queries (e.g. typed `\c` then immediately Enter)
		// don't have anything to match — surface a soft advisory rather
		// than producing an empty Match list.
		m.statusAdvisory = "search: empty query after prefix toggles"
		return m, nil
	}
	matcher, err := search.Compile(cleaned, regex, caseMode)
	if err != nil {
		m.statusAdvisory = fmt.Sprintf("invalid pattern: %v", err)
		return m, nil
	}
	from := int64(1)
	if m.viewport.Height > 0 && m.renderer != nil {
		from = m.renderer.RowToLine(m.renderContext(), m.viewport.YOffset)
		if from < 1 {
			from = 1
		}
	}
	// Capture the post-bump gen for the goroutine to ship back, then
	// install a fresh searchCancel for the supersede / reload paths.
	gen := m.searchGen
	ctx, cancel := context.WithCancel(context.Background())
	m.searchCancel = cancel
	// Install the pending state immediately so the renderer / footer
	// reflect "scan in flight" before the goroutine starts producing
	// results. CurrentMatch=-1 keeps any prior selection from
	// rendering against an obsolete match list.
	m.search = search.State{
		Query:        cleaned,
		Direction:    dir,
		Regex:        regex,
		CaseMode:     caseMode,
		Matches:      nil,
		CurrentMatch: -1,
		Wrapped:      false,
		Pending:      true,
	}
	// Clear any prior "no match" / "wrapped" advisory left over from
	// the previous search so the user sees a clean slate while the new
	// scan is in flight.
	m.statusAdvisory = ""
	if m.renderer != nil {
		m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	}
	// Snapshot the buffer pointer so the goroutine reads from a
	// stable [source.LineProvider] even if a concurrent reload swaps
	// m.stream out (the swap will also bump searchGen via the next
	// runSearch / state change, so the result is dropped on arrival).
	provider := m.stream.Buffer
	cmd := func() tea.Msg {
		// Always release the context's resources when the goroutine
		// exits, regardless of whether the channel closed naturally
		// or m.searchCancel pre-empted us. The on-Model cancel func
		// is for *external* cancel-on-supersede; this defer is for
		// context resource hygiene (`go vet` lostcancel rule).
		defer cancel()
		ch := search.Scan(ctx, provider, matcher, dir, from)
		var matches []search.Match
		scanWrapAt := -1
		for hit := range ch {
			if hit.Line == search.SentinelWrapped {
				scanWrapAt = len(matches)
				continue
			}
			matches = append(matches, hit)
		}
		// Surface ctx cancellation so the handler can drop the
		// result (and any partial matches accumulated before the
		// cancel landed) without applying it.
		cancelled := ctx.Err() != nil
		return searchResultMsg{
			gen:        gen,
			query:      cleaned,
			dir:        dir,
			regex:      regex,
			caseMode:   caseMode,
			matches:    matches,
			scanWrapAt: scanWrapAt,
			cancelled:  cancelled,
		}
	}
	return m, cmd
}

// onSearchResult applies the outcome of an async scan dispatched by
// [Model.runSearch]. Stale results (gen mismatch) and cancelled scans
// are dropped without mutating model state — preserves the H5
// invariant that rapid-typing never lets an older scan overwrite a
// newer one.
func (m Model) onSearchResult(msg searchResultMsg) (tea.Model, tea.Cmd) {
	// Stale: a newer search has since started. Drop without touching
	// state so the live search continues uninterrupted.
	if msg.gen != m.searchGen {
		return m, nil
	}
	// Cancelled: the goroutine exited via ctx — partial matches are
	// not authoritative and the next-in-flight scan will produce the
	// canonical result. Clear Pending defensively in case this was
	// the only outstanding scan, and drop the cancel handle since
	// the goroutine has already exited (its context.WithCancel is
	// done, so the func is a no-op anyway, but holding a handle to
	// a finished context muddies the "is a scan in flight?" check
	// for any future teardown path) (PR#22 review round-2).
	if msg.cancelled {
		m.search.Pending = false
		m.searchCancel = nil
		return m, nil
	}
	// Defensive: if the live state's identity drifted from this
	// message's identity (gen matches but query/direction don't —
	// shouldn't happen in practice because runSearch installs both
	// atomically with the gen bump), prefer the message's identity.
	m.search = search.State{
		Query:        msg.query,
		Direction:    msg.dir,
		Regex:        msg.regex,
		CaseMode:     msg.caseMode,
		Matches:      msg.matches,
		CurrentMatch: -1,
		Wrapped:      false,
		Pending:      false,
	}
	// The scan owned m.searchCancel; once the result has landed the
	// goroutine has already exited so the cancel is moot.
	m.searchCancel = nil
	if len(msg.matches) == 0 {
		m.statusAdvisory = fmt.Sprintf("no match for %q", msg.query)
		if m.renderer != nil {
			m.viewport.SetContent(m.renderer.Render(m.renderContext()))
		}
		return m, nil
	}
	// search.Scan emits matches in *traversal* order, so index 0 is
	// the first hit the user encounters from `from`. Selecting 0
	// for both directions keeps the initial jump intuitive (Copilot
	// review PR#9 #5).
	m.search.CurrentMatch = 0
	if msg.scanWrapAt == 0 {
		m.search.Wrapped = true
		m.statusAdvisory = "search wrapped"
	}
	m.jumpToMatch(m.search.CurrentMatch)
	if m.renderer != nil {
		m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	}
	return m, nil
}

// stripSearchPrefixes peels vim-style prefix toggles from the start of
// `query` and returns the cleaned query alongside the resulting
// (regex, caseMode) pair. Recognised prefixes (per contracts/keys.md):
//
//	\v   force regex
//	\V   force literal
//	\c   force case-insensitive
//	\C   force case-sensitive
//
// Multiple prefixes may stack (e.g. `\v\C`); they're consumed in the
// order they appear. Anything past the first non-prefix rune is
// treated as the literal query.
func stripSearchPrefixes(query string, regex bool, caseMode search.CaseMode) (string, bool, search.CaseMode) {
	for {
		if !strings.HasPrefix(query, `\`) || len(query) < 2 {
			break
		}
		switch query[1] {
		case 'v':
			regex = true
		case 'V':
			regex = false
		case 'c':
			caseMode = search.CaseInsensitive
		case 'C':
			caseMode = search.CaseSensitive
		default:
			return query, regex, caseMode
		}
		query = query[2:]
	}
	return query, regex, caseMode
}

// caseModeFromConfig maps the `case_mode` string in cfg
// ("smart"|"sensitive"|"insensitive") to the search.CaseMode enum.
// Unrecognised / empty strings fall back to `fallback` so the caller
// supplies a sensible default (CaseSmart).
func caseModeFromConfig(spec string, fallback search.CaseMode) search.CaseMode {
	switch strings.ToLower(strings.TrimSpace(spec)) {
	case "smart":
		return search.CaseSmart
	case "sensitive":
		return search.CaseSensitive
	case "insensitive":
		return search.CaseInsensitive
	}
	return fallback
}

// advanceMatch cycles through the search results in `delta` direction
// (+1 for `n`, -1 for `N`). Wraps at the ends and sets the wrapped
// status message so the user sees the loop.
func (m *Model) advanceMatch(delta int) {
	if len(m.search.Matches) == 0 {
		return
	}
	cur := m.search.CurrentMatch + delta
	if cur < 0 {
		cur = len(m.search.Matches) - 1
		m.statusAdvisory = "search wrapped"
		m.search.Wrapped = true
	} else if cur >= len(m.search.Matches) {
		cur = 0
		m.statusAdvisory = "search wrapped"
		m.search.Wrapped = true
	} else {
		m.search.Wrapped = false
	}
	m.search.CurrentMatch = cur
	m.jumpToMatch(cur)
	if m.renderer != nil {
		m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	}
}

// jumpToMatch scrolls the viewport so the match's line is at the top.
func (m *Model) jumpToMatch(idx int) {
	if idx < 0 || idx >= len(m.search.Matches) {
		return
	}
	line := m.search.Matches[idx].Line
	m.scrollToLine(line)
}

// scrollToLine clamps `line` to [1, total] and sets the viewport's
// YOffset so that line is the first visible row. With wrap on, the
// row→line mapping is approximate (one source line may span multiple
// visual rows); the simple `line - 1` mapping is correct for the
// no-wrap path and visually close enough for wrap.
func (m *Model) scrollToLine(line int64) {
	total := int64(0)
	if m.stream != nil && m.stream.Buffer != nil {
		total = m.stream.Buffer.Total()
	}
	if total <= 0 {
		return
	}
	if line < 1 {
		line = 1
	}
	if line > total {
		line = total
	}
	m.viewport.SetYOffset(int(line - 1))
}

// runCommand dispatches a `:`-prefixed command. Recognised commands
// match contracts/keys.md; unknown commands surface a status-bar
// warning rather than crashing the viewer.
func (m Model) runCommand(cmd string) (tea.Model, tea.Cmd) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return m, nil
	}
	// Quit aliases.
	if cmd == "q" || cmd == "quit" {
		if m.cancel != nil {
			m.cancel()
		}
		if m.searchCancel != nil {
			m.searchCancel()
			m.searchCancel = nil
		}
		return m, tea.Quit
	}
	// :0 / :$ jumps.
	if cmd == "0" {
		m.scrollToLine(1)
		return m, nil
	}
	if cmd == "$" {
		total := int64(0)
		if m.stream != nil && m.stream.Buffer != nil {
			total = m.stream.Buffer.Total()
		}
		if total > 0 {
			m.scrollToLine(total)
		}
		return m, nil
	}
	// Numeric jump.
	if n, err := strconv.ParseInt(cmd, 10, 64); err == nil {
		// PDF sources interpret `:N` as a page jump rather than a line
		// jump (T082): the rendered "lines" are page text and the user
		// expects `:42` to take them to page 42 of the PDF.
		if m.source != nil && m.source.Kind() == source.KindPDF {
			return m.jumpToPage(int(n))
		}
		total := int64(0)
		if m.stream != nil && m.stream.Buffer != nil {
			total = m.stream.Buffer.Total()
		}
		if total > 0 && n > total {
			m.statusAdvisory = fmt.Sprintf("line %d > total %d", n, total)
		}
		m.scrollToLine(n)
		return m, nil
	}
	// :set commands.
	if strings.HasPrefix(cmd, "set ") {
		return m.runSetCommand(strings.TrimPrefix(cmd, "set "))
	}
	// :open <path>.
	if strings.HasPrefix(cmd, "open ") {
		return m.runOpenCommand(strings.TrimSpace(strings.TrimPrefix(cmd, "open ")))
	}
	m.statusAdvisory = fmt.Sprintf("unknown command: %q", cmd)
	return m, nil
}

// runSetCommand handles `:set vim` / `:set novim` / `:set theme …`
// per contracts/keys.md.
func (m Model) runSetCommand(rest string) (tea.Model, tea.Cmd) {
	rest = strings.TrimSpace(rest)
	switch rest {
	case "vim":
		// Layer vim bindings on the *preserved* non-vim base so any
		// user `[keys]` overrides survive the toggle (Copilot review
		// PR#9 round-3 #5).
		m.vim = true
		m.keyMap = keys.WithVim(m.baseKeyMap)
		m.statusAdvisory = "vim mode on"
		return m, nil
	case "novim":
		m.vim = false
		m.keyMap = m.baseKeyMap
		m.statusAdvisory = "vim mode off"
		return m, nil
	}
	if strings.HasPrefix(rest, "theme ") {
		spec := strings.TrimSpace(strings.TrimPrefix(rest, "theme "))
		newTheme := render.ResolveTheme(spec, m.caps, m.cfg != nil && m.cfg.NoColor)
		m.theme = newTheme
		// Rebuild the renderer so the new theme styles take effect on
		// the next paint.
		kind := source.KindUnknown
		lang := ""
		if m.source != nil {
			kind = m.source.Kind()
			lang = m.source.Metadata().Language
		}
		deps := render.Dependencies{
			Theme:        newTheme,
			Capabilities: m.caps,
			Highlighter:  m.highlighter,
			LineNumbers:  m.cfg != nil && m.cfg.LineNumbers,
			WordWrap:     m.cfg != nil && m.cfg.WordWrap,
			Language:     lang,
			Source:       m.source,
		}
		m.renderer = render.ForKind(kind, deps)
		m.statusAdvisory = fmt.Sprintf("theme: %s", newTheme.Name)
		if m.renderer != nil {
			m.viewport.SetContent(m.renderer.Render(m.renderContext()))
		}
		return m, nil
	}
	m.statusAdvisory = fmt.Sprintf("unknown :set %q", rest)
	return m, nil
}

// runOpenCommand replaces the current source with the file at `path`.
// Reuses the loader/source paths so the new file goes through the same
// detection / streaming pipeline as the original CLI argument.
func (m Model) runOpenCommand(path string) (tea.Model, tea.Cmd) {
	if path == "" {
		m.statusAdvisory = "open: missing path"
		return m, nil
	}
	src, err := source.FromArgs([]string{path}, nil, "")
	if err != nil {
		m.statusAdvisory = fmt.Sprintf("open %s: %v", path, err)
		return m, nil
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	cfg := m.cfg
	return m, func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := loader.Open(ctx, src, loaderConfigFromConfig(cfg))
		if err != nil {
			cancel()
			return openResultMsg{err: err, src: src}
		}
		return openResultMsg{stream: stream, cancel: cancel, src: src}
	}
}

// onToggleLineNumbers flips [config.Config.LineNumbers] and rebuilds
// the renderer so the gutter appears or disappears on the next paint.
// T100c.
func (m Model) onToggleLineNumbers() (tea.Model, tea.Cmd) {
	if m.cfg == nil {
		return m, nil
	}
	m.cfg.LineNumbers = !m.cfg.LineNumbers
	m.rebuildRenderer()
	if m.renderer != nil {
		m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	}
	return m, nil
}

// onToggleWordWrap flips [config.Config.WordWrap], invalidates the
// loader buffer's wrap cache, and rebuilds the renderer so the next
// paint re-wraps from scratch at the active width. T100c.
func (m Model) onToggleWordWrap() (tea.Model, tea.Cmd) {
	if m.cfg == nil {
		return m, nil
	}
	m.cfg.WordWrap = !m.cfg.WordWrap
	if m.stream != nil && m.stream.Buffer != nil {
		m.stream.Buffer.ClearWrapCaches()
	}
	m.rebuildRenderer()
	if m.renderer != nil {
		m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	}
	return m, nil
}

// onOpenFilePrompt opens the command-line prompt pre-populated with
// the `:open ` prefix. The user types the path and presses Enter; on
// Enter the existing `:open` command handler in [runCommand] does the
// loader.Open + source swap. Esc closes the prompt without loading.
// T100c.
func (m Model) onOpenFilePrompt() (tea.Model, tea.Cmd) {
	m.commandLine.Active = true
	m.commandLine.Prefix = ':'
	m.commandLine.Buffer = "open "
	m.commandLine.HistoryCursor = -1
	return m, nil
}

// rebuildRenderer reconstructs [render.Dependencies] from the current
// session state and invokes [render.ForKind] so the renderer's cached
// LineNumbers / WordWrap / Theme reflect the latest config. Used by
// the toggle handlers and the `:set theme …` runtime swap.
func (m *Model) rebuildRenderer() {
	kind := source.KindUnknown
	lang := ""
	if m.source != nil {
		kind = m.source.Kind()
		lang = m.source.Metadata().Language
	}
	deps := render.Dependencies{
		Theme:        m.theme,
		Capabilities: m.caps,
		Highlighter:  m.highlighter,
		LineNumbers:  m.cfg != nil && m.cfg.LineNumbers,
		WordWrap:     m.cfg != nil && m.cfg.WordWrap,
		Language:     lang,
		Source:       m.source,
	}
	m.renderer = render.ForKind(kind, deps)
}

// onChunk re-renders the viewport on every chunk arrival so streamed
// content appears progressively. The buffer is updated by the loader
// itself; the highlighter populates Tokens, then [LineBuffer.SetTokens]
// pushes those tokens into the buffer's stored copies so subsequent
// renders / scrolls don't re-lex (Copilot review PR#8 #3).
//
// On EOF the handler fires a follow-up [metaUpdatedMsg] so the status
// bar's "<running>… lines" indicator flips to the final pinned total
// on the next paint (T100). The follow-up is sequenced after the
// in-line [viewport.SetContent] so the footer's "…" disappears
// immediately rather than waiting for the next user keystroke.
func (m Model) onChunk(msg chunkLoadedMsg) (tea.Model, tea.Cmd) {
	hitEOF := msg.chunk.EOF
	if hitEOF {
		m.streaming = false
		if m.status == render.StatusStreaming {
			m.status = render.StatusIdle
		}
	}
	if m.highlighter != nil && m.source != nil && m.source.Kind() == source.KindCode {
		highlightLines(m.highlighter, m.source.Metadata().Language, msg.chunk.Lines)
		if m.stream != nil && m.stream.Buffer != nil {
			m.stream.Buffer.SetTokens(msg.chunk.Lines)
		}
	}
	m.maybeAdvisoryFromHighlighter()
	m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	if m.streaming {
		return m, waitForChunk(m.stream)
	}
	if hitEOF {
		return m, metaUpdatedCmd(m.stream)
	}
	return m, nil
}

// onStreamErr translates a [loader.Stream.Errs] arrival into a
// status-bar advisory and re-subscribes for the next warning.
//
// Stale-stream guard mirrors [chunkLoadedMsg] / [streamDoneMsg]:
// warnings from an old stream that ActionReload or :open replaced
// must not surface against the new session.
//
// nil errors are skipped (re-subscription only); they shouldn't
// happen in practice but the loader's `select / default` send is
// best-effort and a nil send would corrupt the advisory.
func (m Model) onStreamErr(msg streamErrMsg) (tea.Model, tea.Cmd) {
	if msg.stream != nil && msg.stream != m.stream {
		return m, nil
	}
	if msg.err == nil {
		return m, waitForStreamErr(m.stream)
	}
	m.statusAdvisory = formatStreamErr(msg.err)
	return m, waitForStreamErr(m.stream)
}

// formatStreamErr renders a [loader.Stream.Errs] error into a
// terse one-line advisory suitable for the status bar.
//
// The two documented sentinels (per loader/stream.go:74-80) are:
//   - ErrLineTruncated, wrapped as "%w: line N" — surfaced as
//     "line N truncated (cap exceeded)" so the user sees both the
//     fact and the affected location.
//   - ErrStdinNonSeekable — surfaced verbatim because the existing
//     wording already explains what the user lost ("scroll-back
//     disabled past resident window").
//
// Anything else falls through to the wrapped error string. The
// status bar collapses below 80 cols and drops the advisory anyway,
// so we keep the format short but informative.
func formatStreamErr(err error) string {
	if errors.Is(err, loader.ErrLineTruncated) {
		// The loader wraps with line number: "line truncated: line N".
		// Re-shape so the line number leads (more relevant to the
		// user) and the cause follows.
		s := err.Error()
		if idx := strings.LastIndex(s, "line "); idx >= 0 {
			lineN := s[idx:]
			return lineN + " truncated (cap exceeded)"
		}
		return "line truncated (cap exceeded)"
	}
	if errors.Is(err, loader.ErrStdinNonSeekable) {
		return err.Error()
	}
	return "loader: " + err.Error()
}

// metaUpdatedCmd returns a tea.Cmd that yields a [metaUpdatedMsg]
// carrying the buffer's pinned total. nil-safe for streams without a
// buffer (degenerate test models).
func metaUpdatedCmd(s *loader.Stream) tea.Cmd {
	if s == nil || s.Buffer == nil {
		return nil
	}
	total := s.Buffer.Total()
	return func() tea.Msg { return metaUpdatedMsg{TotalLines: total} }
}

// onReload implements ActionReload. Cancels the in-flight loader,
// reopens the source, and swaps the buffer atomically on success. On
// failure the prior buffer is retained and the error surfaces in the
// status bar via m.lastError + m.status = StatusError.
//
// Reload also clears the source's cached detection so a file that's
// changed kind on disk (text → PDF, swapped via tooling, etc.) is
// re-classified against the current bytes — without this the new
// content would render through the stale lexer/Kind picked up at the
// initial Open (Copilot review acceptance M2).
func (m Model) onReload() (tea.Model, tea.Cmd) {
	if m.source == nil {
		return m, nil
	}
	// Cancel the previous loader so its background goroutine exits before
	// we open a fresh stream against the same source. The new context is
	// returned via reloadResultMsg so onReloadResult can install it.
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	// Cancel any in-flight async search against the old buffer; the
	// upcoming buffer swap would invalidate its result anyway.
	if m.searchCancel != nil {
		m.searchCancel()
		m.searchCancel = nil
	}
	// Bump searchGen so any post-cancel result that races through still
	// gets dropped by onSearchResult's stale guard.
	m.searchGen++
	// Reload swaps the underlying buffer; any prior matches no longer
	// map to valid line offsets, and the in-flight result will be
	// dropped on arrival (gen mismatch). Clear the transient search
	// state explicitly so the UI doesn't display stale highlights or
	// a stuck "scan in flight" indicator after the buffer swap (PR#22
	// Copilot review — without this, Pending stays true indefinitely
	// when reload races with a Pending search). CurrentMatch=-1 (the
	// "no match selected" sentinel that runSearch uses while Pending);
	// 0 would mean "first match selected" but Matches is nil here.
	m.search.Pending = false
	m.search.Matches = nil
	m.search.CurrentMatch = -1
	src := m.source
	if rd, ok := src.(interface{ Redetect() }); ok {
		rd.Redetect()
	}
	cfg := m.cfg
	return m, func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := loader.Open(ctx, src, loaderConfigFromConfig(cfg))
		if err != nil {
			cancel()
			return reloadResultMsg{err: err}
		}
		return reloadResultMsg{stream: stream, cancel: cancel}
	}
}

// onReloadResult installs the new stream when the reload succeeded;
// otherwise records the error and keeps the prior buffer so the user
// still sees content.
func (m Model) onReloadResult(msg reloadResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = render.StatusError
		m.lastError = msg.err
		m.viewport.SetContent(m.renderer.Render(m.renderContext()))
		return m, nil
	}
	m.stream = msg.stream
	m.cancel = msg.cancel
	m.status = render.StatusStreaming
	m.streaming = true
	m.lastError = nil
	m.totalLines = 0 // reload restarts streaming; finalized cache must clear
	if m.stream.First.EOF {
		m.streaming = false
		m.status = render.StatusIdle
	}
	if m.highlighter != nil && m.source != nil && m.source.Kind() == source.KindCode {
		highlightLines(m.highlighter, m.source.Metadata().Language, m.stream.First.Lines)
		if m.stream.Buffer != nil {
			m.stream.Buffer.SetTokens(m.stream.First.Lines)
		}
	}
	m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	if m.streaming {
		// Re-subscribe to BOTH chunks AND errs after a reload —
		// see the matching comment in [Model.onOpenResult] for
		// the C7 background.
		return m, tea.Batch(waitForChunk(m.stream), waitForStreamErr(m.stream))
	}
	return m, waitForStreamErr(m.stream)
}

// loaderConfigFromConfig pulls the loader-shaped fields out of the
// session config so ActionReload's [loader.Open] uses the same tuning
// the original Open did.
func loaderConfigFromConfig(cfg *config.Config) loader.Config {
	if cfg == nil {
		return loader.Config{}
	}
	return loader.Config{
		MaxResidentBytes: cfg.MaxResidentBytes,
		WindowSize:       cfg.WindowSize,
	}
}

// maybeAdvisoryFromHighlighter drains a single Warning from the
// highlighter's side channel and stages it as the model's advisory.
// Phase 3 surfaces the message via m.statusAdvisory; the full
// auto-clear timer (5 s per contracts/internal-apis.md) lands with the
// US6 status bar.
//
// The receive checks `ok` so a future change that closes the channel
// can't repeatedly deliver the zero-value Warning (which would bind to
// WarnHighlightDisabled and re-stage the advisory every tick) —
// Copilot review PR#8 #6.
func (m *Model) maybeAdvisoryFromHighlighter() {
	if m.highlighter == nil {
		return
	}
	select {
	case w, ok := <-m.highlighter.Warns():
		if !ok {
			return
		}
		switch w.Kind {
		case highlight.WarnHighlightDisabled:
			m.statusAdvisory = "highlighting disabled"
		}
	default:
	}
}

// onNextPage advances the PDF page cursor by one. Non-PDF sources are
// a no-op so the keymap binding doesn't accidentally scroll a code
// buffer when the user remaps `]` / `[`. The handler clamps to the
// total page count when the loader has surfaced one.
func (m Model) onNextPage() (tea.Model, tea.Cmd) {
	if m.source == nil || m.source.Kind() != source.KindPDF {
		return m, nil
	}
	prev := m.page
	m.page++
	if total := m.source.Metadata().PageCount; total > 0 && m.page > total {
		m.page = total
		m.statusAdvisory = "already on last page"
	}
	m.applyPageChange(prev)
	return m, nil
}

// onPrevPage decrements the PDF page cursor, clamping at 1.
func (m Model) onPrevPage() (tea.Model, tea.Cmd) {
	if m.source == nil || m.source.Kind() != source.KindPDF {
		return m, nil
	}
	if m.page <= 1 {
		m.page = 1
		m.statusAdvisory = "already on first page"
		return m, nil
	}
	prev := m.page
	m.page--
	m.applyPageChange(prev)
	return m, nil
}

// jumpToPage handles a `:N` command when the source is a PDF. The
// normal `:N` line jump in [runCommand] still applies for text-shaped
// sources; this branch only fires when the active source is KindPDF
// so we don't surprise a user who typed `:42` in a code buffer.
func (m Model) jumpToPage(n int) (tea.Model, tea.Cmd) {
	if n < 1 {
		n = 1
	}
	total := 0
	if m.source != nil {
		total = m.source.Metadata().PageCount
	}
	if total > 0 && n > total {
		m.statusAdvisory = fmt.Sprintf("page %d > total %d", n, total)
		n = total
	}
	prev := m.page
	m.page = n
	m.applyPageChange(prev)
	return m, nil
}

// applyPageChange re-renders the viewport for the new page cursor and,
// when the page actually changed, snaps scroll back to the top so the
// user doesn't land halfway down a fresh page (or past the end of a
// shorter one). `prev` is the cursor value before the change so a
// no-op `]` on the last page is a true no-op — neither the rendered
// content nor the scroll position is touched. Copilot review PR#11
// round-2 #1, #5, #6.
func (m *Model) applyPageChange(prev int) {
	if m.page == prev {
		return
	}
	if m.renderer != nil {
		m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	}
	m.viewport.GotoTop()
	m.viewport.SetXOffset(0)
}

// renderContext bundles the per-frame state the renderer needs.
func (m Model) renderContext() render.RenderContext {
	page := m.page
	if page <= 0 {
		page = 1
	}
	ctx := render.RenderContext{
		Theme:        m.theme,
		Capabilities: m.caps,
		Viewport:     m.viewport,
		Status:       m.status,
		LastError:    m.lastError,
		Search:       m.search,
		Page:         page,
	}
	if m.stream != nil {
		ctx.Buffer = m.stream.Buffer
	}
	if m.streaming && m.status == render.StatusIdle {
		// Defensive: if the model says we're streaming but the status
		// already idled, prefer the streaming signal so renderers that
		// use Status for the loading indicator stay honest.
		ctx.Status = render.StatusStreaming
	}
	return ctx
}

// matchAction reports whether the supplied key event matches any
// binding for `action` in the provided keymap. The bubble-tea key
// package's Matches helper is used so multi-key bindings (eg.
// Ctrl-C composed of "ctrl+c") work without manual parsing.
func matchAction(km keys.KeyMap, action keys.Action, msg tea.KeyMsg) bool {
	bindings, ok := km[action]
	if !ok {
		return false
	}
	for _, b := range bindings {
		if key.Matches(msg, b) {
			return true
		}
	}
	return false
}

// waitForChunk subscribes to the loader's Updates channel and yields
// each chunk as a Bubble Tea message tagged with the originating
// stream. The handler in [Model.Update] uses the tag to discard
// stale messages that arrive after ActionReload swapped the stream
// pointer (Copilot review PR#8 #2).
func waitForChunk(s *loader.Stream) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-s.Updates
		if !ok {
			return streamDoneMsg{stream: s}
		}
		return chunkLoadedMsg{chunk: c, stream: s}
	}
}

// waitForStreamErr subscribes to the loader's Errs channel and
// forwards each received warning / error as a tea.Msg tagged with
// the originating stream. The handler in [Model.Update] uses the
// tag to drop stale warnings from a stream that ActionReload /
// :open has already swapped out (matches the [waitForChunk]
// tagging convention), and [Model.onStreamErr] guards against the
// theoretical nil-error case from the loader's best-effort
// `select / default` send.
//
// Returns nil when the channel closes. A closed Errs means this
// stream will not emit any more warnings / errors, regardless of
// whether Updates has already closed (the loader closes them in
// different orders depending on whether streaming finished
// synchronously in [loader.Open] or asynchronously on the producer
// goroutine — defers fire LIFO). We don't need a separate "errs
// done" sentinel because streamDoneMsg remains the signal for the
// streaming-finished transition.
//
// Acceptance review C7: before this command existed, the loader's
// errs channel was buffered, written to, and never read — when the
// channel filled up the loader's `select / default` non-blocking
// send dropped subsequent warnings silently, so users were never
// told a 100 KiB+ line was truncated or that stdin scroll-back
// was disabled.
func waitForStreamErr(s *loader.Stream) tea.Cmd {
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		err, ok := <-s.Errs
		if !ok {
			return nil
		}
		return streamErrMsg{err: err, stream: s}
	}
}
