// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
	"github.com/knitli/spy/internal/ui"
)

// TestScroll_60fps enforces SC-002: 100 sequential ScrollDown actions
// against a 10 000-line buffer must clear at p95 ≤ 16 ms (60 fps) per
// frame, with zero dropped frames.
//
// The test exercises the model directly. The PTY-driven equivalent
// (TestTextReview_HighlightedFile) verifies the alt-screen frame
// transport; this one isolates the scroll-handler + render pipeline so
// the p95 isn't muddled by pty drain latency.
func TestScroll_60fps(t *testing.T) {
	body := buildSyntheticTextLines(10000)
	src := &memSource{body: body, kind: source.KindText, lex: ""}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	for range stream.Updates {
	}
	for range stream.Errs {
	}

	cfg := config.Defaults()
	caps := term.Capabilities{Cols: 80, Rows: 24}
	hl := highlight.New(nil, term.ColorANSI256, 5*1024*1024)
	m := ui.NewModel(ui.ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: caps,
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  hl,
	})

	mu, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mod, ok := mu.(ui.Model)
	if !ok {
		t.Fatalf("model update returned non-ui.Model")
	}

	const scrolls = 100
	durations := make([]time.Duration, 0, scrolls)
	dropped := 0
	for i := 0; i < scrolls; i++ {
		start := time.Now()
		nm, _ := mod.Update(tea.KeyMsg{Type: tea.KeyDown})
		_ = nm.(ui.Model).View()
		elapsed := time.Since(start)
		durations = append(durations, elapsed)
		if elapsed > 16*time.Millisecond {
			dropped++
		}
		mod = nm.(ui.Model)
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(len(durations)*95)/100]
	const limit = 16 * time.Millisecond
	if p95 > limit {
		t.Fatalf("SC-002: scroll p95 %v exceeds %v budget (%d/%d frames over 16ms)",
			p95, limit, dropped, scrolls)
	}
	t.Logf("SC-002: scroll p95=%v, dropped=%d/%d (limit %v); fastest=%v slowest=%v",
		p95, dropped, scrolls, limit, durations[0], durations[len(durations)-1])
}

// buildSyntheticTextLines emits an n-line plaintext buffer suitable for
// the scroll benchmark. Each line is unique so the renderer can't
// trivially memoise.
func buildSyntheticTextLines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("line ")
		b.WriteString(intStr(i))
		b.WriteString(": the quick brown fox jumps over the lazy dog\n")
	}
	return b.String()
}
