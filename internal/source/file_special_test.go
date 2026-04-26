// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"errors"
	"os"
	"testing"
)

// TestRejectSpecialMode_TableDriven exercises the mode-bit predicates
// that gate FileSource construction. We use [rejectSpecialMode]
// directly so the test is portable: synthesizing a real FIFO requires
// `mkfifo` and a Unix host, but the mode-bit logic is pure and can be
// validated against synthetic [os.FileMode] values on any platform
// (Copilot review acceptance M1).
//
// The integration tests that drive real FIFOs / sockets / char-device
// inodes live in `file_special_unix_test.go` behind a `//go:build unix`
// tag — Windows builds wouldn't compile the syscall.Mkfifo /
// syscall.SockaddrUnix identifiers (PR#24 review).
func TestRejectSpecialMode_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		mode    os.FileMode
		wantErr bool
		wantSub string
	}{
		{"regular_file", 0o644, false, ""},
		{"regular_file_executable", 0o755, false, ""},
		{"directory", os.ModeDir | 0o755, true, "directory"},
		{"named_pipe", os.ModeNamedPipe | 0o644, true, "named pipe"},
		{"socket", os.ModeSocket | 0o644, true, "socket"},
		{"char_device", os.ModeDevice | os.ModeCharDevice | 0o644, true, "device"},
		{"block_device", os.ModeDevice | 0o644, true, "device"},
		{"symlink_alone_passes", os.ModeSymlink | 0o644, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := rejectSpecialMode("test/path", tc.mode)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("rejectSpecialMode(%v): expected error, got nil", tc.mode)
				}
				if !errors.Is(err, ErrUnsupported) {
					t.Errorf("rejectSpecialMode(%v): want ErrUnsupported, got %v", tc.mode, err)
				}
				if tc.wantSub != "" && !contains(err.Error(), tc.wantSub) {
					t.Errorf("rejectSpecialMode(%v): error %q missing substring %q", tc.mode, err, tc.wantSub)
				}
				return
			}
			if err != nil {
				t.Errorf("rejectSpecialMode(%v): expected nil, got %v", tc.mode, err)
			}
		})
	}
}

// contains is a tiny strings.Contains substitute that avoids the import
// for one usage site; keeps the test file's import surface minimal.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
