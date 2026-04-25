// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"math"
	"strings"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/knitli/spy/internal/term"
)

// LuminanceLightThreshold is the boundary above which an
// [term.Capabilities.BackgroundLuminance] reading is considered "light"
// for the purposes of auto-theme selection. Per research R6 the cutoff
// is ≥ 0.5 → light, < 0.5 → dark.
const LuminanceLightThreshold = 0.5

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

	// SearchHit highlights a non-current search match (US2). The default
	// uses reverse-video so the chroma syntax foreground stays readable
	// underneath; renderers fall back to inserting raw ANSI when the
	// theme is Mono so even no-colour terminals see a visible marker.
	SearchHit lipgloss.Style
	// SearchActive highlights the currently-selected search match
	// (driven by `n`/`N` cycling). Brighter than SearchHit so the user
	// can find the cursor at a glance.
	SearchActive lipgloss.Style
	// PromptLine is the inline `:` / `/` / `?` command-line strip drawn
	// at the bottom of the viewer when a prompt is active.
	PromptLine lipgloss.Style
}

// ThemeDark returns the built-in dark theme: monokai chroma styling
// plus high-contrast chrome.
func ThemeDark() Theme {
	return Theme{
		Name:         "dark",
		ChromaStyle:  "monokai",
		Footer:       lipgloss.NewStyle().Foreground(lipgloss.Color("#9B9B9B")),
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#1E1E1E")),
		Error:        lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true),
		SearchHit:    lipgloss.NewStyle().Background(lipgloss.Color("#444B53")).Foreground(lipgloss.Color("#FFFFFF")),
		SearchActive: lipgloss.NewStyle().Background(lipgloss.Color("#FFB454")).Foreground(lipgloss.Color("#000000")).Bold(true),
		PromptLine:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#3A3A3A")),
	}
}

// ThemeLight returns the built-in light theme: github chroma styling
// plus a softer chrome palette.
func ThemeLight() Theme {
	return Theme{
		Name:         "light",
		ChromaStyle:  "github",
		Footer:       lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")),
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#EEEEEE")),
		Error:        lipgloss.NewStyle().Foreground(lipgloss.Color("#D32F2F")).Bold(true),
		SearchHit:    lipgloss.NewStyle().Background(lipgloss.Color("#FFE082")).Foreground(lipgloss.Color("#000000")),
		SearchActive: lipgloss.NewStyle().Background(lipgloss.Color("#FFA726")).Foreground(lipgloss.Color("#000000")).Bold(true),
		PromptLine:   lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#DCDCDC")),
	}
}

// ResolveTheme picks the active Theme from the user-facing theme spec.
// The auto-detect branch reads [term.Capabilities.BackgroundLuminance]:
// luminance ≥ [LuminanceLightThreshold] selects [ThemeLight], anything
// lower (or NaN) selects [ThemeDark]. Explicit `dark` / `light` and
// named Chroma styles bypass the auto branch entirely.
//
// `noColor` forces the [Theme.Mono] flag so Chroma + lipgloss styling
// is suppressed at render time. The Mono override is applied after
// the underlying theme is chosen so the renderer still has the right
// chrome palette to fall back on if the user toggles colour back on
// later via `:set`.
func ResolveTheme(spec string, caps term.Capabilities, noColor bool) Theme {
	spec = strings.ToLower(strings.TrimSpace(spec))
	var tm Theme
	switch spec {
	case "", "auto":
		tm = autoTheme(caps)
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

// autoTheme implements the auto-detect branch of [ResolveTheme]. NaN
// luminance — the value [term.Detect] uses when the OSC 11 probe and
// the COLORFGBG fallback both came up empty — defaults to dark, which
// matches the most common terminal default and the research R6
// fallback.
func autoTheme(caps term.Capabilities) Theme {
	if !math.IsNaN(caps.BackgroundLuminance) && caps.BackgroundLuminance >= LuminanceLightThreshold {
		return ThemeLight()
	}
	return ThemeDark()
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
