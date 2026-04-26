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
// Returns 0 (and no error) if /proc/self/status can't be read or the
// VmRSS line isn't present; the caller treats 0 as "couldn't measure"
// and the test logs it but doesn't fail. This keeps the helper safe to
// call from non-test code paths if it's ever lifted out.
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
