// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// TestRejectSpecialMode_TableDriven exercises the mode-bit predicates
// that gate FileSource construction. We use [rejectSpecialMode]
// directly so the test is portable: synthesizing a real FIFO requires
// `mkfifo` and a Unix host, but the mode-bit logic is pure and can be
// validated against synthetic [os.FileMode] values on any platform
// (Copilot review acceptance M1).
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

// TestNewFileSource_RejectsFIFO exercises the rejection on a real FIFO
// created with mkfifo(2). Only meaningful on Unix; Windows lacks the
// FIFO concept so we skip there.
func TestNewFileSource_RejectsFIFO(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are POSIX-only")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "myfifo")
	// 0o644 is enough — we don't actually open both ends, just stat it.
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(fifo) })

	_, err := NewFileSource(fifo)
	if err == nil {
		t.Fatal("expected error for FIFO source")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported for FIFO, got %v", err)
	}
}

// TestNewFileSource_RejectsSocket exercises the rejection on a real
// Unix-domain socket. Only meaningful on Unix; on Windows the path
// would not be a Mode()&os.ModeSocket inode anyway.
func TestNewFileSource_RejectsSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-domain sockets are POSIX-only here")
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")
	// Bind a Unix-domain socket to materialise a socket inode.
	addr := &syscall.SockaddrUnix{Name: sock}
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Skipf("socket(2) unavailable: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Close(fd) })
	if err := syscall.Bind(fd, addr); err != nil {
		t.Skipf("bind(2) unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(sock) })

	_, err = NewFileSource(sock)
	if err == nil {
		t.Fatal("expected error for socket source")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported for socket, got %v", err)
	}
}

// TestNewFileSource_RejectsCharDevice exercises rejection of a
// well-known character device — /dev/null on Unix. Skipped where the
// device is unavailable (Windows, sandboxed CI).
func TestNewFileSource_RejectsCharDevice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/dev/null is POSIX-only")
	}
	const dev = "/dev/null"
	info, err := os.Stat(dev)
	if err != nil {
		t.Skipf("%s unavailable: %v", dev, err)
	}
	if info.Mode()&os.ModeDevice == 0 {
		t.Skipf("%s is not a device on this host", dev)
	}
	_, err = NewFileSource(dev)
	if err == nil {
		t.Fatal("expected error for /dev/null source")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported for /dev/null, got %v", err)
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
