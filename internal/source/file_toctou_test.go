// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestFileSource_OpenWithSymlinkSwap_RejectsRedirect verifies the M2
// TOCTOU mitigation: after FileSource construction succeeds against a
// regular file, swapping the path to a symlink pointing elsewhere must
// not silently redirect the next [FileSource.Open]. With O_NOFOLLOW
// the kernel returns ELOOP instead of dereferencing the swapped link
// (Copilot review acceptance M2).
//
// The test simulates the race by constructing the FileSource against a
// regular file, then atomically replacing the path with a symlink
// before invoking Open. Without the M2 fix Open() would happily read
// the symlink target — with the fix it returns an error.
func TestFileSource_OpenWithSymlinkSwap_RejectsRedirect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("O_NOFOLLOW unavailable on Windows; symlinks require admin")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	other := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(target, []byte("legit\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(other, []byte("attacker controlled\n"), 0o644); err != nil {
		t.Fatalf("write other: %v", err)
	}
	src, err := NewFileSource(target)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	// Swap `target` for a symlink → `other`. Keep the original path the
	// FileSource cached so the next Open hits the swapped link.
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove target: %v", err)
	}
	if err := os.Symlink(other, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	rc, err := src.Open()
	if err == nil {
		// The redirect succeeded — verify it didn't follow to `other`.
		defer rc.Close()
		buf, _ := io.ReadAll(rc)
		if string(buf) == "attacker controlled\n" {
			t.Fatal("Open() followed swapped symlink (TOCTOU not mitigated)")
		}
		// If it returned `other`'s content via something else, still fail.
		t.Errorf("Open() succeeded after symlink swap; got %q", buf)
		return
	}
	// Expected error path: O_NOFOLLOW causes ELOOP / ENXIO etc.
	t.Logf("Open after symlink swap returned (expected) error: %v", err)
}

// TestFileSource_RedetectClearsCache verifies the M2 ActionReload fix.
// The cached `s.detected` flag in [FileSource.detectOnce] must be
// reset so a reload re-classifies the new bytes — without this, a
// file that flipped between detection categories would still render
// through the old kind/lexer (Copilot review acceptance M2).
//
// We use an extensionless path so the hint cannot short-circuit
// detection and the swap is observable in the cached Kind.
func TestFileSource_RedetectClearsCache(t *testing.T) {
	dir := t.TempDir()
	// Use an unknown extension so the hint short-circuit in
	// detectKind doesn't fire — both detection passes run their
	// content-based classification path and the swap is observable.
	p := filepath.Join(dir, "swap.unknownext")
	// Initial content: plain text → KindText.
	if err := os.WriteFile(p, []byte("just plain text content\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := NewFileSource(p)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	if k := src.Kind(); k != KindText {
		t.Fatalf("initial Kind: got %v want KindText", k)
	}
	// Now overwrite with PDF magic bytes. detectKind sees the magic
	// signature in the bytes and classifies as KindPDF — but only on
	// a fresh detection pass.
	pdfBody := []byte("%PDF-1.4\n%fakepdf\n")
	if err := os.WriteFile(p, pdfBody, 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// Without Redetect() the cache wins → still KindText.
	if k := src.Kind(); k != KindText {
		t.Errorf("pre-Redetect Kind: expected stale KindText, got %v", k)
	}
	// After Redetect() the next Kind() call reclassifies the bytes.
	src.Redetect()
	if k := src.Kind(); k != KindPDF {
		t.Errorf("post-Redetect Kind: got %v want KindPDF", k)
	}
}

// TestFileSource_RedetectIsIdempotent ensures Redetect() can be called
// repeatedly without surprising side effects (no panic, kind stays
// consistent with the on-disk content).
func TestFileSource_RedetectIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(p, []byte("plain text\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := NewFileSource(p)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	// First detection.
	k1 := src.Kind()
	src.Redetect()
	src.Redetect() // double Redetect is safe.
	k2 := src.Kind()
	if k1 != k2 {
		t.Errorf("Kind drifted after double Redetect: %v then %v", k1, k2)
	}
}

// TestFileSource_RedetectAfterErrorPath ensures Redetect() also clears
// a previous detection error so a transient failure (file briefly
// missing) doesn't permanently stick.
func TestFileSource_RedetectAfterErrorPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "y.txt")
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := NewFileSource(p)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	// Force-fail detection by deleting the file mid-flight.
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := src.Open(); err == nil {
		t.Fatal("expected error from Open() after path removal")
	}
	// Restore content + redetect → next Open() should now succeed.
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	src.Redetect()
	rc, err := src.Open()
	if err != nil {
		t.Fatalf("Open() after Redetect+restore: %v", err)
	}
	defer rc.Close()
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	// Should classify as text now.
	if k := src.Kind(); k != KindText {
		t.Errorf("post-Redetect Kind: got %v want KindText", k)
	}
}

// Sanity: the public Source interface plus Redetect compile-time
// assertion. This ensures future refactors that move Redetect off
// FileSource flag the call site in internal/ui/update.go.
var _ interface {
	Source
	Redetect()
} = (*FileSource)(nil)
