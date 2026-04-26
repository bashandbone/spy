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

// FromArgs is the entry point picking between FileSource (US1) and
// StdinSource (US5). The resolution table is in contracts/cli.md. The
// tests below exercise every distinguishable cell.
//
// Stdin TTY-state is detected via xterm.IsTerminal(int(stdin.Fd())); the
// pipe-backed `*os.File` from os.Pipe() reports false (i.e., non-TTY)
// which is the path the integration suite drives in CI. Tests passing
// `nil` exercise the "no stdin available at all" branch the binary uses
// when a caller explicitly forwards no stream.

// pipeFile returns an `*os.File` that satisfies "non-TTY stdin" without
// requiring a real pty. The read end of an os.Pipe is a regular fd that
// `xterm.IsTerminal` reports as false.
func pipeFile(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	if _, err := w.WriteString("hello from pipe\n"); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("pipe close: %v", err)
	}
	return r
}

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

func TestFromArgs_NoArgsNoStdin(t *testing.T) {
	// Both args and stdin are empty: nothing to display. ErrNoInput is
	// the documented exit-2 condition.
	_, err := FromArgs(nil, nil, "")
	if err == nil {
		t.Fatal("expected ErrNoInput")
	}
	if !errors.Is(err, ErrNoInput) {
		t.Errorf("expected ErrNoInput, got %v", err)
	}
}

func TestFromArgs_NoArgsNonTTYStdin(t *testing.T) {
	// `... | spy` shape: no args, stdin is a pipe (non-TTY) →
	// StdinSource auto-selected.
	src, err := FromArgs(nil, pipeFile(t), "")
	if err != nil {
		t.Fatalf("FromArgs: %v", err)
	}
	if src == nil {
		t.Fatal("nil source")
	}
	if src.DisplayName() != "<stdin>" {
		t.Errorf("DisplayName: got %q want %q", src.DisplayName(), "<stdin>")
	}
}

func TestFromArgs_DashForcesStdin(t *testing.T) {
	// `spy -` shape: explicit dash forces StdinSource regardless of
	// TTY state. We pass the pipe so detection works in tests; in
	// production this is `os.Stdin` and would block on a TTY until
	// Ctrl-D, which is the documented contract.
	src, err := FromArgs([]string{"-"}, pipeFile(t), "")
	if err != nil {
		t.Fatalf("FromArgs: %v", err)
	}
	if src == nil {
		t.Fatal("nil source")
	}
	if src.DisplayName() != "<stdin>" {
		t.Errorf("DisplayName: got %q want %q", src.DisplayName(), "<stdin>")
	}
}

func TestFromArgs_FileWinsOverStdin(t *testing.T) {
	// File argument plus a non-TTY stdin: file wins, stdin is ignored.
	// Per contracts/cli.md "When a file argument is present, stdin is
	// never read, even if it is non-TTY."
	p := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := FromArgs([]string{p}, pipeFile(t), "")
	if err != nil {
		t.Fatalf("FromArgs: %v", err)
	}
	if src.DisplayName() != "x.txt" {
		t.Errorf("file should win; DisplayName got %q", src.DisplayName())
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
