// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package loader streams a Source into the viewer in bounded chunks. Open
// returns a Stream whose first chunk is populated synchronously so the
// first frame can paint within the SC-001 budget; subsequent chunks arrive
// over a bounded Updates channel that applies backpressure to the producer.
// When a file exceeds Config.MaxResidentBytes the loader switches to a
// windowed buffer that pages slices in and out via Source.Reopen. Per-line
// truncation, cancellation, and warning surfacing (long lines, stdin
// non-seekable) are all driven from Config.
package loader
