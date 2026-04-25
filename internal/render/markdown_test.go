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

func newMarkdownDeps(_ *testing.T, lineNumbers bool) Dependencies {
	return Dependencies{
		Theme:        ThemeDark(),
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		LineNumbers:  lineNumbers,
		WordWrap:     true,
	}
}

func loadMarkdown(t *testing.T, body string) *loader.LineBuffer {
	t.Helper()
	buf := loader.NewLineBuffer(0, 0, nil)
	src := strings.TrimRight(body, "\n")
	var lines []source.Line
	for i, l := range strings.Split(src, "\n") {
		lines = append(lines, source.Line{Number: int64(i + 1), Raw: l})
	}
	buf.Append(lines)
	return buf
}

func TestKindMarkdown_RendersHeadings(t *testing.T) {
	deps := newMarkdownDeps(t, false)
	r := ForKind(source.KindMarkdown, deps)
	src := "# Heading One\n\nA short paragraph.\n\n## Heading Two\n"
	buf := loadMarkdown(t, src)
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	plain := stripANSI(out)
	for _, want := range []string{"Heading One", "short paragraph", "Heading Two"} {
		if !strings.Contains(plain, want) {
			t.Errorf("rendered markdown missing %q in %q", want, plain)
		}
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes from glamour rendering; got %q", out)
	}
}

func TestKindMarkdown_RendersList(t *testing.T) {
	deps := newMarkdownDeps(t, false)
	r := ForKind(source.KindMarkdown, deps)
	src := "Top:\n\n- alpha\n- beta\n- gamma\n"
	buf := loadMarkdown(t, src)
	out := stripANSI(r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	}))
	for _, item := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(out, item) {
			t.Errorf("rendered list missing %q in %q", item, out)
		}
	}
}

func TestKindMarkdown_RendersCodeBlock(t *testing.T) {
	deps := newMarkdownDeps(t, false)
	r := ForKind(source.KindMarkdown, deps)
	src := "Run this:\n\n```go\nfunc main() {}\n```\n"
	buf := loadMarkdown(t, src)
	out := stripANSI(r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	}))
	if !strings.Contains(out, "func main()") {
		t.Errorf("rendered code block missing fenced content: %q", out)
	}
}

func TestKindMarkdown_EmptyBufferShowsPlaceholder(t *testing.T) {
	deps := newMarkdownDeps(t, false)
	r := ForKind(source.KindMarkdown, deps)
	out := r.Render(RenderContext{
		Buffer:       loader.NewLineBuffer(0, 0, nil),
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if !strings.Contains(out, "(empty)") {
		t.Errorf("empty buffer should show (empty), got %q", out)
	}
}

func TestKindMarkdown_NilBufferShowsPlaceholder(t *testing.T) {
	deps := newMarkdownDeps(t, false)
	r := ForKind(source.KindMarkdown, deps)
	out := r.Render(RenderContext{
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if !strings.Contains(out, "(empty)") {
		t.Errorf("nil buffer should show (empty), got %q", out)
	}
}

func TestKindMarkdown_MonoFallsBackToText(t *testing.T) {
	deps := newMarkdownDeps(t, false)
	deps.Theme.Mono = true
	r := ForKind(source.KindMarkdown, deps)
	src := "# Heading\n\nbody\n"
	buf := loadMarkdown(t, src)
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("Mono theme should produce no ANSI escapes; got %q", out)
	}
	if !strings.Contains(out, "Heading") {
		t.Errorf("Mono fallback should still emit raw heading text: %q", out)
	}
}

func TestKindMarkdown_ColorMonoCapsFallBackToText(t *testing.T) {
	// NO_COLOR / TERM=dumb sets caps.ColorDepth = ColorMono even when
	// the theme isn't explicitly Mono. Glamour must be bypassed in
	// that case so we don't emit ANSI to a non-colour terminal
	// (Copilot review PR#8 #4).
	deps := newMarkdownDeps(t, false)
	deps.Capabilities.ColorDepth = term.ColorMono
	if deps.Theme.Mono {
		t.Fatalf("test prerequisite: Theme.Mono must be false to isolate the caps gate")
	}
	r := ForKind(source.KindMarkdown, deps)
	src := "# Heading\n\nbody\n"
	buf := loadMarkdown(t, src)
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("ColorMono caps should suppress ANSI even when Theme.Mono is false; got %q", out)
	}
	if !strings.Contains(out, "Heading") {
		t.Errorf("ColorMono fallback should still emit raw heading text: %q", out)
	}
}

func TestKindMarkdown_LineNumbersAddGutter(t *testing.T) {
	deps := newMarkdownDeps(t, true)
	r := ForKind(source.KindMarkdown, deps)
	src := "# Heading\n\nbody\n"
	buf := loadMarkdown(t, src)
	out := stripANSI(r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	}))
	// Each non-empty rendered line should have a leading gutter of
	// spaces (we don't check every line; one non-empty match suffices).
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if line[0] != ' ' {
			t.Errorf("LineNumbers=true: rendered line should be padded with a gutter; got %q", line)
		}
		break
	}
}

func TestGlamourStyleForTheme(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"dark", "dark"},
		{"light", "light"},
		{"auto", "dark"},
		{"monokai", "dark"},
	}
	for _, tc := range cases {
		got := glamourStyleForTheme(Theme{Name: tc.name})
		if got != tc.want {
			t.Errorf("glamourStyleForTheme(%q): got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestKindMarkdown_RowToLineDelegates(t *testing.T) {
	deps := newMarkdownDeps(t, false)
	r := ForKind(source.KindMarkdown, deps)
	buf := loadMarkdown(t, "alpha\nbeta\ngamma\n")
	ctx := RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
	}
	if got := r.RowToLine(ctx, 0); got < 1 {
		t.Errorf("RowToLine(0): got %d want >= 1", got)
	}
}
