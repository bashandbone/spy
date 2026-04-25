// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package graphics encodes images for graphics-capable terminals.
//
// Status: skeleton (Phase 1). The exported encoders and dispatchers land
// in Phase 2 (US4).
//
// Planned: per-protocol encoders for Kitty, iTerm2, and sixel, plus a
// no-op renderer for the unsupported case; cleanup escapes (notably
// Kitty's "delete all images") exposed both as a string for in-session
// use via tea.Cmd and as a CleanupFunc closure the entry point defers so
// cleanup survives signals and panics; PDF rasterization gated behind
// the `fitz` build tag so the default build stays cgo-free.
package graphics
