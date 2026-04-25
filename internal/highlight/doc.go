// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package highlight wraps Chroma v2 with a streaming-friendly highlighter.
//
// Status: skeleton (Phase 1). The exported API lands in Phase 2 (US1).
//
// Planned: a Highlighter that produces source.Token slices for code lines,
// selects a lexer per language, falls back to plain text on unknown
// languages, and honors a byte cap above which it returns un-styled tokens
// and emits a one-shot WarnHighlightDisabled advisory on a side channel
// for the UI to surface in the status bar.
package highlight
