// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package search supports forward / backward / regex / smart-case search.
//
// Status: skeleton (Phase 1). The exported API lands in Phase 2 (US2).
//
// Planned: a Compile entry point that turns a query into a Matcher; a Scan
// function that walks a source.LineProvider and emits matches on a channel
// until cancellation or exhaustion; a synthetic wrap-around sentinel that
// signals the UI to surface "search wrapped" without re-walking the buffer.
package search
