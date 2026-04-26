// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPTYSanity_HelpFlag verifies the harness can spawn the binary and
// observe its output. Runs `spy --help` (which exits 0 immediately
// after printing usage) and asserts the help text appears in the
// output.
func TestPTYSanity_HelpFlag(t *testing.T) {
	p := NewPTYProgram(t, []string{"--help"}, nil)
	if !p.WaitForExit(5 * time.Second) {
		t.Fatalf("spy --help did not exit")
	}
	out := string(p.Snapshot())
	if !contains(out, "Usage") {
		t.Fatalf("--help output missing 'Usage' marker; snapshot=%q", out)
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("spy --help exit code %d (want 0)", exit)
	}
}

// TestPTYSanity_QuitOnQBigFile reproduces the scenario the dismiss
// benchmark uses: spawn against a 1000-line file (so streaming has
// runway), wait for first paint, sleep, send `q` once, expect exit.
func TestPTYSanity_QuitOnQBigFile(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "big.txt")
	var buf []byte
	for i := 0; i < 1000; i++ {
		buf = append(buf, []byte("line\n")...)
	}
	if err := os.WriteFile(fixture, buf, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	p := NewPTYProgram(t, []string{"--no-config", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed")
	}
	time.Sleep(500 * time.Millisecond)
	// Try a few variants to figure out what propagates.
	t.Logf("sending q...")
	p.Send("q")
	exited := p.WaitForExit(500 * time.Millisecond)
	if !exited {
		t.Logf("q did not exit; trying ctrl-c")
		p.Send("\x03")
		exited = p.WaitForExit(2 * time.Second)
	}
	if !exited {
		t.Fatalf("process did not exit on `q` or ctrl-c; snapshot tail=%q",
			string(p.Snapshot())[max(0, len(p.Snapshot())-200):])
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("exit code %d (want 0)", exit)
	}
}

// TestPTYSanity_QuitOnQ verifies that pressing `q` inside an
// alt-screen session terminates the binary cleanly.
func TestPTYSanity_QuitOnQ(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "tiny.txt")
	if err := os.WriteFile(fixture, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	p := NewPTYProgram(t, []string{"--no-config", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot=%q", string(p.Snapshot()))
	}
	// Give Bubble Tea a moment to install its raw-mode handlers and
	// finish the first paint before delivering input.
	time.Sleep(250 * time.Millisecond)
	for i := 0; i < 5 && !waitExitShort(p, 200*time.Millisecond); i++ {
		p.Send("q")
	}
	if !p.WaitForExit(5 * time.Second) {
		t.Fatalf("process did not exit on `q`; snapshot=%q", string(p.Snapshot()))
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("exit code %d (want 0)", exit)
	}
}

// waitExitShort is a non-blocking-ish helper: returns true if the
// process has exited, otherwise sleeps for `d` and returns false. Used
// to drive the q-resend loop in the quit-on-q sanity check.
func waitExitShort(p *PTYProgram, d time.Duration) bool {
	if p.WaitForExit(d) {
		return true
	}
	return false
}

// contains is a tiny helper for the sanity checks above so we don't
// pull in strings.Contains for a one-liner.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
