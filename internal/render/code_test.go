// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"regexp"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// ansiPattern matches the SGR escape sequences chroma emits.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func newCodeDeps(t *testing.T, lang string, lineNumbers, wordWrap bool) Dependencies {
	t.Helper()
	return Dependencies{
		Theme:        ThemeDark(),
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		Highlighter:  highlight.New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024),
		LineNumbers:  lineNumbers,
		WordWrap:     wordWrap,
		Language:     lang,
	}
}

func loadLines(t *testing.T, raw string) *loader.LineBuffer {
	t.Helper()
	buf := loader.NewLineBuffer(0, 0, nil)
	src := strings.TrimRight(raw, "\n")
	var lines []source.Line
	for i, l := range strings.Split(src, "\n") {
		lines = append(lines, source.Line{Number: int64(i + 1), Raw: l})
	}
	buf.Append(lines)
	return buf
}

func TestKindCode_HighlightedOutputContainsANSI(t *testing.T) {
	deps := newCodeDeps(t, "go", true, false)
	r := ForKind(source.KindCode, deps)
	src := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"
	buf := loadLines(t, src)
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes in highlighted go code; got %q", out)
	}
	plain := stripANSI(out)
	for _, want := range []string{"package", "main", "import", "fmt", "func", "Println", "hi"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain (ANSI-stripped) output missing %q in %q", want, plain)
		}
	}
}

func TestKindCode_LineNumbersHonoured(t *testing.T) {
	// LineNumbers=true → gutter shows "1", "2", "3".
	deps := newCodeDeps(t, "go", true, false)
	r := ForKind(source.KindCode, deps)
	buf := loadLines(t, "alpha\nbeta\ngamma\n")
	out := stripANSI(r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	}))
	for _, n := range []string{"1", "2", "3"} {
		if !strings.Contains(out, n) {
			t.Errorf("LineNumbers=true: missing line %q in %q", n, out)
		}
	}
}

func TestKindCode_NoLineNumbersHidesGutter(t *testing.T) {
	deps := newCodeDeps(t, "go", false, false)
	r := ForKind(source.KindCode, deps)
	buf := loadLines(t, "alpha\nbeta\n")
	out := stripANSI(r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	}))
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		if line[0] >= '0' && line[0] <= '9' {
			t.Errorf("LineNumbers=false: line should not start with a digit; got %q", line)
		}
	}
}

func TestKindCode_WordWrapWraps(t *testing.T) {
	deps := newCodeDeps(t, "go", false, true)
	deps.Capabilities = term.Capabilities{Cols: 40, Rows: 24, ColorDepth: term.ColorANSI256}
	r := ForKind(source.KindCode, deps)
	long := strings.Repeat("a", 200)
	buf := loadLines(t, long+"\n")
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	rows := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if rows < 5 {
		t.Errorf("200-char line at width 40: expected >= 5 wrapped rows, got %d (out=%q)",
			rows, out)
	}
}

func TestKindCode_NoWrapPreservesLongLine(t *testing.T) {
	deps := newCodeDeps(t, "go", false, false)
	deps.Capabilities = term.Capabilities{Cols: 40, Rows: 24, ColorDepth: term.ColorANSI256}
	r := ForKind(source.KindCode, deps)
	long := strings.Repeat("a", 200)
	buf := loadLines(t, long+"\n")
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	rows := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if rows != 1 {
		t.Errorf("--no-wrap: 200-char line should emit 1 row, got %d", rows)
	}
}

func TestKindCode_EmptyBufferShowsPlaceholder(t *testing.T) {
	deps := newCodeDeps(t, "go", true, false)
	r := ForKind(source.KindCode, deps)
	out := r.Render(RenderContext{
		Buffer:       loader.NewLineBuffer(0, 0, nil),
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if !strings.Contains(out, "(empty)") {
		t.Errorf("empty buffer should show (empty) placeholder, got %q", out)
	}
}

func TestKindCode_NilBufferShowsPlaceholder(t *testing.T) {
	deps := newCodeDeps(t, "go", true, false)
	r := ForKind(source.KindCode, deps)
	out := r.Render(RenderContext{
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if !strings.Contains(out, "(empty)") {
		t.Errorf("nil buffer should show (empty) placeholder, got %q", out)
	}
}

func TestKindCode_MonoThemeBypassesANSI(t *testing.T) {
	deps := newCodeDeps(t, "go", true, false)
	deps.Theme = ThemeDark()
	deps.Theme.Mono = true
	r := ForKind(source.KindCode, deps)
	buf := loadLines(t, "func main() {}\n")
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("Mono theme should produce no ANSI escapes, got %q", out)
	}
}

func TestKindCode_NilHighlighterFallsBackToRaw(t *testing.T) {
	deps := Dependencies{
		Theme:        ThemeDark(),
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		LineNumbers:  true,
		Language:     "go",
	}
	r := ForKind(source.KindCode, deps)
	buf := loadLines(t, "func main() {}\n")
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if !strings.Contains(out, "func main() {}") {
		t.Errorf("nil Highlighter should emit raw line content, got %q", out)
	}
}

func TestKindCode_RowToLineDelegatesToTextRenderer(t *testing.T) {
	deps := newCodeDeps(t, "go", false, false)
	r := ForKind(source.KindCode, deps)
	buf := loadLines(t, "alpha\nbeta\ngamma\n")
	ctx := RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
	}
	if got := r.RowToLine(ctx, 0); got != 1 {
		t.Errorf("RowToLine(0): got %d want 1", got)
	}
	if got := r.RowToLine(ctx, 2); got != 3 {
		t.Errorf("RowToLine(2): got %d want 3", got)
	}
}

func TestKindCode_PreTokenisedLineSkipsRehighlight(t *testing.T) {
	// When source.Line.Tokens is non-nil, codeRenderer should use the
	// pre-tokenised values rather than re-lexing.
	deps := newCodeDeps(t, "go", false, false)
	r := ForKind(source.KindCode, deps).(*codeRenderer)
	preset := []source.Token{{Value: "preset-tokens"}}
	out := r.styleLine(source.Line{Raw: "ignored", Tokens: preset})
	if !strings.Contains(stripANSI(out), "preset-tokens") {
		t.Errorf("pre-tokenised line should produce its token values, got %q", stripANSI(out))
	}
}

func TestKindCode_FormatterMatchesColorDepth(t *testing.T) {
	cases := []struct {
		depth term.ColorDepth
		want  bool // expect a non-nil formatter
	}{
		{term.ColorMono, false},
		{term.ColorANSI16, true},
		{term.ColorANSI256, true},
		{term.ColorTrueColor, true},
	}
	for _, tc := range cases {
		deps := newCodeDeps(t, "go", false, false)
		deps.Capabilities.ColorDepth = tc.depth
		r := ForKind(source.KindCode, deps).(*codeRenderer)
		fm := r.formatter()
		if (fm != nil) != tc.want {
			t.Errorf("depth=%v: formatter present=%v want %v", tc.depth, fm != nil, tc.want)
		}
	}
}

// TestKindCode_ViewportWindowLimitsHighlighting verifies that when a
// viewport height is supplied, only lines within the visible window
// receive ANSI syntax highlighting while lines outside are emitted as
// raw text. This is the SC-004 viewport-bounded behaviour.
func TestKindCode_ViewportWindowLimitsHighlighting(t *testing.T) {
	deps := newCodeDeps(t, "go", false, false)
	r := ForKind(source.KindCode, deps)

	// Build a 20-line buffer; viewport shows rows 5-9 (5 lines starting at yOffset=5).
	const totalLines = 20
	var lines []string
	for i := 0; i < totalLines; i++ {
		lines = append(lines, "x := 1")
	}
	buf := loadLines(t, strings.Join(lines, "\n")+"\n")

	vp := viewport.New(80, 5)
	vp.YOffset = 5

	out := r.Render(RenderContext{
		Buffer:       buf,
		Viewport:     vp,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})

	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != totalLines {
		t.Fatalf("expected %d output rows, got %d", totalLines, len(rows))
	}

	// Rows 5–9 (0-indexed) should have ANSI; the rest should not.
	for i, row := range rows {
		hasANSI := strings.Contains(row, "\x1b[")
		wantANSI := i >= 5 && i < 10
		if hasANSI != wantANSI {
			t.Errorf("row %d: hasANSI=%v wantANSI=%v (row=%q)", i, hasANSI, wantANSI, row)
		}
	}
}

// TestKindCode_ViewportUnknownHeightRendersAll verifies that when the
// viewport height is zero (before the first WindowSizeMsg), all lines
// are treated as visible and receive syntax highlighting.
func TestKindCode_ViewportUnknownHeightRendersAll(t *testing.T) {
	deps := newCodeDeps(t, "go", false, false)
	r := ForKind(source.KindCode, deps)
	buf := loadLines(t, "a := 1\nb := 2\nc := 3\n")

	// Zero viewport → viewportKnown=false → all lines visible.
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})

	if !strings.Contains(out, "\x1b[") {
		t.Error("zero viewport: expected ANSI escapes for all lines, got none")
	}
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for i, row := range rows {
		if !strings.Contains(row, "\x1b[") {
			t.Errorf("zero viewport: row %d missing ANSI escape (row=%q)", i, row)
		}
	}
}

// TestKindCode_ViewportCacheReusesFormattedOutput verifies that the
// per-line cache in codeRenderer avoids re-invoking styleLine for lines
// that have already been rendered in the same renderer's lifetime.
func TestKindCode_ViewportCacheReusesFormattedOutput(t *testing.T) {
	deps := newCodeDeps(t, "go", false, false)
	cr := ForKind(source.KindCode, deps).(*codeRenderer)
	buf := loadLines(t, "func main() {}\n")

	vp := viewport.New(80, 10)
	ctx := RenderContext{
		Buffer:       buf,
		Viewport:     vp,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	}

	// First render populates the cache.
	out1 := cr.Render(ctx)
	if cr.cache == nil {
		t.Fatal("cache should be non-nil after first render")
	}
	if len(cr.cache) == 0 {
		t.Fatal("cache should have at least one entry after first render")
	}

	// Second render with the same context should produce identical output
	// (served from cache).
	out2 := cr.Render(ctx)
	if out1 != out2 {
		t.Errorf("second render produced different output:\nfirst:  %q\nsecond: %q", out1, out2)
	}
}
