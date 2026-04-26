// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build freebsd || netbsd || openbsd || dragonfly

package perf

// rusageMaxrssBytes converts ru_maxrss into bytes for BSD-family OSes
// where getrusage(2) returns kilobytes. See rss_darwin.go for the
// macOS-divergent units.
func rusageMaxrssBytes(maxrss int64) int64 {
	return maxrss * 1024
}
