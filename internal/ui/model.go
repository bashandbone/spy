// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package ui implements the Bubble Tea Model that drives the viewer.
// The Phase 2 foundational viewer wires loader → render → viewport with
// quit-on-q/esc/Ctrl-C and arrow-key scrolling. US1 (Phase 3) layers
// streaming syntax highlighting + ActionReload on top; search, vim,
// command line, and graphics cleanup are added in their respective
// story phases (US2–US6).
package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/viewport"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/search"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// ModelOptions matches contracts/internal-apis.md `internal/ui` plus
// the US1 additions for the highlighter and reload context.
type ModelOptions struct {
	Source       source.Source
	Stream       *loader.Stream
	Capabilities term.Capabilities
	Config       *config.Config
	Theme        render.Theme
	KeyMap       keys.KeyMap

	// BaseKeyMap is the non-vim keymap (defaults + user [keys]
	// overrides). The Model retains this so a runtime `:set novim`
	// can restore it verbatim and `:set vim` can layer
	// [keys.WithVim] on top without losing user overrides (Copilot
	// review PR#9 round-3 #5). When zero/nil, NewModel uses KeyMap as
	// the base — matches pre-existing call sites that don't toggle
	// vim mode at runtime.
	BaseKeyMap keys.KeyMap

	// Highlighter is the per-session syntax highlighter. nil disables
	// highlighting (used by the foundational text path and tests).
	Highlighter *highlight.Highlighter

	// Cancel cancels the loader's background streaming goroutine; the
	// model fires it on tea.Quit so Open's goroutine exits before the
	// program returns. Optional; nil is safe.
	Cancel context.CancelFunc
}

// Model is the Bubble Tea state for a viewer session. The fields are
// unexported because callers construct via [NewModel]; once constructed,
// the model is owned by tea.NewProgram for the duration of the run.
type Model struct {
	source      source.Source
	stream      *loader.Stream
	caps        term.Capabilities
	cfg         *config.Config
	theme       render.Theme
	keyMap      keys.KeyMap
	baseKeyMap  keys.KeyMap // non-vim base; preserved for :set novim
	cancel      context.CancelFunc
	highlighter *highlight.Highlighter
	renderer    render.Renderer

	viewport viewport.Model
	width    int
	height   int

	// showFooter mirrors the onResize decision: when the terminal is
	// too short for both viewport + footer (height <= 1), the footer
	// is suppressed so we don't overflow into a phantom row.
	showFooter bool

	// streaming flips false on the first chunkLoadedMsg with EOF=true;
	// before then the footer advertises "loading..." instead of the
	// final line count.
	streaming bool

	// status mirrors the foundational [render.Status] surfaced through
	// renderContext. ActionReload bumps it to StatusError on failure;
	// successful reloads return it to StatusStreaming until EOF.
	status render.Status

	// lastError holds the most recent ActionReload (or future load)
	// failure so the status bar can surface it. Cleared on the next
	// successful reload.
	lastError error

	// statusAdvisory is the highlighter's one-shot warning surfaced in
	// the footer (e.g. "highlighting disabled"). Empty when no
	// advisory is active.
	statusAdvisory string

	// search holds the current search engine state. Inactive when
	// search.Query == "". US2 (T056).
	search search.State
	// commandLine is the live `:` / `/` / `?` prompt state machine.
	commandLine CommandLineState
	// vimPendingG carries the "first g" of a vim "gg" sequence. The
	// next key resolves it: another `g` fires ActionGoToTop, anything
	// else cancels and falls through to the regular dispatch path.
	vimPendingG bool
	// vim mirrors cfg.VimMode at construction; runtime `:set vim` /
	// `:set novim` toggles flip this so the UI's prompt-mode + nav
	// dispatch picks up the new keymap and `gg` sequencing.
	vim bool

	// page is the 1-indexed PDF page cursor wired in T082. Non-PDF
	// kinds ignore the field; PDF renderers read it via [render.RenderContext].
	// Defaults to 1 so the first paint shows page 1 even when the
	// user hasn't pressed `]` / `[` yet.
	page int

	// totalLines caches the source's finalized line count once
	// [metaUpdatedMsg] arrives. Zero while streaming (the footer
	// falls back to [loader.LineBuffer.Total] in that case). The
	// cache lets the footer read the post-EOF total without hitting
	// the buffer mutex on every paint and gives the metaUpdatedMsg
	// payload an observable consumer.
	totalLines int64

	// searchCancel cancels the previous search's background goroutine
	// when a fresh search starts. Per the acceptance review (H5),
	// runSearch used to drain the unbuffered [search.Scan] channel
	// synchronously on the Bubble Tea event-loop goroutine — a
	// 50 MB / 1 M-line resident buffer froze key/resize events for
	// seconds. The async path consumes the channel in a tea.Cmd-spawned
	// goroutine and uses this CancelFunc to abort prior searches when
	// the user submits a new query (rapid-typing must not pile up
	// goroutines or let stale results overwrite current state).
	searchCancel context.CancelFunc

	// searchGen is bumped each time runSearch dispatches a fresh
	// scan. The async tea.Cmd captures the current value into the
	// [searchResultMsg.gen] field; on receipt the Update handler
	// drops any message whose gen doesn't match the live counter
	// (mirrors the "stale message guard" pattern used for stream
	// pointers in [chunkLoadedMsg] / [streamErrMsg]). Without this
	// guard a slow scan that completes after the user has already
	// started a new search would silently overwrite the new
	// session's results.
	searchGen uint64

	// openCancel is the in-flight cancel for an active `:open <path>`
	// command. Per the acceptance review (M6), runOpenCommand
	// previously ran loader.Open in a tea.Cmd-spawned goroutine and
	// only handed its CancelFunc to the model via openResultMsg. If
	// the user quit between the command dispatch and the message
	// arrival, Bubble Tea drops the pending message — leaking the
	// new stream's reader goroutine and its CancelFunc. By stashing
	// the cancel on the model BEFORE returning the tea.Cmd, the
	// quit paths (ActionQuit, `:q`/`:quit`) can call openCancel()
	// up-front and tear down the in-flight loader regardless of
	// whether the message arrived. Cleared once openResultMsg lands
	// (success: ownership moves to m.cancel; error: the closure
	// already invoked cancel before the message was sent).
	openCancel context.CancelFunc

	// openGen is bumped each time runOpenCommand dispatches a fresh
	// :open. The async tea.Cmd captures the current value into
	// [openResultMsg.gen]; on receipt the Update handler drops any
	// message whose gen doesn't match the live counter. Without this,
	// a stale openResultMsg from a cancelled prior :open would clear
	// the m.openCancel belonging to the *current* in-flight :open and
	// reintroduce the leak this guard was added to prevent (PR#26
	// review). Mirrors the searchGen pattern used for H5.
	openGen uint64
}

// NewModel constructs the viewer's Bubble Tea model. The first frame
// uses the synchronously-loaded First chunk from the loader stream so
// the alt-screen paints immediately; further chunks arrive via
// chunkLoadedMsg as the producer streams.
func NewModel(opts ModelOptions) Model {
	lang := ""
	kind := source.KindUnknown
	if opts.Source != nil {
		kind = opts.Source.Kind()
		lang = opts.Source.Metadata().Language
	}
	if opts.Highlighter != nil && lang != "" {
		opts.Highlighter.SetLang(lang)
	}
	deps := render.Dependencies{
		Theme:        opts.Theme,
		Capabilities: opts.Capabilities,
		Highlighter:  opts.Highlighter,
		LineNumbers:  opts.Config != nil && opts.Config.LineNumbers,
		WordWrap:     opts.Config != nil && opts.Config.WordWrap,
		Language:     lang,
		Source:       opts.Source,
	}
	baseKM := opts.BaseKeyMap
	if baseKM == nil {
		baseKM = opts.KeyMap
	}
	m := Model{
		source:      opts.Source,
		stream:      opts.Stream,
		caps:        opts.Capabilities,
		cfg:         opts.Config,
		theme:       opts.Theme,
		keyMap:      opts.KeyMap,
		baseKeyMap:  baseKM,
		cancel:      opts.Cancel,
		highlighter: opts.Highlighter,
		renderer:    render.ForKind(kind, deps),
		streaming:   true,
		status:      render.StatusStreaming,
		commandLine: CommandLineState{HistoryCursor: -1},
		vim:         opts.Config != nil && opts.Config.VimMode,
		page:        1,
	}
	if opts.Stream != nil && opts.Stream.First.EOF {
		m.streaming = false
		m.status = render.StatusIdle
	}
	// Highlight the synchronously-loaded First chunk so the first paint
	// already shows colors rather than waiting for the next tick. The
	// loader has already copied the lines into Stream.Buffer (Append
	// does a struct-copy), so we must *also* push the resulting tokens
	// back into the buffer via SetTokens — otherwise the renderer
	// re-lexes on every frame and the byte cap exhausts after a few
	// repaints (Copilot review PR#8 #1).
	if opts.Stream != nil && opts.Highlighter != nil && kind == source.KindCode {
		highlightLines(opts.Highlighter, lang, opts.Stream.First.Lines)
		if opts.Stream.Buffer != nil {
			opts.Stream.Buffer.SetTokens(opts.Stream.First.Lines)
		}
	}
	return m
}

// chunkLoadedMsg announces that the loader produced another chunk. The
// model's Update routes it through the buffer (already done by the
// loader internally) and re-renders. The originating `*loader.Stream`
// is carried so the handler can ignore stale chunks from a stream that
// ActionReload has already swapped out (Copilot review PR#8 #2).
type chunkLoadedMsg struct {
	chunk  loader.Chunk
	stream *loader.Stream
}

// streamDoneMsg is sent when the loader's Updates channel closes.
// Carries the originating Stream so post-reload deliveries from the
// previous loader don't stomp on the new stream's status.
type streamDoneMsg struct {
	stream *loader.Stream
}

// streamErrMsg announces a warning or error read from
// [loader.Stream.Errs] — the side-channel the loader uses to surface
// non-fatal conditions like [loader.ErrLineTruncated] (a single line
// exceeded MaxLineBytes and was clipped) or [loader.ErrStdinNonSeekable]
// (windowed-mode entered against non-seekable stdin so scroll-back is
// unavailable past the resident region).
//
// The originating Stream is carried so post-reload deliveries from
// the previous loader don't surface against the new session's source
// (acceptance review C7 — `Stream.Errs` was unconsumed in production
// before this message landed).
type streamErrMsg struct {
	err    error
	stream *loader.Stream
}

// reloadMsg requests a fresh loader.Open against the current Source.
// Fired by the keymap's ActionReload binding.
type reloadMsg struct{}

// metaUpdatedMsg announces that the loader has finalized the source's
// total line count. The model fires it as a follow-up tea.Cmd
// whenever it observes an EOF chunk so the status bar can flip from
// "<running>… lines" to the final "<total> lines" rendering on the
// next paint without waiting for another scroll / resize / chunk to
// trigger a redraw.
//
// Per T100 (specs/001-popup-reader/tasks.md). The handler stores
// TotalLines on [Model.totalLines] so the footer reads the cached
// finalized count instead of taking the buffer mutex on every paint;
// while streaming the cache is zero and the footer falls back to
// [loader.LineBuffer.Total]. This is the message's observable effect
// (Copilot review PR#13 round-2 #4 + #5 — TotalLines is consumed,
// the docstring no longer claims the handler "simply re-renders").
type metaUpdatedMsg struct {
	TotalLines int64
}

// reloadResultMsg carries the outcome of an in-flight reload request.
// On success Stream and (optional) Cancel replace the model's; on
// failure Err is non-nil and the model retains the prior buffer.
type reloadResultMsg struct {
	stream *loader.Stream
	cancel context.CancelFunc
	err    error
}

// openResultMsg carries the outcome of an in-flight `:open <path>`
// command. On success the model swaps in the new source and stream;
// the rest of the session state (cfg / caps / highlighter / theme /
// keymap) survives unchanged on the receiving Model so we don't need
// to round-trip it through the message. On failure the prior session
// is retained and the error is surfaced via statusAdvisory.
type openResultMsg struct {
	stream *loader.Stream
	cancel context.CancelFunc
	src    source.Source
	err    error
	// gen is the value of [Model.openGen] at the moment the
	// originating runOpenCommand dispatched this message. Update
	// drops the message if it no longer matches the live counter,
	// preventing a stale result from a cancelled prior :open from
	// clearing the m.openCancel of a newer in-flight :open.
	gen uint64
}

// searchResultMsg carries the outcome of an asynchronous search scan.
// The scan goroutine, spawned by [Model.runSearch] via a tea.Cmd, drains
// [search.Scan]'s unbuffered result channel off the event-loop
// goroutine and yields one of these once the scan completes (or is
// cancelled). The handler in [Model.Update] applies the matches,
// computes the initial selection, and clears [search.State.Pending].
//
// `gen` tags the message with the searchGen value the dispatching
// runSearch captured. If a newer search has since started — bumping
// [Model.searchGen] — the result is stale and the handler ignores it
// to prevent a slow first scan from clobbering a fresh second scan
// (acceptance review H5). The pattern mirrors the stale-stream
// guard on [chunkLoadedMsg] / [streamErrMsg].
type searchResultMsg struct {
	gen        uint64
	query      string
	dir        search.Direction
	regex      bool
	caseMode   search.CaseMode
	matches    []search.Match
	scanWrapAt int  // index in `matches` where the wrap section begins (-1 if no wrap)
	cancelled  bool // true when the scan exited via ctx cancellation
}

// highlightLines runs the highlighter against `lines` in place. Exists
// as a standalone helper so [NewModel] (constructing) and [onChunk]
// (post-arrival) share one path.
func highlightLines(h *highlight.Highlighter, lang string, lines []source.Line) {
	if h == nil {
		return
	}
	for i := range lines {
		if lines[i].Tokens != nil {
			continue
		}
		lines[i].Tokens = h.Highlight(lang, lines[i].Raw)
	}
}
