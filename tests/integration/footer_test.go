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
	"testing"
	"time"
)

// TestFooter_LineCounterAdvancesOnScroll is the US6 PTY-driven
// integration test (T096): start `spy <100-line file>`, observe the
// footer's "100 lines" / "Line 1" markers, scroll past the first
// viewport via PageDown, and assert the "Line N" counter advanced.
//
// Sub-80-column collapse coverage is layered on top via
// TestFooter_SubEightyCollapse below.
func TestFooter_LineCounterAdvancesOnScroll(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "lines.txt")
	var src bytes.Buffer
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&src, "line %03d\n", i)
	}
	if err := os.WriteFile(fixture, src.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgramOpts(t, []string{"--no-config", fixture}, nil, PTYOptions{Cols: 100, Rows: 24})
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(300 * time.Millisecond)

	// First paint should show file basename, total lines, current line 1.
	frame := string(p.Snapshot())
	if !strings.Contains(frame, "lines.txt") {
		t.Fatalf("footer missing basename; snapshot tail=%q", truncTail([]byte(frame), 400))
	}
	if !strings.Contains(frame, "100 lines") {
		t.Fatalf("footer missing '100 lines' marker (streaming may have not collapsed); snapshot tail=%q", truncTail([]byte(frame), 400))
	}
	if !strings.Contains(frame, "Line 1") {
		t.Fatalf("footer missing 'Line 1' marker; snapshot tail=%q", truncTail([]byte(frame), 400))
	}

	// Scroll past the first viewport with PageDown (xterm: \x1b[6~).
	// Issue twice so the line counter is unambiguously past 1.
	p.Send("\x1b[6~")
	time.Sleep(100 * time.Millisecond)
	p.Send("\x1b[6~")
	time.Sleep(200 * time.Millisecond)

	// Footer should now show a higher Line N. Search the snapshot
	// for any "Line N" where N > 1 — we don't pin a specific value
	// because viewport height varies with PTY config and footer
	// padding may differ.
	frame = string(p.Snapshot())
	advanced := false
	for n := 2; n <= 100; n++ {
		if strings.Contains(frame, fmt.Sprintf("Line %d", n)) {
			advanced = true
			break
		}
	}
	if !advanced {
		t.Fatalf("footer 'Line N' did not advance past 1 after PageDown; snapshot tail=%q", truncTail([]byte(frame), 400))
	}

	// Quit.
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

// TestFooter_SubEightyCollapse covers the spec.md Q4 minimum-size
// edge case: at < 80 columns, the footer collapses to the short
// "<basename> · L<N>" form (no " | " separators). We start at the
// collapsed width since SIGWINCH-driven resize is exercised by
// TestResize_PreservesViewportAnchor in resize_test.go.
func TestFooter_SubEightyCollapse(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "tiny.txt")
	if err := os.WriteFile(fixture, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgramOpts(t, []string{"--no-config", fixture}, nil, PTYOptions{Cols: 60, Rows: 20})
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(300 * time.Millisecond)

	frame := string(p.Snapshot())
	if !strings.Contains(frame, "tiny.txt") {
		t.Fatalf("collapsed footer missing basename; snapshot tail=%q", truncTail([]byte(frame), 400))
	}
	// Collapsed form uses · (U+00B7). Pinned in
	// internal/render/statusbar.go.
	if !strings.Contains(frame, "·") {
		t.Fatalf("collapsed footer missing middle-dot separator; snapshot tail=%q", truncTail([]byte(frame), 400))
	}
	// And it should NOT have the verbose " | " separator at this width.
	if strings.Contains(frame, "tiny.txt | ") {
		t.Fatalf("footer did not collapse at 60 cols; snapshot tail=%q", truncTail([]byte(frame), 400))
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
