// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

// View produces the full frame for tea.Program. Phase 2 emits the
// viewport plus a single-line footer when there's space for one; the
// full status bar (US6) lands in T098–T100.
//
// In very tight terminals (height <= 1) onResize sets m.showFooter =
// false and we drop the footer rather than overflow the row budget
// (Copilot review PR#7 #23).
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if !m.showFooter {
		return m.viewport.View()
	}
	return m.viewport.View() + "\n" + m.footerLine()
}

// footerLine renders the foundational footer: <displayname> | <total>
// lines | Line <current>. Streaming renders a "…" indicator until the
// loader emits its EOF chunk.
func (m Model) footerLine() string {
	name := "<no source>"
	if m.source != nil {
		name = filepath.Base(m.source.DisplayName())
	}
	total := int64(0)
	if m.stream != nil && m.stream.Buffer != nil {
		total = m.stream.Buffer.Total()
	}
	// Ask the renderer to map the viewport's visual row at top to the
	// matching source line; that's the only path that stays consistent
	// across windowed-mode eviction (Copilot review PR#7 #15) and
	// word-wrap inflation (Copilot review PR#7 #24). For empty input
	// the renderer returns 0 which doubles as the contracts/cli.md
	// "Line 0" footer sentinel (Copilot review PR#7 #6).
	current := int64(0)
	if m.renderer != nil {
		current = m.renderer.RowToLine(m.renderContext(), m.viewport.YOffset)
	}

	totalDisplay := fmt.Sprintf("%d", total)
	if m.streaming {
		totalDisplay = fmt.Sprintf("%d…", total)
	}
	line := fmt.Sprintf(" %s | %s lines | Line %d ", name, totalDisplay, current)
	style := m.theme.Footer
	// Pad to full width; lipgloss Width() handles ANSI / wide chars.
	pad := m.width - lipgloss.Width(line)
	if pad > 0 {
		line += pads(pad)
	}
	return style.Render(line)
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
