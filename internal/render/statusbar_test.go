// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/knitli/spy/internal/source"
)

func TestStatusBar_StandardFormat(t *testing.T) {
	vp := viewport.New(80, 23)
	in := StatusInput{
		DisplayName: "hello.go",
		Meta:        source.Metadata{LineCount: 42},
		Viewport:    vp,
		Width:       80,
		Current:     7,
	}
	out := StatusBarRender(in, ThemeDark())
	if !strings.Contains(out, "hello.go") {
		t.Errorf("expected display name in output: %q", out)
	}
	if !strings.Contains(out, "42 lines") {
		t.Errorf("expected total-line count in output: %q", out)
	}
	if !strings.Contains(out, "Line 7") {
		t.Errorf("expected current-line indicator: %q", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("non-streaming output should not show '…': %q", out)
	}
}

func TestStatusBar_StreamingShowsEllipsis(t *testing.T) {
	vp := viewport.New(80, 23)
	in := StatusInput{
		DisplayName: "big.log",
		Meta:        source.Metadata{LineCount: -1},
		Viewport:    vp,
		Width:       100,
		Current:     1,
		Streaming:   true,
	}
	out := StatusBarRender(in, ThemeDark())
	if !strings.Contains(out, "…") {
		t.Errorf("streaming status bar should include '…' indicator: %q", out)
	}
}

func TestStatusBar_PDFShowsPageIndicator(t *testing.T) {
	vp := viewport.New(120, 23)
	in := StatusInput{
		DisplayName: "manual.pdf",
		Meta:        source.Metadata{PageCount: 5, LineCount: 240},
		Viewport:    vp,
		Width:       120,
		Current:     12,
		Page:        2,
		Kind:        source.KindPDF,
	}
	out := StatusBarRender(in, ThemeDark())
	if !strings.Contains(out, "Page 2/5") {
		t.Errorf("PDF status bar should show 'Page m/n': %q", out)
	}
	if !strings.Contains(out, "manual.pdf") {
		t.Errorf("PDF status bar should show display name: %q", out)
	}
}

func TestStatusBar_PDFWithUnknownPageCount(t *testing.T) {
	// PageCount == 0 is the loader's "not yet known" sentinel; the
	// status bar must degrade gracefully rather than print "Page 1/0".
	vp := viewport.New(120, 23)
	in := StatusInput{
		DisplayName: "stream.pdf",
		Meta:        source.Metadata{PageCount: 0, LineCount: 100},
		Viewport:    vp,
		Width:       120,
		Current:     3,
		Page:        1,
		Kind:        source.KindPDF,
	}
	out := StatusBarRender(in, ThemeDark())
	if strings.Contains(out, "Page 1/0") {
		t.Errorf("status bar should not print 'Page 1/0' when total is unknown: %q", out)
	}
	if !strings.Contains(out, "Page 1") {
		t.Errorf("status bar should still show current page: %q", out)
	}
}

func TestStatusBar_SubEightyColumnCollapse(t *testing.T) {
	// Below 80 columns the contract collapses the bar to "<short> · L<N>".
	vp := viewport.New(40, 23)
	in := StatusInput{
		DisplayName: "/very/long/path/to/main.go",
		Meta:        source.Metadata{LineCount: 120},
		Viewport:    vp,
		Width:       40,
		Current:     5,
		Mono:        true, // strip ANSI for easier substring assertions
	}
	out := StatusBarRender(in, ThemeDark())
	if strings.Contains(out, " | ") {
		t.Errorf("collapsed status bar should not use ' | ' separators: %q", out)
	}
	if !strings.Contains(out, "·") {
		t.Errorf("collapsed status bar should use '·' separator: %q", out)
	}
	if !strings.Contains(out, "L5") {
		t.Errorf("collapsed status bar should print 'L<current>': %q", out)
	}
	// Width budget honoured.
	if widthOf(out) > 40 {
		t.Errorf("collapsed status bar must fit within width=40, got %d cols", widthOf(out))
	}
}

func TestStatusBar_AdvisoryAppendedWhenSpaceAllows(t *testing.T) {
	vp := viewport.New(120, 23)
	in := StatusInput{
		DisplayName: "src.go",
		Meta:        source.Metadata{LineCount: 30},
		Viewport:    vp,
		Width:       120,
		Current:     1,
		Advisory:    "highlighting disabled",
	}
	out := StatusBarRender(in, ThemeDark())
	if !strings.Contains(out, "highlighting disabled") {
		t.Errorf("advisory should appear in wide status bar: %q", out)
	}
}

func TestStatusBar_AdvisoryDroppedInCollapsedMode(t *testing.T) {
	// Below 80 columns we drop the advisory rather than truncate awkwardly.
	vp := viewport.New(40, 23)
	in := StatusInput{
		DisplayName: "src.go",
		Meta:        source.Metadata{LineCount: 30},
		Viewport:    vp,
		Width:       40,
		Current:     1,
		Advisory:    "highlighting disabled",
		Mono:        true,
	}
	out := StatusBarRender(in, ThemeDark())
	if strings.Contains(out, "highlighting disabled") {
		t.Errorf("collapsed bar should drop advisory rather than overflow: %q", out)
	}
}

func TestStatusBar_WideRenderNeverExceedsWidth(t *testing.T) {
	// Copilot review PR#13 #1: long filenames + advisories must NOT
	// push the footer past `width`. Confirm the renderer degrades
	// (drops advisory, then truncates name) rather than overflows.
	cases := []struct {
		name        string
		displayName string
		advisory    string
		width       int
	}{
		{
			name:        "long advisory triggers drop",
			displayName: "main.go",
			advisory:    "highlighting disabled because the file exceeds the configured cap",
			width:       80,
		},
		{
			name:        "long display name triggers truncation",
			displayName: "averyveryveryveryveryveryverylongbasename-with-suffix.markdown",
			advisory:    "",
			width:       80,
		},
		{
			name:        "both long: advisory dropped + name truncated",
			displayName: "averyveryveryveryveryveryverylongbasename-with-suffix.markdown",
			advisory:    "highlighting disabled because the file exceeds the configured cap",
			width:       80,
		},
		{
			name:        "extreme: width barely fits the totals + current",
			displayName: "main.go",
			advisory:    "",
			width:       80, // smallest wide-mode width
		},
		{
			// Wide-character regression: a CJK-heavy filename plus a
			// long emoji-laden advisory must still fit. lipgloss.Width
			// counts CJK as 2 cols and emoji as 2; rune count is wrong.
			name:        "wide chars: CJK basename + emoji advisory",
			displayName: "メインプログラム-超長いファイル名.go",
			advisory:    "🔥 highlighting disabled 🔥",
			width:       80,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vp := viewport.New(tc.width, 23)
			in := StatusInput{
				DisplayName: tc.displayName,
				Meta:        source.Metadata{LineCount: 1234},
				Viewport:    vp,
				Width:       tc.width,
				Current:     7,
				Advisory:    tc.advisory,
				Mono:        true, // strip ANSI for the width check
			}
			out := StatusBarRender(in, ThemeDark())
			if widthOf(out) > tc.width {
				t.Errorf("status bar width %d > budget %d: %q", widthOf(out), tc.width, out)
			}
			if widthOf(out) < tc.width {
				t.Errorf("status bar width %d < budget %d (missing right-pad): %q",
					widthOf(out), tc.width, out)
			}
			// Even when degrading, the totals + current segments must
			// remain visible so the user still sees the line counter.
			if !strings.Contains(out, "1234 lines") {
				t.Errorf("totals segment missing under truncation: %q", out)
			}
			if !strings.Contains(out, "Line 7") {
				t.Errorf("current segment missing under truncation: %q", out)
			}
		})
	}
}

func TestStatusBar_MonoSuppressesANSI(t *testing.T) {
	vp := viewport.New(80, 23)
	in := StatusInput{
		DisplayName: "x.txt",
		Meta:        source.Metadata{LineCount: 1},
		Viewport:    vp,
		Width:       80,
		Current:     1,
		Mono:        true,
	}
	out := StatusBarRender(in, ThemeDark())
	if strings.Contains(out, "\x1b[") {
		t.Errorf("mono status bar must not emit ANSI escapes: %q", out)
	}
}

// widthOf returns the rendered terminal-column width of the status
// bar line. Uses [lipgloss.Width] so wide characters (CJK, emoji) and
// combining marks are counted correctly, matching the production
// width budget enforced by [renderFull] / [padToWidth] (Copilot
// review PR#13 round-3 #6 — rune count was wrong for non-ASCII
// content). lipgloss.Width also strips ANSI escapes, so the helper
// works on both styled and mono renders.
func widthOf(s string) int {
	return lipgloss.Width(strings.TrimRight(s, "\n"))
}
