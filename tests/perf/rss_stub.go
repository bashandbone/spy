// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package perf

// readRSS is a stub for platforms without /proc/self/status or
// getrusage parity (notably Windows). Returning (0, nil) lets the
// caller (`residentBytes`) detect the no-measurement case and fall
// through to HeapInuse with a "heap-inuse-fallback" label, so the
// SC-005 PR-gate still runs on Windows — just with HeapInuse as a
// degraded proxy for RSS rather than skipping outright. (PR#23 review
// — the prior comment claimed Windows was skipped, which wasn't
// accurate; the test runs but with weaker measurement.)
func readRSS() (int64, error) {
	return 0, nil
}
