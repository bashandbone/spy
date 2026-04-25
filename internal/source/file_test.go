// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestFileSource_Open(t *testing.T) {
	p := writeTempFile(t, "hello.go", "package main\n")
	src, err := NewFileSource(p)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	r, err := src.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(b) != "package main\n" {
		t.Errorf("read content: %q", b)
	}
	if src.Kind() != KindCode {
		t.Errorf("Kind: got %v want %v", src.Kind(), KindCode)
	}
	if src.DisplayName() != "hello.go" {
		t.Errorf("DisplayName: got %q want %q", src.DisplayName(), "hello.go")
	}
}

func TestFileSource_OpenIsRepeatable(t *testing.T) {
	p := writeTempFile(t, "a.txt", "hello")
	src, err := NewFileSource(p)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	for i := 0; i < 3; i++ {
		r, err := src.Open()
		if err != nil {
			t.Fatalf("Open #%d: %v", i, err)
		}
		_, _ = io.ReadAll(r)
		r.Close()
	}
}

func TestFileSource_Reopen(t *testing.T) {
	p := writeTempFile(t, "a.txt", "abcdef")
	src, err := NewFileSource(p)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	rs, err := src.Reopen()
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer func() {
		if c, ok := rs.(io.Closer); ok {
			c.Close()
		}
	}()
	if _, err := rs.Seek(2, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	b := make([]byte, 4)
	if _, err := io.ReadFull(rs, b); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(b) != "cdef" {
		t.Errorf("seeked read: got %q want %q", b, "cdef")
	}
}

func TestFileSource_Missing(t *testing.T) {
	_, err := NewFileSource(filepath.Join(t.TempDir(), "does_not_exist.txt"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFileSource_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	p := writeTempFile(t, "secret.txt", "shh")
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	src, err := NewFileSource(p)
	if err != nil {
		// NewFileSource may stat() first; permission error is acceptable here.
		if !errors.Is(err, ErrPermission) {
			t.Errorf("expected ErrPermission from NewFileSource, got %v", err)
		}
		return
	}
	// If NewFileSource succeeded, the error should surface on Open().
	_, err = src.Open()
	if err == nil {
		t.Fatal("expected permission error on Open")
	}
	if !errors.Is(err, ErrPermission) {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

func TestFileSource_BrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := NewFileSource(link)
	if err == nil {
		t.Fatal("expected error for broken symlink")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFileSource_BinaryRejected(t *testing.T) {
	// Build a file whose content is mostly NUL bytes; detectKind should
	// flag it as binary on Open() rather than crash later.
	p := filepath.Join(t.TempDir(), "blob.bin")
	body := make([]byte, 9000)
	for i := range body {
		body[i] = 0x00
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := NewFileSource(p)
	if err == nil {
		// Construction does NOT inspect content (we only stat); the binary
		// rejection happens during detectKind on Open. Either is acceptable
		// per contract — but the error must wrap ErrBinary.
		_, err = src.Open()
	}
	if err == nil {
		t.Fatal("expected ErrBinary somewhere in the construction/open path")
	}
	if !errors.Is(err, ErrBinary) {
		t.Errorf("expected ErrBinary, got %v", err)
	}
}

func TestFileSource_Metadata(t *testing.T) {
	p := writeTempFile(t, "m.txt", "content")
	src, err := NewFileSource(p)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	meta := src.Metadata()
	if meta.Path != p {
		t.Errorf("meta.Path: got %q want %q", meta.Path, p)
	}
	if meta.Size != 7 {
		t.Errorf("meta.Size: got %d want 7", meta.Size)
	}
	if meta.LineCount != -1 {
		t.Errorf("meta.LineCount before streaming: got %d want -1", meta.LineCount)
	}
}
