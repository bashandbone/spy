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
// viewport plus a single-line footer; the full status bar (US6) lands
// in T098–T100.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	footer := m.footerLine()
	return m.viewport.View() + "\n" + footer
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
	current := m.viewport.YOffset + 1

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
