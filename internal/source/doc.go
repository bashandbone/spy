// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package source models the input the viewer renders.
//
// Status: skeleton (Phase 1). The exported types and constructors land in
// Phase 2.
//
// Planned: a Source interface with a Kind (Code, Markdown, Text, PDF, Image,
// Binary, Unknown), a display name, and Open / Reopen primitives so consumers
// can stream bytes or seek for windowed re-reads; FileSource and StdinSource
// implementations; a FromArgs constructor that picks one based on positional
// arguments and stdin TTY status; and Line / Token types that live here so
// highlighter, search, and renderer consumers depend on `source` rather than
// on each other.
package source
