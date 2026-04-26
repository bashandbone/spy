// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build linux

package perf

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// readRSS returns the resident-set size of the current process in bytes
// by parsing `VmRSS:` out of /proc/self/status. Linux-only.
//
// VmRSS reports the total resident memory the kernel has mapped for
// this process — including pages backing cgo allocations (notably MuPDF
// via go-fitz for SC-010) which `runtime.MemStats.HeapInuse` does not
// see. SC-005 is an RSS budget per spec, so the test must read RSS
// (not heap) to verify the promise on cgo builds.
//
// Returns 0 with the underlying I/O / parse error if /proc/self/status
// can't be opened or the VmRSS line can't be parsed, and 0 with a nil
// error if the file is readable but contains no VmRSS line (extremely
// rare; would require a kernel without procfs accounting). The caller
// (`residentBytes`) treats either case as "couldn't measure" and falls
// through to HeapInuse with a "heap-inuse-fallback" label, so the test
// still produces a reading. (PR#23 review — the prior comment claimed
// "0 and no error on failure" which didn't match the implementation.)
func readRSS() (int64, error) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// "VmRSS:    1234 kB"
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		// Strip prefix, trim whitespace, split off "kB" suffix.
		v := strings.TrimSpace(strings.TrimPrefix(line, "VmRSS:"))
		v = strings.TrimSuffix(v, " kB")
		v = strings.TrimSpace(v)
		kb, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			return 0, perr
		}
		return kb * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, nil
}
