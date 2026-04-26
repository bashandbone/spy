// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build perf && !race

package perf

import (
	"context"
	"testing"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/tests/integration"
)

// TestLargeFile_Nightly is the SC-005 1 GiB / 500 MiB heavyweight case
// behind `-tags perf`. The nightly workflow runs it on `ubuntu-latest`
// with `RUNNER_OS_RAM_HINT=8192`. Failure files an issue tagged
// `perf-regression` rather than blocking PRs, so blame attribution
// stays clear at the nightly cadence.
func TestLargeFile_Nightly(t *testing.T) {
	const sizeBytes = 1 << 30 // 1 GiB
	const lineBytes = 256
	path := writeSyntheticFile(t, sizeBytes, lineBytes)

	rssBefore := residentBytes(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src, err := source.FromArgs([]string{path}, nil, "")
	if err != nil {
		t.Fatalf("source.FromArgs: %v", err)
	}
	// Use the spec's default 256 MiB windowed-mode threshold so the
	// loader switches into windowed mode mid-stream.
	stream, err := loader.Open(ctx, src, loader.Config{
		MaxResidentBytes: 256 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	for range stream.Updates {
	}
	integration.DrainStreamErrs(t, stream.Errs)

	rssAfter := residentBytes(t)
	delta := rssAfter - rssBefore
	const limitBytes = 500 * 1024 * 1024 // 500 MiB
	if delta > limitBytes {
		t.Fatalf("SC-005 nightly: RSS delta %.1f MiB exceeds %.0f MiB budget",
			float64(delta)/1024/1024, float64(limitBytes)/1024/1024)
	}
	t.Logf("SC-005 nightly: RSS delta %.1f MiB (limit %.0f MiB)",
		float64(delta)/1024/1024, float64(limitBytes)/1024/1024)
}
