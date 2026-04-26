// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build darwin

package perf

// rusageMaxrssBytes converts ru_maxrss into bytes for darwin. Apple's
// getrusage(2) returns ru_maxrss in BYTES, unlike every other Unix
// (which reports it in kilobytes). Keep the conversion in a per-OS
// file so the rest of the perf tests don't carry a build-tag matrix.
func rusageMaxrssBytes(maxrss int64) int64 {
	return maxrss
}
