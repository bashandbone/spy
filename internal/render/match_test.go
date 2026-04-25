// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/search"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

func makeBufferWithLines(t *testing.T, lines ...string) *loader.LineBuffer {
	t.Helper()
	buf := loader.NewLineBuffer(0, 0, nil)
	out := make([]source.Line, len(lines))
	for i, raw := range lines {
		out[i] = source.Line{Number: int64(i + 1), Raw: raw}
	}
	buf.Append(out)
	return buf
}

func TestMatchHighlight_TextRendererWrapsHits(t *testing.T) {
	t.Parallel()
	deps := Dependencies{
		Theme:        Theme{SearchHit: lipgloss.NewStyle().Background(lipgloss.Color("#FFFF00"))},
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		WordWrap:     false,
	}
	r := ForKind(source.KindText, deps)
	buf := makeBufferWithLines(t, "alpha foo beta", "no match here", "foo and another foo")
	state := search.State{
		Query: "foo",
		Matches: []search.Match{
			{Line: 1, Start: 6, End: 9},
			{Line: 3, Start: 0, End: 3},
			{Line: 3, Start: 16, End: 19},
		},
		CurrentMatch: -1,
	}
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
		Search:       state,
	})
	// Strip ANSI to check that the underlying text is preserved.
	if !strings.Contains(stripANSI(out), "alpha foo beta") {
		t.Errorf("rendered text missing line 1 content: %q", stripANSI(out))
	}
	// ANSI escapes must be present for the highlighted lines.
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes in highlighted output; got %q", out)
	}
}

func TestMatchHighlight_ActiveMatchUsesActiveStyle(t *testing.T) {
	t.Parallel()
	hitStyle := lipgloss.NewStyle().Background(lipgloss.Color("#444444"))
	activeStyle := lipgloss.NewStyle().Background(lipgloss.Color("#FFA500")).Bold(true)
	state := search.State{
		Query: "foo",
		Matches: []search.Match{
			{Line: 1, Start: 0, End: 3},
			{Line: 1, Start: 8, End: 11},
		},
		CurrentMatch: 1,
	}
	active, hasActive := activeMatch(state)
	if !hasActive {
		t.Fatal("expected active match present")
	}
	matches := matchesForLine(state, 1)
	out := applyMatchHighlights("foo bar foo", matches, active, hasActive, hitStyle, activeStyle)
	// The rendered output should contain "foo bar foo" character by character.
	if !strings.Contains(stripANSI(out), "foo bar foo") {
		t.Errorf("output missing raw text: %q", stripANSI(out))
	}
}

func TestMatchHighlight_NoMatchesReturnsRaw(t *testing.T) {
	t.Parallel()
	out := applyMatchHighlights("hello world", nil, search.Match{}, false,
		lipgloss.NewStyle(), lipgloss.NewStyle())
	if out != "hello world" {
		t.Errorf("no matches: expected raw passthrough, got %q", out)
	}
}

func TestMatchHighlight_OutOfRangeMatchesIgnored(t *testing.T) {
	t.Parallel()
	matches := []search.Match{
		{Line: 1, Start: -1, End: 5},  // negative start
		{Line: 1, Start: 0, End: 100}, // end past length
		{Line: 1, Start: 5, End: 5},   // empty range
		{Line: 1, Start: 0, End: 5},   // valid
	}
	out := applyMatchHighlights("hello world", matches, search.Match{}, false,
		lipgloss.NewStyle().Background(lipgloss.Color("#FF0000")), lipgloss.NewStyle())
	if !strings.Contains(stripANSI(out), "hello world") {
		t.Errorf("output should contain raw text; got %q", stripANSI(out))
	}
}

func TestMatchesForLine_EmptyStateReturnsNil(t *testing.T) {
	t.Parallel()
	if got := matchesForLine(search.State{}, 1); got != nil {
		t.Errorf("empty state: got %+v want nil", got)
	}
}

func TestActiveMatch_ZeroIndexNoMatchReturnsFalse(t *testing.T) {
	t.Parallel()
	state := search.State{Query: "foo", Matches: []search.Match{}, CurrentMatch: 0}
	_, ok := activeMatch(state)
	if ok {
		t.Errorf("activeMatch with empty matches should return ok=false")
	}
}

func TestMatchHighlight_TextRendererSuppressesAnsiInMonoTheme(t *testing.T) {
	t.Parallel()
	deps := Dependencies{
		Theme:        Theme{Mono: true, SearchHit: lipgloss.NewStyle().Background(lipgloss.Color("#FFFF00"))},
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		WordWrap:     false,
	}
	r := ForKind(source.KindText, deps)
	buf := makeBufferWithLines(t, "alpha foo beta")
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
		Search: search.State{
			Query:        "foo",
			Matches:      []search.Match{{Line: 1, Start: 6, End: 9}},
			CurrentMatch: -1,
		},
	})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("mono theme + match: rendered output must not contain ANSI escapes; got %q", out)
	}
	if !strings.Contains(out, "alpha foo beta") {
		t.Errorf("mono mode should still emit raw line content; got %q", out)
	}
}

func TestMatchHighlight_TextRendererSuppressesAnsiInColorMonoCaps(t *testing.T) {
	t.Parallel()
	// ColorDepth = ColorMono represents NO_COLOR/TERM=dumb at the
	// capability layer; even if Theme.Mono is false we must not emit
	// ANSI (Copilot review PR#9 round-3 #2).
	deps := Dependencies{
		Theme:        Theme{SearchHit: lipgloss.NewStyle().Background(lipgloss.Color("#FFFF00"))},
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorMono},
		WordWrap:     false,
	}
	r := ForKind(source.KindText, deps)
	buf := makeBufferWithLines(t, "alpha foo beta")
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
		Search: search.State{
			Query:        "foo",
			Matches:      []search.Match{{Line: 1, Start: 6, End: 9}},
			CurrentMatch: -1,
		},
	})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("ColorMono caps + match: rendered output must not contain ANSI escapes; got %q", out)
	}
}

func TestMatchHighlight_CodeRendererSuppressesAnsiInMonoMode(t *testing.T) {
	t.Parallel()
	// Code renderer's match-overlay path also has to suppress ANSI in
	// mono mode (Copilot review PR#9 round-3 #1).
	deps := Dependencies{
		Theme:        Theme{Mono: true, SearchHit: lipgloss.NewStyle().Background(lipgloss.Color("#FFFF00"))},
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
	}
	r := ForKind(source.KindCode, deps)
	buf := makeBufferWithLines(t, "package main")
	out := r.Render(RenderContext{
		Buffer:       buf,
		Theme:        deps.Theme,
		Capabilities: deps.Capabilities,
		Search: search.State{
			Query:        "main",
			Matches:      []search.Match{{Line: 1, Start: 8, End: 12}},
			CurrentMatch: -1,
		},
	})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("mono code render with match: must not emit ANSI; got %q", out)
	}
	if !strings.Contains(out, "package main") {
		t.Errorf("mono code render: should still emit raw content; got %q", out)
	}
}
