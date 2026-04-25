// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"strings"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/knitli/spy/internal/term"
)

// Theme is the active styling profile for a viewer session. It bundles
// the Chroma style name (used by the highlighter), the lipgloss styles
// for chrome (status bar, error banner, footer), and the Mono override
// that disables colour entirely.
type Theme struct {
	Name        string // "dark" | "light" | "<chroma-style>"
	ChromaStyle string
	Mono        bool

	Footer lipgloss.Style
	Status lipgloss.Style
	Error  lipgloss.Style
}

// ThemeDark returns the built-in dark theme: monokai chroma styling
// plus high-contrast chrome.
func ThemeDark() Theme {
	return Theme{
		Name:        "dark",
		ChromaStyle: "monokai",
		Footer:      lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")),
		Status:      lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#1E1E1E")),
		Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true),
	}
}

// ThemeLight returns the built-in light theme: github chroma styling
// plus a softer chrome palette.
func ThemeLight() Theme {
	return Theme{
		Name:        "light",
		ChromaStyle: "github",
		Footer:      lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")),
		Status:      lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#EEEEEE")),
		Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("#D32F2F")).Bold(true),
	}
}

// ResolveTheme picks the active Theme from the user-facing theme spec.
// Per T029 the auto-detect branch falls back to dark until US3 wires
// the OSC 11 luminance probe in T066.
//
// The supplied capabilities give US3 the luminance signal; ignored in
// Phase 2. NoColor forces the [Theme.Mono] flag so chroma + lipgloss
// styling is suppressed at render time.
func ResolveTheme(spec string, _ term.Capabilities, noColor bool) Theme {
	spec = strings.ToLower(strings.TrimSpace(spec))
	var tm Theme
	switch spec {
	case "", "auto":
		tm = ThemeDark()
	case "dark":
		tm = ThemeDark()
	case "light":
		tm = ThemeLight()
	default:
		tm = resolveByName(spec)
	}
	if noColor {
		tm.Mono = true
	}
	return tm
}

// resolveByName looks up a Chroma style name and produces a Theme that
// uses it. Unknown style names fall back to the dark theme — the
// highlighter then uses the dark chroma style as a safety net.
func resolveByName(name string) Theme {
	if styles.Get(name) == nil {
		// Unknown style — fall back to dark.
		return ThemeDark()
	}
	tm := ThemeDark()
	tm.Name = name
	tm.ChromaStyle = name
	return tm
}
