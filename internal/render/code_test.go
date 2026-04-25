// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"regexp"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"

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
