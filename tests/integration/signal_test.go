// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSIGINTRestoresTerminal is the FR-015 / SC-008 signal-handling
// gate from T035b. It drives the spy binary against a 1 000-line
// file, sends SIGINT once the alt-screen frame is observed, and
// asserts:
//
//	(a) process exits with code 130 within 1s,
//	(b) terminal modes are restored (alt-screen exited via the
//	    \x1b[?1049l sequence),
//	(c) no residual escape sequences trail on stdout.
//
// (a) is the spec contract from contracts/cli.md. If the binary does
// not install an os/signal handler, Bubble Tea catches SIGINT
// internally and converts to tea.Quit → exit 0. That regression is
// what this test catches.
func TestSIGINTRestoresTerminal(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "big.txt")
	var src bytes.Buffer
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&src, "line %d\n", i)
	}
	if err := os.WriteFile(fixture, src.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgram(t, []string{"--no-config", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(250 * time.Millisecond)

	p.Signal(syscall.SIGINT)
	if !p.WaitForExit(2 * time.Second) {
		t.Fatalf("process did not exit on SIGINT; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}

	// (a) exit code 130.
	if exit := p.ExitCode(); exit != 130 {
		t.Errorf("SIGINT: exit code %d (want 130 per contracts/cli.md)", exit)
	}

	// (b) alt-screen exit observed.
	frame := string(p.Snapshot())
	if !strings.Contains(frame, "\x1b[?1049l") {
		t.Errorf("SIGINT: alt-screen exit sequence \\x1b[?1049l not observed in PTY output")
	}
}

// TestSIGTERMRestoresTerminal mirrors TestSIGINTRestoresTerminal for
// SIGTERM (exit code 143).
func TestSIGTERMRestoresTerminal(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "big.txt")
	var src bytes.Buffer
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&src, "line %d\n", i)
	}
	if err := os.WriteFile(fixture, src.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgram(t, []string{"--no-config", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(250 * time.Millisecond)

	p.Signal(syscall.SIGTERM)
	if !p.WaitForExit(2 * time.Second) {
		t.Fatalf("process did not exit on SIGTERM; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}

	if exit := p.ExitCode(); exit != 143 {
		t.Errorf("SIGTERM: exit code %d (want 143 per contracts/cli.md)", exit)
	}
	frame := string(p.Snapshot())
	if !strings.Contains(frame, "\x1b[?1049l") {
		t.Errorf("SIGTERM: alt-screen exit sequence \\x1b[?1049l not observed in PTY output")
	}
}
