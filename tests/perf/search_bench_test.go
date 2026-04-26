// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/search"
	"github.com/knitli/spy/internal/source"
)

// osOpenAppend is a tiny shim so the helper above doesn't pull os
// into the import block when the rest of the file doesn't need it.
func osOpenAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
}

// TestSearch_Under500ms enforces SC-003: a literal search across a
// > 1 MiB file completes in ≤ 500 ms. The fixture has the needle
// embedded once near the end (last 1 KiB) so the scan must walk most
// of the buffer to find it; the test asserts the match was actually
// found.
func TestSearch_Under500ms(t *testing.T) {
	const sizeBytes = 1310720 // 1.25 MiB; > 1 MiB per SC-003
	const lineBytes = 256
	const needle = "xyzpdq"
	path := writeSyntheticFileWithNeedle(t, sizeBytes, lineBytes, needle)

	ctx := context.Background()
	src, err := source.FromArgs([]string{path}, nil, "")
	if err != nil {
		t.Fatalf("source.FromArgs: %v", err)
	}
	stream, err := loader.Open(ctx, src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	// Drain the stream synchronously so search runs against a settled
	// buffer, mirroring the steady-state case the user experiences.
	for range stream.Updates {
	}
	for range stream.Errs {
	}

	matcher, err := search.Compile(needle, false, search.CaseSmart)
	if err != nil {
		t.Fatalf("search.Compile: %v", err)
	}

	start := time.Now()
	results := search.Scan(ctx, stream.Buffer, matcher, search.DirForward, 1)
	count := 0
	for range results {
		count++
	}
	elapsed := time.Since(start)

	if count < 1 {
		t.Fatalf("SC-003: needle %q never matched — synthetic fixture is broken", needle)
	}

	const limit = 500 * time.Millisecond
	if elapsed > limit {
		t.Fatalf("SC-003: full-scan elapsed %v exceeds %v budget", elapsed, limit)
	}
	t.Logf("SC-003: scanned %d matches across %d bytes in %v (limit %v)",
		count, sizeBytes, elapsed, limit)
}

// writeSyntheticFileWithNeedle is [writeSyntheticFile] with the
// needle embedded once on a line near the tail of the file. The
// fixture remains close to `targetBytes` total — the needle line
// replaces one of the synthetic stride-256 lines rather than being
// appended on top.
func writeSyntheticFileWithNeedle(t *testing.T, targetBytes, lineBytes int, needle string) string {
	t.Helper()
	path := writeSyntheticFile(t, targetBytes, lineBytes)
	// Append the needle on its own line at EOF. Search.Scan will see
	// it as the last (or near-last) match.
	f, err := osOpenAppend(path)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(needle + "\n"); err != nil {
		t.Fatalf("append needle: %v", err)
	}
	return path
}
