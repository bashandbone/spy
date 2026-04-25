// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package integration provides PTY-driven end-to-end harnesses for tests
// that exercise capability paths (alt-screen, signal handling, graphics
// emit, theme probes) which cannot be covered by unit tests alone.
//
// Status: skeleton (Phase 1). Phase 2 picks the PTY library, adds it as a
// dependency, and fills in the concrete spawn / capture / snapshot paths.
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
