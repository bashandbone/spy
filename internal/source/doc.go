// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package source models the input the viewer is rendering. A Source has a
// Kind (Code, Markdown, Text, PDF, Image, Binary, Unknown), a display name,
// and Open / Reopen primitives so consumers can stream bytes or seek for
// windowed re-reads. FileSource and StdinSource implement the interface;
// FromArgs picks one based on positional arguments and stdin TTY status.
// The Line and Token types live here so highlighter, search, and renderer
// consumers depend on `source` rather than on each other.
package source
