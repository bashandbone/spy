// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package loader streams a source.Source into the viewer in bounded chunks.
//
// Status: skeleton (Phase 1). The exported API lands in Phase 2.
//
// Planned: an Open entry point returning a Stream whose first chunk is
// populated synchronously so the first frame paints within the SC-001
// budget; subsequent chunks delivered over a bounded Updates channel that
// applies backpressure to the producer; a windowed buffer that takes over
// when a file exceeds Config.MaxResidentBytes and pages slices in and out
// via Source.Reopen; per-line truncation, cancellation, and warning
// surfacing (long lines, stdin non-seekable) all driven from Config.
package loader
