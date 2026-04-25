// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package term is the terminal-capability layer for spy.
//
// Status: skeleton (Phase 1). The exported runtime API lands in Phase 2.
//
// Planned: a [Detect] entry point that probes TTY status, dimensions,
// color depth, graphics protocol, and background luminance under a
// time-bounded, goroutine-safe contract; environment-variable overrides
// (NO_COLOR, COLORTERM, SPY_THEME, SPY_GRAPHICS) that short-circuit
// specific probes; and a panic-safe [Restore] primitive the entry point
// defers so the user's terminal state is recovered on any exit path.
package term
