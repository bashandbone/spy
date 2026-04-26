// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/tests/integration"
)

// SC-005 is an RSS budget per spec.md (not a heap budget). Cgo
// allocations — notably MuPDF arenas reached via go-fitz on `-tags
// fitz` builds for SC-010 — are invisible to runtime.MemStats. The
// helpers below read OS-reported resident size so the SC-005 budget
// holds even when cgo memory is in flight. See rss_linux.go,
// rss_other.go, rss_darwin.go, rss_bsd.go, rss_stub.go for the
// per-platform implementations.

// TestLargeFile_PRGate is the SC-005 PR-gate budget: load a 200 MiB
// synthetic file (the largest size that does NOT trigger windowed mode
// at the spec's 256 MiB threshold) and assert resident memory stays
// ≤ 250 MiB after the entire stream has drained.
//
// 200 MiB sits just below the 256 MiB windowed-mode threshold, so this
// case verifies we don't blow up when the in-memory tier holds the
// largest file the spec promises to keep resident. The nightly tier
// covers the windowed-mode 1 GiB / 500 MiB scaling case.
func TestLargeFile_PRGate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-file PR gate in -short mode")
	}
	const sizeBytes = 200 * 1024 * 1024 // 200 MiB
	const lineBytes = 256
	path := writeSyntheticFile(t, sizeBytes, lineBytes)

	rssBefore, source0 := residentBytes(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src, err := source.FromArgs([]string{path}, nil, "")
	if err != nil {
		t.Fatalf("source.FromArgs: %v", err)
	}
	stream, err := loader.Open(ctx, src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	for range stream.Updates {
	}
	integration.DrainStreamErrs(t, stream.Errs)

	rssAfter, source1 := residentBytes(t)
	// Keep stream alive so that debug.FreeOSMemory() inside residentBytes
	// does not cause the GC to collect stream.Buffer (and all its strings)
	// before the RSS snapshot is taken. Without this, the compiler can
	// legally declare stream dead after DrainStreamErrs — the GC would
	// then collect the 200 MiB buffer and the delta would be near-zero
	// rather than reflecting the actual cost of keeping the buffer live.
	runtime.KeepAlive(stream)
	delta := rssAfter - rssBefore
	const limitBytes = 250 * 1024 * 1024
	if delta > limitBytes {
		t.Errorf("SC-005 PR-gate: RSS delta %.1f MiB exceeds %.0f MiB budget (measured via %s → %s)",
			float64(delta)/1024/1024, float64(limitBytes)/1024/1024, source0, source1)
	} else {
		t.Logf("SC-005 PR-gate: RSS delta %.1f MiB (limit %.0f MiB; measured via %s)",
			float64(delta)/1024/1024, float64(limitBytes)/1024/1024, source1)
	}
}

// writeSyntheticFile emits `targetBytes` of newline-separated content
// with a stable per-line stride of `lineBytes`. The caller cleans up
// via t.TempDir().
func writeSyntheticFile(t *testing.T, targetBytes, lineBytes int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	line := make([]byte, lineBytes)
	for i := 0; i < lineBytes-1; i++ {
		line[i] = byte('a' + (i % 26))
	}
	line[lineBytes-1] = '\n'
	written := 0
	for written < targetBytes {
		n, err := f.Write(line)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		written += n
	}
	return path
}

// residentBytes returns the OS-reported resident-set size of this
// process in bytes, plus a short label identifying which mechanism
// produced the number. SC-005 is an RSS budget per spec.md, and cgo
// allocations (notably MuPDF arenas via go-fitz on `-tags fitz`
// builds) are absent from runtime.MemStats — so reading HeapInuse
// would understate true memory pressure on cgo paths.
//
// Read order:
//   - Linux: /proc/self/status:VmRSS (current RSS, see rss_linux.go)
//   - Darwin/BSD: getrusage(RUSAGE_SELF).Maxrss (high-water mark, see
//     rss_darwin.go / rss_bsd.go for unit handling — Maxrss is in
//     bytes on Darwin, kilobytes on BSD)
//   - Windows / unsupported: returns runtime.MemStats.HeapInuse with
//     the label "heap-inuse-fallback" so the test still produces a
//     reading rather than 0. The SC-005 PR-gate runs on Windows with
//     this degraded measurement; SC-005's RSS budget is unverified
//     there but the test catches gross regressions in HeapInuse
//     deltas. (PR#23 review — the prior comment claimed a Windows
//     skip that didn't actually exist.)
//
// debug.FreeOSMemory() is called first so that: (a) any GC garbage
// from previous operations is collected, and (b) idle heap pages that
// the Go runtime holds but is no longer actively using are returned to
// the OS. This measures the steady-state RSS — what the process
// actually needs to keep the loaded buffer alive — rather than the
// peak RSS observed during streaming (which can be ~1.5–2× higher due
// to transient allocations in the loader pipeline that haven't been
// scavenged yet). runtime.MemStats.HeapSys minus HeapReleased
// approximates the same quantity; debug.FreeOSMemory() collapses the
// difference by forcing the scavenger synchronously.
func residentBytes(t *testing.T) (int64, string) {
	t.Helper()
	debug.FreeOSMemory() // GC + return idle pages to OS; see comment above
	if rss, err := readRSS(); err == nil && rss > 0 {
		return rss, "os-rss"
	}
	// Last-resort fallback so the test still produces a reading on
	// platforms without /proc/self/status or getrusage parity.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return int64(ms.HeapInuse), "heap-inuse-fallback"
}
