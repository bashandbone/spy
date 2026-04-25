// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
)

// Init sets up the initial command pipeline: subscribe to the loader's
// Updates channel via [waitForChunk] and seed the viewport with the
// first chunk's content.
func (m Model) Init() tea.Cmd {
	if m.stream == nil {
		return nil
	}
	return waitForChunk(m.stream)
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
		return m, nil
	case reloadMsg:
		return m.onReload()
	case reloadResultMsg:
		return m.onReloadResult(msg)
	}
	return m, nil
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
// into a tea.Cmd. Phase 3 adds ActionReload on top of the foundational
// scroll/quit actions; the rest are added in their story phases.
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if matchAction(m.keyMap, keys.ActionQuit, msg) {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	if matchAction(m.keyMap, keys.ActionReload, msg) {
		return m, func() tea.Msg { return reloadMsg{} }
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
	return m, nil
}

// onChunk re-renders the viewport on every chunk arrival so streamed
// content appears progressively. The buffer is updated by the loader
// itself; the highlighter populates Tokens, then [LineBuffer.SetTokens]
// pushes those tokens into the buffer's stored copies so subsequent
// renders / scrolls don't re-lex (Copilot review PR#8 #3).
func (m Model) onChunk(msg chunkLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.chunk.EOF {
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
	return m, nil
}

// onReload implements ActionReload. Cancels the in-flight loader,
// reopens the source, and swaps the buffer atomically on success. On
// failure the prior buffer is retained and the error surfaces in the
// status bar via m.lastError + m.status = StatusError.
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
	src := m.source
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
		return m, waitForChunk(m.stream)
	}
	return m, nil
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

// renderContext bundles the per-frame state the renderer needs.
func (m Model) renderContext() render.RenderContext {
	ctx := render.RenderContext{
		Theme:        m.theme,
		Capabilities: m.caps,
		Viewport:     m.viewport,
		Status:       m.status,
		LastError:    m.lastError,
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
