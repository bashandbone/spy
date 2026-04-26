// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"context"
	"io"
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
	"github.com/knitli/spy/tests/integration"
)

// memSource adapts a string body into a [source.Source]. Used by the
// perf benchmarks so they don't touch the filesystem.
type memSource struct {
	body string
	kind source.Kind
	lex  string
}

func (m *memSource) Kind() source.Kind   { return m.kind }
func (m *memSource) DisplayName() string { return "synth.go" }
func (m *memSource) Metadata() source.Metadata {
	return source.Metadata{Path: "synth.go", Language: m.lex, LineCount: -1}
}
func (m *memSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(m.body)), nil
}
func (m *memSource) Reopen() (io.ReadSeeker, error) {
	return strings.NewReader(m.body), nil
}

// TestThemeSwap_Under16ms is the SC-004 gate: 100 `:set theme dark|light`
// swaps must re-render the visible viewport at p95 ≤ 16 ms (60 fps).
// The token cache is reused — Chroma styles apply at format time per
// research R7, not at tokenisation. The renderer formats only the lines
// within the active viewport window, bounding work to O(viewport height)
// regardless of total buffer size.
func TestThemeSwap_Under16ms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10000-line theme swap regression in -short mode")
	}
	const visibleScaleLines = 10000
	measureThemeSwap(t, visibleScaleLines, 16*time.Millisecond, true)
}

// TestThemeSwap_FullSpecCase is the SC-004 spec-level gate: 100 theme
// swaps against the full 10 000-line buffer must meet p95 ≤ 16 ms.
func TestThemeSwap_FullSpecCase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10000-line spec case in -short mode")
	}
	measureThemeSwap(t, 10000, 16*time.Millisecond, true)
}

// measureThemeSwap is the shared driver for both theme-swap benchmark
// shapes. When `failOnBudget` is true and p95 exceeds `limit`, the test
// fails. Otherwise the wall-clock is logged for visibility.
func measureThemeSwap(t *testing.T, lines int, limit time.Duration, failOnBudget bool) {
	t.Helper()
	body := buildSyntheticGo(lines)
	src := &memSource{body: body, kind: source.KindCode, lex: "go"}

	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	// Drain the stream so all lines are tokenised before we start
	// timing — we want to measure pure re-render, not load latency.
	for range stream.Updates {
	}
	integration.DrainStreamErrs(t, stream.Errs)

	cfg := config.Defaults()
	caps := term.Capabilities{Cols: 100, Rows: 30}
	hl := highlight.New(nil, term.ColorTrueColor, 5*1024*1024)
	hl.SetLang("go")

	model := ui.NewModel(ui.ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: caps,
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  hl,
	})

	// Drive a resize so the viewport is sized for rendering.
	mu, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, ok := mu.(ui.Model)
	if !ok {
		t.Fatalf("model update did not return ui.Model")
	}

	const swaps = 100
	durations := make([]time.Duration, 0, swaps)
	themes := []string{"dark", "light"}
	for i := 0; i < swaps; i++ {
		theme := themes[i%2]
		// Pre-buffer the prompt + body so timing only covers the
		// Enter key — i.e., the actual `:set theme` execution + paint.
		m = applyKeystrokes(t, m, ":set theme "+theme)
		start := time.Now()
		m = applyKeystrokes(t, m, "\r")
		durations = append(durations, time.Since(start))
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})
	p95 := p95Duration(durations)
	t.Logf("SC-004 (%d lines): theme-swap p95=%v across %d swaps (limit %v); fastest=%v slowest=%v",
		lines, p95, swaps, limit, durations[0], durations[len(durations)-1])
	if failOnBudget && p95 > limit {
		t.Fatalf("SC-004: theme-swap p95 %v exceeds %v budget at %d lines", p95, limit, lines)
	}
}

// applyKeystrokes feeds each rune in `s` to the model as a sequence of
// tea.KeyMsg events, returning the resulting model after all messages
// are processed. `\r` (carriage return) maps to tea.KeyEnter.
func applyKeystrokes(t *testing.T, m ui.Model, s string) ui.Model {
	t.Helper()
	for _, r := range s {
		var msg tea.Msg
		switch r {
		case '\r':
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case '\x1b':
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		}
		mm, _ := m.Update(msg)
		var ok bool
		m, ok = mm.(ui.Model)
		if !ok {
			t.Fatalf("update returned non-ui.Model")
		}
	}
	return m
}

// buildSyntheticGo emits an `lines`-line Go source string (plus a
// short package + main wrapper). Each line is a short statement that
// exercises Chroma's Go lexer (keywords, identifiers, numbers,
// operators, and string literals). Reused by the first-frame and
// theme-swap benchmarks at different line counts.
func buildSyntheticGo(lines int) string {
	var b strings.Builder
	b.WriteString("package main\n\nimport \"fmt\"\n\nfunc main() {\n")
	for i := 0; i < lines; i++ {
		// Keywords + ints + strings → exercises lexer state transitions.
		b.WriteString("\tif x := ")
		b.WriteString(intStr(i))
		b.WriteString("; x > 0 { fmt.Println(\"line ")
		b.WriteString(intStr(i))
		b.WriteString("\") }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// intStr is a stack-friendly int formatter for buildSynthetic*. The
// digit accumulator lives on a fixed-size stack array so the only
// allocation per call is the final string conversion (which is
// unavoidable in Go without a *strings.Builder caller).
func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte // enough for int64
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
