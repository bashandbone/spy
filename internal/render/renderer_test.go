// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"strings"
	"testing"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

func TestForKind_DispatchesEveryKind(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true}
	cases := []struct {
		kind source.Kind
		name string
	}{
		{source.KindUnknown, "Unknown"},
		{source.KindCode, "Code"},
		{source.KindMarkdown, "Markdown"},
		{source.KindText, "Text"},
		{source.KindPDF, "PDF"},
		{source.KindImage, "Image"},
		{source.KindBinary, "Binary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ForKind(tc.kind, deps)
			if r == nil {
				t.Fatalf("ForKind(%v) returned nil", tc.kind)
			}
		})
	}
}

func TestKindText_PassthroughLineNumbers(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true}
	r := ForKind(source.KindText, deps)
	buf := loader.NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{
		{Number: 1, Raw: "alpha"},
		{Number: 2, Raw: "beta"},
		{Number: 3, Raw: "gamma"},
	})
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
	})
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") || !strings.Contains(out, "gamma") {
		t.Errorf("rendered output missing lines: %q", out)
	}
	// Line numbers in the gutter — the foundational text renderer must
	// at minimum show the line digits so the user can navigate.
	for _, n := range []string{"1", "2", "3"} {
		if !strings.Contains(out, n) {
			t.Errorf("rendered output missing line number %q: %q", n, out)
		}
	}
}

func TestKindText_LineNumbersDisabled(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: false}
	r := ForKind(source.KindText, deps)
	buf := loader.NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{{Number: 1, Raw: "hello"}})
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
	})
	if strings.HasPrefix(strings.TrimSpace(out), "1") {
		t.Errorf("LineNumbers=false: rendered output still leads with a digit: %q", out)
	}
}

func TestUnsupportedKind_StubFrame(t *testing.T) {
	// PDF/Image/Code/Markdown stubs all produce a "pending USx" notice
	// in Phase 2 — they're filled in by their respective story phases.
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true}
	for _, k := range []source.Kind{source.KindPDF, source.KindImage} {
		r := ForKind(k, deps)
		out := r.Render(RenderContext{
			Theme:        deps.Theme,
			Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		})
		if !strings.Contains(strings.ToLower(out), "pending") {
			t.Errorf("Kind=%v stub should mention 'pending' in foundational, got %q", k, out)
		}
	}
}

func TestKindText_EmptyInputShowsPlaceholder(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true}
	r := ForKind(source.KindText, deps)
	out := r.Render(RenderContext{
		Buffer:       loader.NewLineBuffer(0, 0, nil),
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
	})
	if !strings.Contains(out, "(empty)") {
		t.Errorf("empty buffer: expected (empty) placeholder, got %q", out)
	}
}

func TestKindText_NilBufferShowsPlaceholder(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true}
	r := ForKind(source.KindText, deps)
	out := r.Render(RenderContext{
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
	})
	if !strings.Contains(out, "(empty)") {
		t.Errorf("nil buffer: expected (empty) placeholder, got %q", out)
	}
}

func TestKindBinary_StubFrame(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true}
	r := ForKind(source.KindBinary, deps)
	out := r.Render(RenderContext{
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
	})
	if !strings.Contains(strings.ToLower(out), "binary") {
		t.Errorf("KindBinary frame should mention 'binary', got %q", out)
	}
}

func TestKindText_RowToLine_NoWrap(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true, WordWrap: false}
	r := ForKind(source.KindText, deps)
	buf := loader.NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{
		{Number: 1, Raw: "alpha"},
		{Number: 2, Raw: "beta"},
		{Number: 3, Raw: "gamma"},
	})
	ctx := RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
	}
	cases := []struct {
		row  int
		want int64
	}{
		{0, 1}, {1, 2}, {2, 3},
		// Out-of-range rows clamp to the last source line.
		{99, 3},
	}
	for _, tc := range cases {
		if got := r.RowToLine(ctx, tc.row); got != tc.want {
			t.Errorf("no-wrap RowToLine(row=%d): got %d want %d",
				tc.row, got, tc.want)
		}
	}
}

func TestKindText_RowToLine_WrapsCountVisualRows(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: false, WordWrap: true}
	r := ForKind(source.KindText, deps)
	buf := loader.NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{
		{Number: 1, Raw: strings.Repeat("a", 100)}, // wraps at width 40 → 3 rows
		{Number: 2, Raw: "short"},                  // 1 row
		{Number: 3, Raw: strings.Repeat("b", 80)},  // 2 rows
	})
	ctx := RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 40, Rows: 24},
	}
	// Visual row layout (no gutter): rows 0-2 = line 1, row 3 = line 2,
	// rows 4-5 = line 3.
	cases := []struct {
		row  int
		want int64
	}{
		{0, 1}, {1, 1}, {2, 1},
		{3, 2},
		{4, 3}, {5, 3},
	}
	for _, tc := range cases {
		if got := r.RowToLine(ctx, tc.row); got != tc.want {
			t.Errorf("wrap RowToLine(row=%d): got %d want %d",
				tc.row, got, tc.want)
		}
	}
}

func TestKindText_RowToLine_EmptyBufferReturnsZero(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true, WordWrap: true}
	r := ForKind(source.KindText, deps)
	if got := r.RowToLine(RenderContext{}, 0); got != 0 {
		t.Errorf("empty ctx: RowToLine should return 0, got %d", got)
	}
	buf := loader.NewLineBuffer(0, 0, nil)
	got := r.RowToLine(RenderContext{Buffer: buf}, 0)
	if got != 0 {
		t.Errorf("empty buffer: RowToLine should return 0, got %d", got)
	}
}

func TestKindText_WordWrapWraps(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true, WordWrap: true}
	r := ForKind(source.KindText, deps)
	buf := loader.NewLineBuffer(0, 0, nil)
	long := strings.Repeat("a", 200)
	buf.Append([]source.Line{{Number: 1, Raw: long}})
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 40, Rows: 24},
	})
	rows := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if rows < 5 {
		t.Errorf("200-char line at width 40 should produce >= 5 wrapped rows, got %d (out=%q)",
			rows, out)
	}
}

func TestKindText_NoWrapPreservesLongLine(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true, WordWrap: false}
	r := ForKind(source.KindText, deps)
	buf := loader.NewLineBuffer(0, 0, nil)
	long := strings.Repeat("a", 200)
	buf.Append([]source.Line{{Number: 1, Raw: long}})
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 40, Rows: 24},
	})
	rows := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if rows != 1 {
		t.Errorf("--no-wrap: 200-char line should emit 1 row, got %d", rows)
	}
	if !strings.Contains(out, long) {
		t.Errorf("--no-wrap should preserve full line content")
	}
}

func TestKindText_BlankLineDoesNotCollapseUnderWrap(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), LineNumbers: true, WordWrap: true}
	r := ForKind(source.KindText, deps)
	buf := loader.NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{
		{Number: 1, Raw: "alpha"},
		{Number: 2, Raw: ""},
		{Number: 3, Raw: "gamma"},
	})
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
	})
	rows := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if rows != 3 {
		t.Errorf("blank middle line must stay as one row; got %d rows in %q", rows, out)
	}
}

// --- Theme ---

func TestThemeDarkVsLight(t *testing.T) {
	d := ThemeDark()
	l := ThemeLight()
	if d.Name == l.Name {
		t.Errorf("dark and light themes share Name=%q", d.Name)
	}
	if d.ChromaStyle == "" || l.ChromaStyle == "" {
		t.Errorf("both themes must have a chroma style: dark=%q light=%q",
			d.ChromaStyle, l.ChromaStyle)
	}
}

func TestThemeFallback_UnknownStyleResolvesToDefault(t *testing.T) {
	// Any unknown chroma-style name must NOT crash; ResolveTheme returns
	// a real Theme by falling back to the named built-in theme.
	tm := resolveByName("does-not-exist")
	if tm.ChromaStyle == "" {
		t.Errorf("unknown style should fall back to a real theme")
	}
}
