// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// FromArgs in Phase 2 only constructs FileSource; stdin/StdinSource is
// added in US5 (T090–T091). When stdin would be required, the result is
// ErrNoInput so callers can produce the documented "spy: no input"
// stderr message.
//
// The compile-time `var _ source.LineProvider = (*loader.LineBuffer)(nil)`
// assertion called for in T017 is added in T020 once loader.LineBuffer
// exists; placing it here now would break the source phase's `go test
// ./internal/source/...` until the loader phase lands.

func TestFromArgs_FilePath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := FromArgs([]string{p}, nil, "")
	if err != nil {
		t.Fatalf("FromArgs: %v", err)
	}
	if src == nil {
		t.Fatal("nil source")
	}
	if src.DisplayName() != "x.txt" {
		t.Errorf("DisplayName: got %q", src.DisplayName())
	}
}

func TestFromArgs_NoArgs(t *testing.T) {
	// Phase 2: stdin construction is deferred to US5; no args + no stdin
	// → ErrNoInput regardless of TTY state.
	_, err := FromArgs(nil, nil, "")
	if err == nil {
		t.Fatal("expected ErrNoInput")
	}
	if !errors.Is(err, ErrNoInput) {
		t.Errorf("expected ErrNoInput, got %v", err)
	}
}

func TestFromArgs_DashIsNoInputUntilUS5(t *testing.T) {
	// Phase 2 contract: explicit "-" returns ErrNoInput (StdinSource is
	// US5). After US5 this test will be replaced/extended.
	_, err := FromArgs([]string{"-"}, nil, "")
	if err == nil {
		t.Fatal("expected ErrNoInput for '-' positional")
	}
	if !errors.Is(err, ErrNoInput) {
		t.Errorf("expected ErrNoInput, got %v", err)
	}
}

func TestFromArgs_FileMissing(t *testing.T) {
	_, err := FromArgs([]string{filepath.Join(t.TempDir(), "nope")}, nil, "")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFromArgs_HintPropagates(t *testing.T) {
	// Write a file with a name that wouldn't auto-detect as Go (.bak).
	dir := t.TempDir()
	p := filepath.Join(dir, "main.go.bak")
	body := "package main\nfunc main() {}\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := FromArgs([]string{p}, nil, "go")
	if err != nil {
		t.Fatalf("FromArgs: %v", err)
	}
	if src.Kind() != KindCode {
		t.Errorf("hint should classify as Code, got %v", src.Kind())
	}
}
