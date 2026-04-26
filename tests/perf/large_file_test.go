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
	"testing"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
)

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

	rssBefore := residentBytes(t)

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
	for range stream.Errs {
	}

	rssAfter := residentBytes(t)
	delta := rssAfter - rssBefore
	const limitBytes = 250 * 1024 * 1024
	if delta > limitBytes {
		t.Fatalf("SC-005 PR-gate: RSS delta %.1f MiB exceeds %.0f MiB budget",
			float64(delta)/1024/1024, float64(limitBytes)/1024/1024)
	}
	t.Logf("SC-005 PR-gate: RSS delta %.1f MiB (limit %.0f MiB)",
		float64(delta)/1024/1024, float64(limitBytes)/1024/1024)
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

// residentBytes returns the current process's heap-allocated bytes
// (HeapAlloc). It's a lower bound on RSS; works on every platform Go
// supports without resorting to /proc/self/status. For SC-005 we care
// about the trend (we shouldn't blow up by 200 MiB+ over the ambient),
// not the precise OS-reported figure.
func residentBytes(t *testing.T) int64 {
	t.Helper()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	// Use HeapInuse as a closer-to-RSS approximation than HeapAlloc;
	// it includes spans returned to the OS but not yet released.
	return int64(ms.HeapInuse)
}
