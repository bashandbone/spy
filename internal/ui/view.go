// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"

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
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if !m.showFooter {
		return m.viewport.View()
	}
	if m.commandLine.Active {
		return m.viewport.View() + "\n" + m.promptLine()
	}
	return m.viewport.View() + "\n" + m.footerLine()
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
	advisory := ""
	if m.statusAdvisory != "" {
		advisory = " | " + m.statusAdvisory
	}
	line := fmt.Sprintf(" %s | %s lines | Line %d%s ", name, totalDisplay, current, advisory)
	// Pad to full width; lipgloss Width() handles ANSI / wide chars.
	pad := m.width - lipgloss.Width(line)
	if pad > 0 {
		line += pads(pad)
	}
	if m.isMono() {
		// Mono mode: bypass lipgloss styling so the footer stays
		// ANSI-free (Copilot review PR#9 round-3 #3).
		return line
	}
	return m.theme.Footer.Render(line)
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
