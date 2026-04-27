// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// View produces the full frame for tea.Program. The viewport claims
// the height minus one footer row; when a `:` / `/` / `?` prompt is
// active, the footer row is replaced with the prompt line so the user
// sees what they're typing without losing the file context.
//
// In very tight terminals (height <= 1) onResize sets m.showFooter =
// false and we drop the footer/prompt rather than overflow the row
// budget (Copilot review PR#7 #23).
func (m Model) View() tea.View {
	var content string
	if m.width != 0 && m.height != 0 {
		if !m.showFooter {
			content = m.viewport.View()
		} else if m.commandLine.Active {
			content = m.viewport.View() + "\n" + m.promptLine()
		} else {
			content = m.viewport.View() + "\n" + m.footerLine()
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// promptLine renders the active `:` / `/` / `?` prompt buffer with the
// theme's PromptLine style. The prefix sigil is always shown so the
// user can tell which prompt is active without watching the keymap.
//
// In mono mode (`--no-color` / NO_COLOR=1 / TERM=dumb) the lipgloss
// style is bypassed so no ANSI leaks into the rendered frame
// (Copilot review PR#9 round-3 #3).
func (m Model) promptLine() string {
	line := string(m.commandLine.Prefix) + m.commandLine.Buffer
	pad := m.width - lipgloss.Width(line)
	if pad > 0 {
		line += pads(pad)
	}
	if m.isMono() {
		return line
	}
	return m.theme.PromptLine.Render(line)
}

// footerLine renders the US6 status bar via [render.StatusBarRender].
// The model collects the dynamic state (current line, streaming flag,
// PDF page cursor, advisory) into a [render.StatusInput] and lets the
// status-bar package handle the format + collapse logic.
func (m Model) footerLine() string {
	name := "<no source>"
	kind := source.KindUnknown
	var meta source.Metadata
	if m.source != nil {
		name = filepath.Base(m.source.DisplayName())
		kind = m.source.Kind()
		meta = m.source.Metadata()
	}
	// Surface the running / finalized total via Metadata.LineCount so
	// the status bar's "<n>… lines" indicator stays accurate while the
	// loader is still streaming AND flips to the final count after
	// EOF. Prefer the [metaUpdatedMsg]-cached m.totalLines once it's
	// non-zero so the footer doesn't take the buffer mutex on every
	// paint; while streaming the cache is zero and we read the
	// running total from the buffer instead.
	//
	// M14 verified: [loader.LineBuffer.Total] takes the buffer mutex
	// for a single int64 read inside the lock — no torn reads. The
	// underlying total only ever increases (Append grows
	// startLine + len(lines); MarkComplete pins totalLines), so
	// successive paints observe a monotonically non-decreasing value.
	// metaUpdatedMsg arrival is the same monotonic sequence: any
	// paint between streamDoneMsg and metaUpdatedMsg reads
	// Buffer.Total() directly and sees the already-pinned final
	// total (MarkComplete fired before close(updates) which is what
	// triggered streamDoneMsg). The footer therefore cannot race
	// between two frames — the worst case is reading a slightly
	// stale running total before metaUpdatedMsg lands, and that is
	// exactly the M5 streaming display the spec asked for.
	if m.totalLines > 0 {
		meta.LineCount = m.totalLines
	} else if m.stream != nil && m.stream.Buffer != nil {
		meta.LineCount = m.stream.Buffer.Total()
	}
	current := int64(0)
	if m.renderer != nil {
		// Map the viewport's visual top row to the matching source
		// line; that's the only path that stays consistent across
		// windowed-mode eviction (Copilot review PR#7 #15) and
		// word-wrap inflation (Copilot review PR#7 #24). For empty
		// input the renderer returns 0 which doubles as the
		// contracts/cli.md "Line 0" footer sentinel.
		current = m.renderer.RowToLine(m.renderContext(), m.viewport.YOffset)
	}
	page := 0
	if kind == source.KindPDF {
		page = m.page
	}
	in := render.StatusInput{
		DisplayName: name,
		Meta:        meta,
		Viewport:    m.viewport,
		Width:       m.width,
		Current:     current,
		Page:        page,
		Streaming:   m.streaming,
		Advisory:    m.statusAdvisory,
		Mono:        m.isMono(),
		Kind:        kind,
	}
	return render.StatusBarRender(in, m.theme)
}

// isMono reports whether the active session is in mono / no-color
// mode. Used by the chrome-rendering helpers (footer, prompt) to
// suppress lipgloss styling when ANSI must not be emitted.
func (m Model) isMono() bool {
	if m.theme.Mono {
		return true
	}
	return m.caps.ColorDepth == term.ColorMono
}

func pads(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}
