// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build unix

package source

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestNewFileSource_RejectsFIFO exercises the rejection on a real FIFO
// created with mkfifo(2). Unix-only: Windows lacks the FIFO concept
// and the syscall.Mkfifo identifier doesn't exist on the windows
// build, so this test lives behind `//go:build unix` rather than a
// runtime GOOS skip — without the build tag the file would fail to
// compile on Windows (PR#24 review).
func TestNewFileSource_RejectsFIFO(t *testing.T) {
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
// Unix-domain socket. Unix-only for the same compile-time reason as
// the FIFO test above.
func TestNewFileSource_RejectsSocket(t *testing.T) {
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
// device is unavailable (sandboxed CI). Unix-only because /dev/null
// doesn't exist on Windows.
func TestNewFileSource_RejectsCharDevice(t *testing.T) {
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
