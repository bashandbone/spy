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
// runway), wait for the first content frame (which can only be painted
// after Bubble Tea's input reader is subscribed to stdin), send `q`
// once, expect exit.
//
// Root-cause fix (M7): rather than sleeping after the alt-screen
// prologue (which is emitted BEFORE Bubble Tea's cancelreader
// subscription), we wait for "line" — actual viewport content from the
// loader's first chunk. Rendered content can only appear after the
// event loop has processed a WindowSizeMsg, which is dispatched after
// initCancelReader returns, so this is a reliable "input pipeline live"
// signal.
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
	// Wait for actual viewport content: "line" appears in every row of
	// the rendered file and can only reach the PTY after the event loop
	// has processed the initial WindowSizeMsg (which is dispatched
	// after initCancelReader). This is the reliable input-ready signal.
	if !p.WaitFor("line", 5*time.Second) {
		t.Fatalf("first content frame not observed; snapshot=%q", string(p.Snapshot()))
	}
	p.Send("q")
	exited := p.WaitForExit(500 * time.Millisecond)
	if !exited {
		// Safety-net: ctrl-c if `q` still didn't propagate.
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
//
// Root-cause fix (M7): the previous implementation slept 250 ms after
// the alt-screen prologue and then retransmitted `q` in a loop because
// the prologue escapes (including \x1b[?2004h) are emitted BEFORE
// Bubble Tea's cancelreader subscribes to stdin. Instead we now wait
// for "2 lines" — the streaming-complete footer for the 2-line
// fixture. Rendered content can only appear after the event loop has
// processed both the WindowSizeMsg and the streamDoneMsg/metaUpdatedMsg
// sequence, all of which run after initCancelReader returns. This
// eliminates the race window entirely.
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
	// Wait for the streaming-complete footer ("2 lines") — this is the
	// reliable input-ready signal (see comment above).
	if !p.WaitFor("2 lines", 5*time.Second) {
		t.Fatalf("streaming-complete footer not observed; snapshot=%q", string(p.Snapshot()))
	}
	p.Send("q")
	if !p.WaitForExit(5 * time.Second) {
		t.Fatalf("process did not exit on `q`; snapshot=%q", string(p.Snapshot()))
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("exit code %d (want 0)", exit)
	}
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
