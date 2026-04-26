// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/knitli/spy/tests/integration"
)

// TestDismiss_Under500ms enforces SC-007: from user keypress (`q`) to
// `tea.Program.Run()` returning, p95 ≤ 500 ms across N invocations
// against a 10 000-line file. The PR-gate runs 10 invocations to keep
// CI wall-clock under control; the nightly variant
// (TestDismiss_FullSpecCase, behind `-tags perf`) runs the spec's full
// 100-iteration case.
func TestDismiss_Under500ms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-driven dismiss benchmark in -short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY harness requires a Unix-like OS")
	}
	measureDismiss(t, 10, 500*time.Millisecond, true)
}

// measureDismiss is the shared driver for the PR-gate and nightly
// dismiss-budget cases.
func measureDismiss(t *testing.T, iterations int, limit time.Duration, failOnBudget bool) {
	t.Helper()
	dir := t.TempDir()
	bigPath := filepath.Join(dir, "big.txt")
	if err := writeBigFixture(bigPath, 10000); err != nil {
		t.Fatalf("write big.txt: %v", err)
	}

	durations := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		// SkipCleanup so the PTY's FD + drain buffer don't pile up
		// across iterations; we Close explicitly at the end of each
		// pass. The 100-iteration nightly tier would otherwise leak
		// 100 master/slave fd pairs and 100 byte buffers until the
		// test returned.
		p := integration.NewPTYProgramOpts(t,
			[]string{"--no-config", bigPath}, nil,
			integration.PTYOptions{SkipCleanup: true})
		iterationDone := false
		closeOnce := func() {
			if iterationDone {
				return
			}
			iterationDone = true
			_ = p.Close()
		}
		// Best-effort cleanup if the iteration t.Fatal's mid-flight.
		t.Cleanup(closeOnce)
		if !p.WaitFor(integration.AltScreenEnter, 5*time.Second) {
			closeOnce()
			t.Fatalf("iteration %d: alt-screen entry not observed", i)
		}
		// Wait for Bubble Tea's raw-mode handlers to be installed
		// before delivering input. The bracketed-paste-mode escape is
		// emitted late in the bootstrap, after the alt-screen entry
		// but before the first content paint, so it's a stable signal
		// that the input pipeline is live.
		if !p.WaitFor("\x1b[?2004h", 2*time.Second) {
			closeOnce()
			t.Fatalf("iteration %d: bracketed-paste setup not observed", i)
		}
		// Wait for the streaming-complete footer ("N lines" with no
		// trailing ellipsis) so the loader pipeline has settled and
		// the input loop is unambiguously live. The wait is not part
		// of the SC-007 measurement — the timer starts after it.
		if !p.WaitFor("10000 lines", 5*time.Second) {
			closeOnce()
			t.Fatalf("iteration %d: streaming-complete footer never painted", i)
		}
		// Small additional buffer so the very first key isn't lost on
		// a remaining input-pipeline race (cancelreader / raw-mode
		// installation runs concurrently with first paint).
		time.Sleep(150 * time.Millisecond)

		// Some Bubble Tea + PTY combinations drop the very first
		// post-stream keystroke. Send `q`, poll for exit, and (only
		// if the first send was lost) retransmit and start the SC-007
		// timer at that retransmit. That way the recorded duration
		// reflects the keystroke the binary actually saw rather than
		// the harness's input-pipeline race.
		//
		// The first-pass timeout (firstQTimeout) is a measurement
		// floor: every iteration whose first `q` propagates is
		// credited with this value as a strict upper bound on the
		// real dismiss latency. The previous 200 ms floor made
		// regressions in the 50 → 199 ms range invisible — the test
		// would record 200 ms either way. 10 ms keeps the floor an
		// order of magnitude under the 500 ms budget so that band of
		// regressions actually moves the recorded p95.
		//
		// The trade-off: legitimately-slow first-pass exits (rare on
		// commodity CI but possible under load) hit the timeout and
		// fall through to the retransmit path, which times the
		// second `q` precisely. The recorded duration in that case
		// is the second-pass elapsed time, which still reflects the
		// real dismiss latency for the keystroke the binary actually
		// consumed. Net effect: the p95 across iterations is no
		// worse than before, and the resolution is dramatically
		// finer.
		const firstQTimeout = 10 * time.Millisecond
		p.Send("q")
		var elapsed time.Duration
		exited := p.WaitForExit(firstQTimeout)
		if exited {
			elapsed = firstQTimeout
		} else {
			start := time.Now()
			p.Send("q")
			exited = p.WaitForExit(2 * time.Second)
			elapsed = time.Since(start)
		}
		if !exited {
			snap := string(p.Snapshot())
			closeOnce()
			t.Logf("iteration %d snapshot at hang: %q", i, snap)
			t.Fatalf("iteration %d: process did not exit after `q`", i)
		}
		durations = append(durations, elapsed)
		exit := p.ExitCode()
		closeOnce()
		if exit != 0 {
			t.Fatalf("iteration %d: exit code %d (want 0)", i, exit)
		}
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(len(durations)*95)/100]
	t.Logf("SC-007: dismiss p95=%v across %d invocations (limit %v); fastest=%v slowest=%v",
		p95, iterations, limit, durations[0], durations[len(durations)-1])
	if failOnBudget && p95 > limit {
		t.Fatalf("SC-007: dismiss p95 %v exceeds %v budget", p95, limit)
	}
}

// writeBigFixture emits an n-line text file under `path`. Returns
// an error rather than calling t.Fatal so test functions can branch on
// failure.
func writeBigFixture(path string, n int) error {
	var b strings.Builder
	b.Grow(n * 32)
	for i := 0; i < n; i++ {
		b.WriteString("line ")
		b.WriteString(intStr(i))
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
