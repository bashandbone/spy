// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/knitli/spy/internal/source"
)

// StatusBarMinWidth is the column threshold below which the status bar
// switches to its collapsed `<short-name> · L<current>` form. The
// 80-column boundary matches the contracts/cli.md "minimum size"
// section and the spec's quickstart Step 14.
const StatusBarMinWidth = 80

// StatusInput bundles the per-frame data the status bar consumes. The
// UI builds one each tick from its [Model] state and passes it into
// [StatusBarRender]. Keeping the input as a struct (rather than a long
// parameter list or a back-reference into ui.Model) preserves the
// render → ui dependency direction while letting the bar surface
// runtime state the static [source.Metadata] doesn't carry (current
// scroll position, the streaming/EOF transition, the highlighter
// advisory).
type StatusInput struct {
	// DisplayName is what the bar prints — typically the file basename
	// or "<stdin>" for piped input. The UI fills this from
	// [source.Source.DisplayName] so stdin sessions don't show an empty
	// path.
	DisplayName string

	// Meta is the source metadata. LineCount and PageCount drive the
	// bar's totals; -1 in LineCount is treated as the streaming sentinel.
	Meta source.Metadata

	// Viewport is the scroll region the renderer paints into. Width
	// matters for the collapse boundary; YOffset is informational only
	// (Current is the authoritative top-line indicator).
	Viewport viewport.Model

	// Width is the terminal column count. Zero falls back to
	// Viewport.Width so callers that haven't seen a [tea.WindowSizeMsg]
	// yet still produce a sensible bar.
	Width int

	// Current is the 1-based source line number visible at the top of
	// the viewport, accounting for word-wrap inflation. Zero is the
	// "Line 0" sentinel for empty input.
	Current int64

	// Page is the 1-based PDF page cursor. Zero means "not paginated";
	// non-PDF kinds always pass zero.
	Page int

	// Streaming is true while the loader is still emitting chunks. The
	// bar shows the running count plus a "…" marker until EOF flips
	// this to false.
	Streaming bool

	// Advisory is a one-line message surfaced to the right of the line
	// counter (e.g. "highlighting disabled", "search wrapped"). Empty
	// when no advisory is active. Dropped automatically when the bar
	// collapses below [StatusBarMinWidth].
	Advisory string

	// Mono suppresses lipgloss styling so no ANSI leaks into the
	// rendered frame. Mirrors [Theme.Mono] — the UI sets this when the
	// active session is in mono / no-color mode (NO_COLOR=1, TERM=dumb,
	// or `--no-color`).
	Mono bool

	// Kind selects the bar's totals format: KindPDF prints a "Page m/n"
	// indicator; everything else prints "<total> lines".
	Kind source.Kind
}

// StatusBarRender produces the rendered status-bar line. The bar is
// always single-line and exactly [StatusInput.Width] columns wide
// (padded right). Below [StatusBarMinWidth] columns the format
// collapses to `<short-name> · L<current>`.
//
// The user-controlled string fields (DisplayName, Advisory) are
// funneled through [Neutralize] at the entry point so embedded
// `\x1b` / `\x9b` bytes from a hostile filename or upstream loader
// warning cannot drive terminal protocols. Acceptance review C4.
func StatusBarRender(in StatusInput, theme Theme) string {
	in.DisplayName = Neutralize(in.DisplayName)
	in.Advisory = Neutralize(in.Advisory)

	width := in.Width
	if width <= 0 {
		width = in.Viewport.Width
	}
	if width <= 0 {
		// Nothing useful we can paint; degenerate to an empty string so
		// callers' join logic doesn't add an empty padded row.
		return ""
	}

	if width < StatusBarMinWidth {
		return renderCollapsed(in, theme, width)
	}
	return renderFull(in, theme, width)
}

// renderFull builds the wide-mode status line:
//
//	" <name> | <totals> | Line <current> | <advisory> "
//
// The result is exactly `width` columns: shorter renders are
// right-padded with spaces; longer renders are degraded — first by
// dropping the advisory, then by truncating the basename — until they
// fit, so the footer never overflows into a second visual row
// (Copilot review PR#13 #1).
//
// Trailing/leading single-space padding keeps the lipgloss background
// off the very edge of the terminal so the bar reads as a contiguous
// strip rather than text against the cell border.
func renderFull(in StatusInput, theme Theme, width int) string {
	line := buildFullLine(in.DisplayName, totalsSegment(in), currentSegment(in), in.Advisory)
	if lipgloss.Width(line) > width {
		// Drop the advisory first — it's the lowest-priority chunk and
		// dropping the whole segment is cleaner than truncating mid-word.
		line = buildFullLine(in.DisplayName, totalsSegment(in), currentSegment(in), "")
	}
	if lipgloss.Width(line) > width {
		// Still over budget: shorten the display name (its prefix is
		// most likely a long path the upstream filepath.Base already
		// trimmed once, so we trim runes from the END to preserve the
		// leading filename rather than its extension).
		shortened := truncateToWidth(in.DisplayName, maxNameWidth(width, totalsSegment(in), currentSegment(in)))
		line = buildFullLine(shortened, totalsSegment(in), currentSegment(in), "")
	}
	if lipgloss.Width(line) > width {
		// Pathological: even the bare " <… | totals | line> " doesn't
		// fit. Truncate the whole line to width so we don't overflow.
		line = truncateToWidth(line, width)
	}
	line = padToWidth(line, width)
	if in.Mono {
		return line
	}
	return theme.Footer.Render(line)
}

// buildFullLine assembles the wide-mode status line with the supplied
// segments. Empty segments are dropped so trailing " | " separators
// never appear (e.g. when the advisory is empty).
func buildFullLine(name, totals, current, advisory string) string {
	parts := make([]string, 0, 4)
	parts = append(parts, name)
	parts = append(parts, totals)
	parts = append(parts, current)
	if advisory != "" {
		parts = append(parts, advisory)
	}
	return " " + strings.Join(parts, " | ") + " "
}

// maxNameWidth returns the column budget left for the display name
// once the totals + current segments and the " | " separators + the
// leading/trailing padding spaces are accounted for. Used by
// [renderFull]'s name-truncation fallback.
func maxNameWidth(width int, totals, current string) int {
	// Layout: " <name> | <totals> | <current> "
	// Fixed cost: leading space, " | <totals>", " | <current>", trailing space.
	fixed := 1 + len(" | ") + lipgloss.Width(totals) + len(" | ") + lipgloss.Width(current) + 1
	budget := width - fixed
	if budget < 1 {
		return 1
	}
	return budget
}

// renderCollapsed builds the narrow-mode status line:
//
//	" <short> · L<current> "
//
// Drops the advisory because the column budget can't accommodate it
// without overflowing onto a second visual row.
func renderCollapsed(in StatusInput, theme Theme, width int) string {
	short := shortName(in.DisplayName)
	core := fmt.Sprintf("%s · L%d", short, in.Current)
	line := " " + core + " "
	// Trim before padding so we never overflow `width`. shortName
	// already keeps the basename to a sensible length, but a long
	// extension or a high line number can still push past the budget.
	if lipgloss.Width(line) > width {
		line = truncateToWidth(line, width)
	}
	line = padToWidth(line, width)
	if in.Mono {
		return line
	}
	return theme.Footer.Render(line)
}

// totalsSegment returns the format-appropriate totals chunk for the
// current source kind: "Page m/n" for KindPDF (with a graceful fallback
// when the PageCount is still unknown) or "<total> lines" with a
// streaming "…" marker for everything else.
func totalsSegment(in StatusInput) string {
	if in.Kind == source.KindPDF && in.Page > 0 {
		if in.Meta.PageCount > 0 {
			return fmt.Sprintf("Page %d/%d", in.Page, in.Meta.PageCount)
		}
		return fmt.Sprintf("Page %d", in.Page)
	}
	total := in.Meta.LineCount
	if total < 0 {
		// Streaming: lean on a positive Current as a fallback so the
		// bar isn't stuck at "0 lines" while the loader catches up.
		total = in.Current
	}
	if in.Streaming {
		return fmt.Sprintf("%d… lines", total)
	}
	return fmt.Sprintf("%d lines", total)
}

// currentSegment returns the "Line <N>" chunk. The 1-based numbering
// matches the gutter the renderer prints; zero is the documented "Line
// 0" sentinel for an empty buffer.
func currentSegment(in StatusInput) string {
	return fmt.Sprintf("Line %d", in.Current)
}

// shortName collapses long paths to just the basename so the
// sub-80-column bar fits. The upstream UI already runs DisplayName
// through filepath.Base for files, but defensive trimming here lets
// callers pass a raw path without surprising behavior.
func shortName(name string) string {
	if name == "" {
		return ""
	}
	if name == "<stdin>" {
		return name
	}
	return filepath.Base(name)
}

// padToWidth right-pads `s` with ASCII spaces so the printed result
// occupies exactly `width` columns. ANSI escapes embedded in `s`
// already pass through lipgloss.Width unchanged, so the padding maths
// is correct for both styled and mono renders.
func padToWidth(s string, width int) string {
	cur := lipgloss.Width(s)
	if cur >= width {
		return s
	}
	pad := strings.Repeat(" ", width-cur)
	return s + pad
}

// truncateToWidth trims `s` to at most `width` columns. Used by the
// collapsed renderer when even the short name + line counter exceed
// the column budget; we drop runes from the end and let the renderer
// pad back up.
func truncateToWidth(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	// Walk runes; stop once the cumulative width hits the cap.
	var b strings.Builder
	cur := 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if cur+w > width {
			break
		}
		cur += w
		b.WriteRune(r)
	}
	return b.String()
}
