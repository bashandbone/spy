// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package render produces the visible frame for the viewer. ForKind dispatches
// on source.Kind to per-kind Renderers (code, markdown, text, image, PDF)
// that consume a RenderContext populated by internal/ui each frame. The
// package also owns the Theme abstraction — built-in dark/light defaults
// plus ResolveTheme, which honors capability detection and explicit
// overrides — and the status-bar layout used across all kinds.
package render
