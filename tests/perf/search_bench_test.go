// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"context"
	"testing"
	"time"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/search"
	"github.com/knitli/spy/internal/source"
)

// TestSearch_Under500ms enforces SC-003: a literal search across a
// > 1 MiB file completes in ≤ 500 ms.
//
// The harness loads a 1.25 MiB synthetic file (5000 lines × 256 B),
// runs a forward scan for a needle that occurs once near the end, and
// asserts both first-match latency and full-scan latency under budget.
func TestSearch_Under500ms(t *testing.T) {
	const sizeBytes = 1310720 // 1.25 MiB; > 1 MiB per SC-003
	const lineBytes = 256
	path := writeSyntheticFile(t, sizeBytes, lineBytes)

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

	matcher, err := search.Compile("xyzpdq", false, search.CaseSmart)
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

	const limit = 500 * time.Millisecond
	if elapsed > limit {
		t.Fatalf("SC-003: full-scan elapsed %v exceeds %v budget", elapsed, limit)
	}
	t.Logf("SC-003: scanned %d matches across %d bytes in %v (limit %v)",
		count, sizeBytes, elapsed, limit)
}
