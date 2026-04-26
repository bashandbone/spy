// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !linux && (darwin || freebsd || netbsd || openbsd || dragonfly)

package perf

import "syscall"

// readRSS returns the resident-set size of the current process in
// bytes via getrusage(RUSAGE_SELF).Maxrss. This is the BSD/macOS
// fallback path; Linux uses /proc/self/status:VmRSS via rss_linux.go,
// which reports current RSS (Maxrss is the high-water mark).
//
// PORTABILITY NOTE: ru_maxrss units differ across Unixes:
//   - Linux: kilobytes (1024 bytes). The spec says "RSS"; on Linux we
//     use /proc/self/status:VmRSS instead — it tracks current RSS
//     rather than the high-water mark, so the SC-005 measurement
//     reflects the post-stream resident size, not the worst case
//     during streaming. This file is Linux-excluded for that reason.
//   - macOS / Darwin: BYTES. (XNU diverges from BSD here; Apple's
//     getrusage(2) man page documents this.)
//   - *BSD: kilobytes.
//
// We treat darwin specially and return Maxrss as-is; everywhere else
// we multiply by 1024.
func readRSS() (int64, error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, err
	}
	return rusageMaxrssBytes(ru.Maxrss), nil
}
