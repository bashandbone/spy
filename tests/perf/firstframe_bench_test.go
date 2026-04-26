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

	// Use the slowest run as the gate — first-frame is a per-launch
	// experience, not a percentile. The 20-iteration loop guards
	// against an unlucky outlier driving the gate.
	worst := durations[0]
	for _, d := range durations {
		if d > worst {
			worst = d
		}
	}
	const limit = 100 * time.Millisecond
	if worst > limit {
		t.Fatalf("SC-001: first-frame worst-case %v exceeds %v budget", worst, limit)
	}
	t.Logf("SC-001: first-frame worst=%v across %d runs (limit %v)",
		worst, len(durations), limit)
}
