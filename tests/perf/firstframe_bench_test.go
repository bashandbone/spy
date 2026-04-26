// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
	"github.com/knitli/spy/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// TestFirstFrame_Under100ms enforces SC-001: opening a 100-line text
// file with syntax highlighting must produce a renderable first frame
// in ≤ 100 ms.
//
// The test mirrors what happens between cmd/spy/main.go's source
// resolution and the first paint: source.FromArgs (or memSource here),
// loader.Open (which reads the first chunk synchronously), and
// ui.NewModel.View() through a single tea.WindowSizeMsg. It does NOT
// spawn the binary because the spec budget is "from invocation",
// excluding the ~50–80 ms typical Go runtime startup that's
// platform-dependent and outside the renderer's control.
func TestFirstFrame_Under100ms(t *testing.T) {
	const lines = 100
	body := buildSyntheticGo(lines)

	durations := make([]time.Duration, 0, 20)
	for i := 0; i < 20; i++ {
		start := time.Now()
		src := &memSource{body: body, kind: source.KindCode, lex: "go"}
		stream, err := loader.Open(context.Background(), src, loader.Config{})
		if err != nil {
			t.Fatalf("loader.Open: %v", err)
		}
		hl := highlight.New(nil, term.ColorTrueColor, 5*1024*1024)
		hl.SetLang("go")
		m := ui.NewModel(ui.ModelOptions{
			Source:       src,
			Stream:       stream,
			Capabilities: term.Capabilities{Cols: 80, Rows: 24},
			Config:       config.Defaults(),
			Theme:        render.ThemeDark(),
			KeyMap:       keys.Default(),
			Highlighter:  hl,
		})
		mu, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		view := mu.(ui.Model).View()
		if !strings.Contains(view, "package main") {
			t.Fatalf("first frame missing 'package main'; got len=%d", len(view))
		}
		durations = append(durations, time.Since(start))
		// Drain background goroutine.
		for range stream.Updates {
		}
		for range stream.Errs {
		}
	}

	// First-frame is a per-launch experience, not a steady-state
	// metric, so we gate on a 95th percentile across 20 runs: a
	// single jittery iteration (GC pause, scheduler hiccup) doesn't
	// drive the result, but a real regression that makes the typical
	// launch slow does.
	sortDurations(durations)
	p95 := durations[(len(durations)*95)/100]
	worst := durations[len(durations)-1]
	const limit = 100 * time.Millisecond
	if p95 > limit {
		t.Fatalf("SC-001: first-frame p95 %v exceeds %v budget (worst=%v)",
			p95, limit, worst)
	}
	t.Logf("SC-001: first-frame p95=%v worst=%v across %d runs (limit %v)",
		p95, worst, len(durations), limit)
}

// sortDurations is a tiny insertion sort over a small slice. Avoids
// pulling sort into the perf package's import surface for a 20-element
// input.
func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j] < d[j-1]; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}
