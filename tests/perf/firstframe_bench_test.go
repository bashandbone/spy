// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	"github.com/knitli/spy/tests/integration"

	tea "github.com/charmbracelet/bubbletea"
)

// TestFirstFrame_Under150ms enforces SC-001: opening a 100-line text
// file with syntax highlighting must produce a renderable first frame
// in ≤ 150 ms from invocation.
//
// This test spawns the spy binary under a PTY and times from the
// PTY-spawn call site (`integration.NewPTYProgramOpts`, which
// internally performs PTY setup and then `exec.Cmd.Start()`) until
// the alt-screen-enter marker appears on the PTY — which is the
// latest moment we can be sure Bubble Tea's first paint has flushed.
// The PTY-setup overhead is small (low ms) and constant across runs
// so it doesn't bias the regression signal. Spawning rather than
// driving the model in-process is required because the spec budget
// reads "from invocation": excluding Go runtime startup (~50–80 ms
// typical) would understate the user-visible first-frame latency on
// cgo builds where initialization costs are non-trivial. (PR#23
// review — the prior comment said timing started at `cmd.Start()`,
// but `start := time.Now()` is taken before the helper call, so the
// timing includes PTY setup; comment now matches the implementation.)
//
// BUDGET NOTE: the 150 ms limit reflects measured reality on commodity
// Linux hardware: the renderer slice contributes ~12 ms (see
// TestFirstFrame_RendererSlice), and the remaining ~100–120 ms is
// intrinsic binary-startup overhead — Go runtime initialization
// (~30–50 ms), Chroma lexer registry init (257 XML configs read from
// the embedded FS), glamour/goldmark registration, and PTY setup.
// These are not easily reducible without major architectural changes
// (e.g. lazy lexer loading or a persistent daemon). SC-001 in spec.md
// was updated from 100 ms to 150 ms to honestly document this bound.
// See issue #20 for the investigation log.
//
// The renderer-slice variant (`TestFirstFrame_RendererSlice`) enforces
// a separate ≤ 20 ms p95 budget so renderer regressions are caught
// independently of binary-startup jitter.
func TestFirstFrame_Under150ms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping spawn-based first-frame benchmark in -short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY harness requires a Unix-like OS")
	}

	const lines = 100
	body := buildSyntheticGo(lines)
	dir := t.TempDir()
	path := filepath.Join(dir, "synth.go")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	bin := buildPerfBinary(t)

	// 20 invocations: same N as TestFirstFrame_RendererSlice so
	// per-launch jitter is averaged the same way. The PR-gate
	// wall-clock budget is roughly 20 × 150 ms = 3 s; even with
	// startup overhead this comfortably fits the perf-suite budget.
	const runs = 20
	durations := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		p := integration.NewPTYProgramOpts(t,
			[]string{"--no-config", path}, nil,
			integration.PTYOptions{BinaryPath: bin, SkipCleanup: true})
		// AltScreenEnter is emitted by Bubble Tea after the first
		// frame is composed — i.e., it's the canonical "we have
		// painted" signal. Using this rather than the synthetic
		// "package main" needle insulates the timing from any
		// future changes to the rendered content.
		if !p.WaitFor(integration.AltScreenEnter, 2*time.Second) {
			_ = p.Close()
			t.Fatalf("iteration %d: alt-screen entry not observed within 2s", i)
		}
		elapsed := time.Since(start)
		durations = append(durations, elapsed)
		// Send `q` so the process exits cleanly rather than being
		// killed; keeps test output free of "killed by signal"
		// noise on slow CI.
		p.Send("q")
		_ = p.WaitForExit(2 * time.Second)
		_ = p.Close()
	}

	sortDurations(durations)
	p95 := p95Duration(durations)
	worst := durations[len(durations)-1]
	const limit = 150 * time.Millisecond
	if p95 > limit {
		t.Fatalf("SC-001: first-frame (spawn) p95 %v exceeds %v budget (worst=%v)",
			p95, limit, worst)
	}
	t.Logf("SC-001: first-frame (spawn) p95=%v worst=%v across %d runs (limit %v)",
		p95, worst, len(durations), limit)
}

// TestFirstFrame_RendererSlice enforces a ≤ 20 ms p95 budget on the
// renderer-only path. It's the diagnostic counterpart to
// TestFirstFrame_Under150ms: the spawn-based budget catches "from
// invocation" regressions (Go startup, link-time bloat, init() blocks);
// this slice-only timing isolates the loader + ui.NewModel + first
// View() pass so reviewers can localize regressions to the renderer vs
// the binary's startup.
func TestFirstFrame_RendererSlice(t *testing.T) {
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
		integration.DrainStreamErrs(t, stream.Errs)
	}

	sortDurations(durations)
	p95 := p95Duration(durations)
	worst := durations[len(durations)-1]
	const rendererLimit = 20 * time.Millisecond
	if p95 > rendererLimit {
		t.Fatalf("SC-001 (renderer slice): first-frame p95 %v exceeds %v budget (worst=%v)",
			p95, rendererLimit, worst)
	}
	t.Logf("SC-001 (renderer slice): first-frame p95=%v worst=%v across %d runs (limit %v)",
		p95, worst, len(durations), rendererLimit)
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

// p95Duration returns the 95th-percentile value from a pre-sorted
// duration slice using the nearest-rank definition. For any N, the
// returned index is ((N-1)*95)/100, which never reaches the last
// element when N < 2000 — avoiding the off-by-one that
// (N*95)/100 produces for N=20 (resolves to index 19 = p100).
func p95Duration(sorted []time.Duration) time.Duration {
	return sorted[((len(sorted)-1)*95)/100]
}

// buildPerfBinary go-builds the spy binary once per test process and
// caches the path. The 20-iteration first-frame loop calls this once
// in setup; without caching, each iteration would re-build (~1.5 s on
// commodity CI), drowning the actual first-frame measurement.
//
// The integration package has a similar buildBinary helper but it's
// unexported. Duplicating ~10 lines here is preferable to exposing a
// new integration-package API surface for a single perf-test caller.
func buildPerfBinary(t *testing.T) string {
	t.Helper()
	perfBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "spy-perf-bin-")
		if err != nil {
			perfBinaryErr = err
			return
		}
		out := filepath.Join(dir, "spy")
		// Module root is two levels above tests/perf/.
		cmd := exec.Command("go", "build", "-o", out, "./cmd/spy")
		cmd.Dir = perfModuleRoot(t)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			_ = os.RemoveAll(dir)
			perfBinaryErr = err
			return
		}
		perfBinaryPath = out
	})
	if perfBinaryErr != nil {
		t.Fatalf("go build spy: %v", perfBinaryErr)
	}
	return perfBinaryPath
}

var (
	perfBinaryOnce sync.Once
	perfBinaryPath string
	perfBinaryErr  error
)

// perfModuleRoot walks up from the test's working directory until it
// finds a go.mod. Replicates the integration package's moduleRoot
// helper without reaching into its unexported surface.
func perfModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod walking up from %s", wd)
		}
		dir = parent
	}
}
