// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package term probes the controlling terminal for capabilities that drive
// the rest of the renderer pipeline: TTY status, dimensions, color depth,
// graphics protocol, and background luminance. Detection is time-bounded
// and goroutine-safe; environment-variable overrides (NO_COLOR, COLORTERM,
// SPY_THEME, SPY_GRAPHICS) short-circuit specific probes. The package also
// exposes a panic-safe Restore primitive that the entry point defers so
// the user's terminal state is recovered on any exit path.
package term
