// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package integration provides PTY-driven harnesses for end-to-end tests
// that exercise capability paths (alt-screen, signal handling, graphics
// emit, theme probes) which cannot be covered by unit tests alone. The
// helpers here wrap creos/pty + golang.org/x/term to spawn the spy binary
// against a controlled pseudo-terminal and capture frames for golden-file
// comparison.
//
// This is the skeleton placeholder created by T004; the concrete
// NewPTYProgram and golden helpers are filled in by Phase 2 tests.
package integration

import (
	"testing"
)

// PTYProgram is the handle returned by NewPTYProgram. The full surface
// (Send, Read, Snapshot, Close, ExitCode) is filled in alongside Phase 2
// foundational tests.
type PTYProgram struct {
	t *testing.T
}

// NewPTYProgram spawns the spy binary against a fresh PTY with the given
// args and environment. Implementation lands in Phase 2; today this is a
// declared API that downstream tests can reference.
func NewPTYProgram(t *testing.T, args []string, env map[string]string) *PTYProgram {
	t.Helper()
	return &PTYProgram{t: t}
}
