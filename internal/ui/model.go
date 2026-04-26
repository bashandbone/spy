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
	// already shows colours rather than waiting for the next tick. The
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

// reloadMsg requests a fresh loader.Open against the current Source.
// Fired by the keymap's ActionReload binding.
type reloadMsg struct{}

// metaUpdatedMsg announces that the loader has finalized the source's
// total line count (or, in the future, page count). The model fires it
// as a follow-up tea.Cmd whenever it observes an EOF chunk so the
// status bar can flip from "<running>… lines" to the final "<total>
// lines" rendering on the next paint without waiting for another
// scroll / resize / chunk to trigger a redraw.
//
// Per T100 (specs/001-popup-reader/tasks.md). The metadata-only update
// keeps the rest of the model state untouched — it's a render-only
// signal, not a state mutation, so the [Update] handler simply
// re-renders.
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
