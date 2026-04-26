// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build perf

package perf

import (
	"runtime"
	"testing"
	"time"
)

// TestDismiss_FullSpecCase is the SC-007 100-iteration nightly tier.
// Runs the dismiss budget against the full sample size from the spec;
// the PR-gate variant uses 10 iterations to keep CI wall-clock bounded.
func TestDismiss_FullSpecCase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY harness requires a Unix-like OS")
	}
	measureDismiss(t, 100, 500*time.Millisecond, true)
}
