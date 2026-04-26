// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package perf

// readRSS is a stub for platforms without /proc/self/status or
// getrusage parity (notably Windows). The SC-005 large-file tests skip
// on Windows already; this stub exists only so the perf package
// compiles cleanly on every supported toolchain.
func readRSS() (int64, error) {
	return 0, nil
}
