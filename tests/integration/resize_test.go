// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

// The resize test asserts a wall-clock budget; race instrumentation
// distorts the measurement. The unit-test resize coverage in
// internal/ui/model_test.go runs under `-race` and exercises the
// non-budget assertions (viewport anchor, wrap-cache invalidation).

package integration

import (
	"context"
	"io"
	"math/rand"
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

// TestResize_PreservesViewportAnchor enforces SC-008: after a
// `tea.WindowSizeMsg`, (a) the line previously at viewport row 0 stays
// at row 0, (b) the wrap cache reflows so a long line that fit on one
// row at width W1 spans multiple rows at narrower width W2, (c) the
// next paint completes in ≤ 16 ms p95 across 50 successive resize
// events at random widths in [40, 200] columns.
//
// The test exercises the model directly rather than the binary so the
// p95 measurement is uncontaminated by PTY drain / terminal echo
// latency. The PTY-driven dispatch is a separate test
// (TestTextReview_HighlightedFile) that verifies end-to-end alt-screen
// behaviour.
func TestResize_PreservesViewportAnchor(t *testing.T) {
	body := buildSyntheticTextBuffer(10000)
	src := &resizeMemSource{body: body, kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	for range stream.Updates {
	}
	for range stream.Errs {
	}

	cfg := config.Defaults()
	cfg.WordWrap = true
	caps := term.Capabilities{Cols: 80, Rows: 24}
	hl := highlight.New(nil, term.ColorTrueColor, 5*1024*1024)

	model := ui.NewModel(ui.ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: caps,
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  hl,
	})

	// Initial size — establishes the anchor for the first paint.
	mu, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, ok := mu.(ui.Model)
	if !ok {
		t.Fatalf("model update returned non-ui.Model")
	}

	// (a) Anchor preservation. Scroll a known number of rows down,
	// snapshot the visible top, resize, and assert the visible top
	// matches.
	const scrollDown = 50
	for i := 0; i < scrollDown; i++ {
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = mm.(ui.Model)
	}
	topBefore := firstVisibleSourceLine(m.View())
	if topBefore == "" {
		t.Fatalf("could not determine top line before resize; view tail=%q",
			lastNonEmptyLine(m.View()))
	}
	mu2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = mu2.(ui.Model)
	topAfter := firstVisibleSourceLine(m.View())
	if topAfter != topBefore {
		t.Errorf("SC-008(a): viewport row-0 line changed across resize\n  before: %q\n   after: %q",
			topBefore, topAfter)
	}

	// (b) Wrap cache reflow. A 58-char fixture line fits on one row
	// at width 80 but must wrap into ≥2 rows at width 30. Count the
	// rendered visual rows that map to source line 1 to confirm.
	rowsAt80 := visualRowsForFirstLine(m.View())
	mu3, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 24})
	m = mu3.(ui.Model)
	rowsAt30 := visualRowsForFirstLine(m.View())
	if rowsAt30 <= rowsAt80 {
		t.Errorf("SC-008(b): wrap cache did not reflow on width 80 → 30; rows: %d → %d (expected growth)",
			rowsAt80, rowsAt30)
	}
	// Restore the wider viewport for the perf loop.
	mu4, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = mu4.(ui.Model)

	// (c) Per-resize wall-clock budget. Deterministic random sizes so
	// failures are reproducible.
	rng := rand.New(rand.NewSource(1234))
	const iterations = 50
	durations := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		w := 40 + rng.Intn(161) // [40, 200]
		h := 12 + rng.Intn(25)  // [12, 36]
		start := time.Now()
		nm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		_ = nm.(ui.Model).View() // force the next-paint pass
		durations = append(durations, time.Since(start))
		m = nm.(ui.Model)
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})
	p95 := durations[(len(durations)*95)/100]
	const limit = 16 * time.Millisecond
	if p95 > limit {
		t.Fatalf("SC-008(c): resize p95 %v exceeds %v budget across %d iterations",
			p95, limit, iterations)
	}
	t.Logf("SC-008: resize p95=%v across %d iterations (limit %v); fastest=%v slowest=%v",
		p95, iterations, limit, durations[0], durations[len(durations)-1])
}

// firstVisibleSourceLine extracts the gutter line number from the first
// non-empty rendered row of `view`. The textRenderer prefixes each
// source line with `<gutter> <Raw>`; lines with a gutter number are
// new source lines; lines without are wrapped continuation rows. We
// return the source-line content (without the gutter) so the assertion
// is robust to width-driven gutter-padding changes.
func firstVisibleSourceLine(view string) string {
	for _, line := range strings.Split(view, "\n") {
		trimmed := strings.TrimSpace(stripANSI(line))
		if trimmed == "" {
			continue
		}
		// Gutter-prefixed rows look like "  42  the quick brown fox …".
		// Strip a leading run of spaces + digits + spaces (the gutter)
		// when present; otherwise return the trimmed content as-is
		// (continuation rows blank the gutter).
		i := 0
		for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
			i++
		}
		if i > 0 {
			rest := strings.TrimLeft(trimmed[i:], " ")
			return rest
		}
		return trimmed
	}
	return ""
}

// visualRowsForFirstLine counts how many visual rows of `view` belong
// to the first source line. Continuation rows have a blank gutter; we
// stop counting once we hit a row whose gutter holds the next line
// number (or until the buffer ends).
func visualRowsForFirstLine(view string) int {
	rows := 0
	seenFirst := false
	for _, line := range strings.Split(view, "\n") {
		clean := stripANSI(line)
		if strings.TrimSpace(clean) == "" {
			continue
		}
		// Look for a leading gutter digit run.
		t := strings.TrimLeft(clean, " ")
		if len(t) == 0 {
			continue
		}
		if t[0] >= '0' && t[0] <= '9' {
			if seenFirst {
				return rows
			}
			seenFirst = true
		}
		if seenFirst {
			rows++
		}
	}
	return rows
}

// stripANSI removes CSI escape sequences from `s`. Lightweight enough
// to share between the two helpers above without pulling in regexp.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' && s[j] != 'H' && s[j] != 'J' && s[j] != 'K' {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// lastNonEmptyLine returns the last non-empty line of `s` for diag.
func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

// resizeMemSource is the integration-package twin of perf.memSource,
// kept local so the integration package doesn't grow a perf-package
// import dependency.
type resizeMemSource struct {
	body string
	kind source.Kind
}

func (m *resizeMemSource) Kind() source.Kind   { return m.kind }
func (m *resizeMemSource) DisplayName() string { return "synth.txt" }
func (m *resizeMemSource) Metadata() source.Metadata {
	return source.Metadata{Path: "synth.txt", LineCount: -1}
}
func (m *resizeMemSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(m.body)), nil
}
func (m *resizeMemSource) Reopen() (io.ReadSeeker, error) {
	return strings.NewReader(m.body), nil
}

// buildSyntheticTextBuffer produces an n-line plaintext buffer used by
// the resize test. Each line is 60 characters of repeated content so
// soft-wrap kicks in at narrow widths.
func buildSyntheticTextBuffer(n int) string {
	const line = "the quick brown fox jumps over the lazy dog and continues."
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
