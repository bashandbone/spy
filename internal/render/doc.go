// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package render produces the visible frame for the viewer.
//
// Status: skeleton (Phase 1). The exported API lands in Phase 2.
//
// Planned: a ForKind dispatcher that selects per-kind Renderers (code,
// markdown, text, image, PDF) consuming a RenderContext populated by
// internal/ui each frame; a Theme abstraction with built-in dark/light
// defaults plus ResolveTheme honoring capability detection and explicit
// overrides; and the status-bar layout used across all kinds.
package render
