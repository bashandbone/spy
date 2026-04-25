// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package graphics encodes images for terminals that speak Kitty, iTerm2,
// or sixel protocols, plus the no-op renderer for the unsupported case.
// Cleanup escapes (notably Kitty's "delete all images") are exposed both
// as a string for in-session use via tea.Cmd and as a CleanupFunc closure
// the entry point defers so cleanup survives signals and panics. PDF
// rasterization is gated behind the `fitz` build tag so the default build
// stays cgo-free.
package graphics
