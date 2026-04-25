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
