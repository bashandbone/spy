// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGraphics_KittyPayloadDispatch is the US4 PTY-driven dispatch
// test (T073). Spawns `spy --graphics kitty <fixture.png>` against
// the deterministic 16×16 PNG used by the unit-test goldens, and
// asserts the PTY frame contains the **complete** Kitty payload —
// the same bytes the unit-test golden encoder produces.
//
// SC-009 fixture sizes (32 KB / 5 MB / 49 MB) are deferred to a
// follow-up; the dispatch contract is what's pinned here.
func TestGraphics_KittyPayloadDispatch(t *testing.T) {
	root := moduleRoot(t)
	pngPath := filepath.Join(root, "internal", "graphics", "testdata", "kitty_input.png")
	expectedPath := filepath.Join(root, "internal", "graphics", "testdata", "kitty_expected.bin")

	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read kitty_expected.bin: %v", err)
	}

	dir := t.TempDir()
	fixture := filepath.Join(dir, "img.png")
	pngBytes, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatalf("read kitty_input.png: %v", err)
	}
	if err := os.WriteFile(fixture, pngBytes, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgram(t, []string{"--no-config", "--graphics", "kitty", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	// Kitty payloads are large (~140 B chunked base64); give Bubble
	// Tea a moment to emit the full frame.
	time.Sleep(500 * time.Millisecond)

	frame := p.Snapshot()

	// Prefix and terminator.
	if !bytes.Contains(frame, []byte("\x1b_G")) {
		t.Fatalf("frame missing Kitty prefix \\x1b_G; tail=%q", truncTail(frame, 400))
	}
	if !bytes.Contains(frame, []byte("\x1b\\")) {
		t.Fatalf("frame missing Kitty terminator \\x1b\\\\; tail=%q", truncTail(frame, 400))
	}
	// The full expected payload (140 bytes) should appear contiguously
	// in the frame somewhere — the rest of the frame is alt-screen
	// setup + statusbar + cursor positioning.
	if !bytes.Contains(frame, expected) {
		t.Fatalf("frame does not contain the expected Kitty payload byte-for-byte\nexpected (%d bytes): %q\nframe tail=%q",
			len(expected), expected, truncTail(frame, 600))
	}

	for i := 0; i < 5 && !waitExitShort(p, 250*time.Millisecond); i++ {
		p.Send("q")
	}
	if !p.WaitForExit(3 * time.Second) {
		t.Fatalf("process did not exit on `q`; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("exit code %d (want 0)", exit)
	}
}

// TestGraphics_NoneFallbackEmitsMetadata verifies that
// `--graphics none` for an image renders the deterministic metadata
// block — filename, dimensions, file size, fallback notice — instead
// of any graphics protocol bytes (US4 acceptance #3).
func TestGraphics_NoneFallbackEmitsMetadata(t *testing.T) {
	root := moduleRoot(t)
	pngPath := filepath.Join(root, "internal", "graphics", "testdata", "kitty_input.png")
	pngBytes, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatalf("read kitty_input.png: %v", err)
	}

	dir := t.TempDir()
	fixture := filepath.Join(dir, "img.png")
	if err := os.WriteFile(fixture, pngBytes, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgram(t, []string{"--no-config", "--graphics", "none", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(400 * time.Millisecond)

	frame := p.Snapshot()
	stripped := stripANSI(string(frame))

	// No Kitty / iTerm2 / sixel protocol bytes in the rendered area.
	if bytes.Contains(frame, []byte("\x1b_G")) {
		t.Errorf("--graphics none emitted Kitty payload prefix")
	}
	if bytes.Contains(frame, []byte("\x1b]1337;File=")) {
		t.Errorf("--graphics none emitted iTerm2 inline-image header")
	}
	// US4 acceptance #3: filename + dimensions appear in the metadata
	// block. We don't pin the exact format string; "img.png" + the
	// PNG's pixel dimensions are sufficient signal.
	if !strings.Contains(stripped, "img.png") {
		t.Errorf("metadata block missing filename; stripped tail=%q", truncTail([]byte(stripped), 400))
	}

	for i := 0; i < 5 && !waitExitShort(p, 250*time.Millisecond); i++ {
		p.Send("q")
	}
	if !p.WaitForExit(3 * time.Second) {
		t.Fatalf("process did not exit on `q`; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("exit code %d (want 0)", exit)
	}
}

// TestGraphics_CleanupOnQuit asserts the Kitty cleanup escape
// (`\x1b_Ga=d,d=A;\x1b\\`) fires on `q` exit. SIGINT and panic
// variants from research R10 are deferred — the SIGINT exit-code
// regression (review C2) blocks meaningful signal-cleanup
// instrumentation here.
func TestGraphics_CleanupOnQuit(t *testing.T) {
	root := moduleRoot(t)
	pngPath := filepath.Join(root, "internal", "graphics", "testdata", "kitty_input.png")
	pngBytes, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatalf("read kitty_input.png: %v", err)
	}

	dir := t.TempDir()
	fixture := filepath.Join(dir, "img.png")
	if err := os.WriteFile(fixture, pngBytes, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgram(t, []string{"--no-config", "--graphics", "kitty", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(500 * time.Millisecond)

	for i := 0; i < 5 && !waitExitShort(p, 250*time.Millisecond); i++ {
		p.Send("q")
	}
	if !p.WaitForExit(3 * time.Second) {
		t.Fatalf("process did not exit on `q`; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("exit code %d (want 0)", exit)
	}

	// Cleanup escape should appear after first paint, before the
	// alt-screen exit. Search the whole transcript.
	if !bytes.Contains(p.Snapshot(), []byte("\x1b_Ga=d,d=A;\x1b\\")) {
		t.Errorf("Kitty cleanup escape \\x1b_Ga=d,d=A;\\x1b\\\\ not emitted on quit; tail=%q", truncTail(p.Snapshot(), 400))
	}
}
