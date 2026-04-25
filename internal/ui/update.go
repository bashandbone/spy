// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
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
		return m.onChunk(msg)
	case streamDoneMsg:
		m.streaming = false
		return m, nil
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
// into a tea.Cmd. Only the foundational actions (quit, scroll up/down
// and pages) are handled here; the rest are added in their story
// phases.
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if matchAction(m.keyMap, keys.ActionQuit, msg) {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
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
// itself; we only need to repaint.
func (m Model) onChunk(msg chunkLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.chunk.EOF {
		m.streaming = false
	}
	m.viewport.SetContent(m.renderer.Render(m.renderContext()))
	if m.streaming {
		return m, waitForChunk(m.stream)
	}
	return m, nil
}

// renderContext bundles the per-frame state the renderer needs.
func (m Model) renderContext() render.RenderContext {
	ctx := render.RenderContext{
		Theme:        m.theme,
		Capabilities: m.caps,
		Viewport:     m.viewport,
	}
	if m.stream != nil {
		ctx.Buffer = m.stream.Buffer
	}
	if m.streaming {
		ctx.Status = render.StatusStreaming
	} else {
		ctx.Status = render.StatusIdle
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
// each chunk as a Bubble Tea message. Returns nil when the channel
// closes so the model stops re-subscribing on streamDoneMsg.
func waitForChunk(s *loader.Stream) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-s.Updates
		if !ok {
			return streamDoneMsg{}
		}
		return chunkLoadedMsg{chunk: c}
	}
}
