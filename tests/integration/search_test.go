// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSearch_NavigatesAndJumps is the US2 PTY-driven integration test
// (T051): drive the search prompt against a 10 000-line file (one
// number per line), navigate through matches, and exercise `:`-jump
// aliases.
//
// The literal "9999" appears exactly once in the buffer (the line
// that *is* 9999), which makes the wrap behavior observable: pressing
// `n` after the first match must wrap and surface the "search wrapped"
// status.
func TestSearch_NavigatesAndJumps(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "big.txt")

	var src bytes.Buffer
	for i := 1; i <= 10000; i++ {
		fmt.Fprintf(&src, "%d\n", i)
	}
	if err := os.WriteFile(fixture, src.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgram(t, []string{"--no-config", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(300 * time.Millisecond)

	// 1. Forward search: /9999<Enter> jumps to the line containing
	//    "9999". Send-with-retry covers the dropped-first-keystroke
	//    issue documented in pty_sanity_test.go — a swallowed leading
	//    `/` leaves the prompt closed and the keystrokes consumed as
	//    no-op key bindings, so we retry until the viewport actually
	//    moves past the initial top-of-file paint.
	if !sendUntil(p, "/9999\r", "9999", 5*time.Second) {
		t.Fatalf("search /9999 did not surface match; snapshot tail=%q", truncTail(p.Snapshot(), 600))
	}

	// 2. :1<Enter> jumps back to top. The `:1\r` command goes through
	//    the prompt, so dismissing the prompt rewrites the ENTIRE footer
	//    row (prompt style → normal footer), making "Line 1" a
	//    contiguous string in the diff output. Drop the trailing space:
	//    the footer ends with \x1b[K (erase-to-EOL), not a space.
	if !sendUntil(p, ":1\r", "Line 1", 3*time.Second) {
		t.Fatalf(":1 jump did not return to top; snapshot tail=%q", truncTail(p.Snapshot(), 600))
	}

	// 3. :$<Enter> jumps to last line.
	if !sendUntil(p, ":$\r", "10000", 3*time.Second) {
		t.Fatalf(":$ jump did not reach end; snapshot tail=%q", truncTail(p.Snapshot(), 600))
	}

	// 4. Re-search /9999 then `n` — the wrap-around path. With one
	//    literal "9999" in the buffer, `n` immediately wraps; the
	//    UI surfaces "search wrapped" as a status advisory. We
	//    assert on the advisory text rather than viewport state
	//    because the cursor stays on the same match across wrap.
	if !sendUntil(p, "/9999\r", "9999", 5*time.Second) {
		t.Fatalf("re-search /9999 did not surface match; snapshot tail=%q", truncTail(p.Snapshot(), 600))
	}
	if !sendUntil(p, "n", "search wrapped", 3*time.Second) {
		t.Fatalf("`n` did not surface 'search wrapped' advisory; snapshot tail=%q", truncTail(p.Snapshot(), 600))
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

// TestSearch_VimMode_GG_G exercises the vim-mode jump bindings
// against the same 10 000-line fixture.
//
// `--vim` enables `gg` and `G` on top of the default arrow keymap.
// The default arrow keys still work (additive, not replacement) but
// this test focuses on the vim jump bindings covered below.
// Ctrl-D / Ctrl-U coverage is deferred — they're bound (default.go
// `ActionHalfPage{Up,Down}` routed via `WithVim`) but the half-page
// distance assertion needs viewport-anchor introspection that the
// current PTY harness doesn't expose.
func TestSearch_VimMode_GG_G(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "big.txt")
	var src bytes.Buffer
	for i := 1; i <= 10000; i++ {
		fmt.Fprintf(&src, "%d\n", i)
	}
	if err := os.WriteFile(fixture, src.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgram(t, []string{"--no-config", "--vim", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(300 * time.Millisecond)

	// G → jump to last line.
	if !sendUntil(p, "G", "10000", 3*time.Second) {
		t.Fatalf("G did not jump to end; snapshot tail=%q", truncTail(p.Snapshot(), 600))
	}

	// gg → jump back to top. Two-keystroke vim sequence; bundle as a
	// single Send so the keymap state machine sees both presses
	// without intervening time-debounced inputs from the harness.
	// BT v2's cell diff only writes the changed footer digit ("1"),
	// not "Line 1" as a contiguous string. Use viewport content
	// instead: line 1 of the 10000-line file renders as "    1  1"
	// (5-wide gutter + 2-space separator + content "1"), which IS
	// fully written because all viewport cells change on gg-scroll.
	if !sendUntil(p, "gg", "    1  1", 3*time.Second) {
		t.Fatalf("gg did not return to top; snapshot tail=%q", truncTail(p.Snapshot(), 600))
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

// sendUntil works around the dropped-first-keystroke quirk
// documented in pty_sanity_test.go (the first keystroke after first
// paint is occasionally consumed before Bubble Tea has finished
// installing its raw-mode handlers). It sends `keys` byte-by-byte
// with a small inter-byte gap (so multi-byte command sequences like
// "/9999\r" register as discrete key events through the prompt state
// machine), then polls Snapshot for `needle` in the post-baseline
// slice. If the needle doesn't appear within ~700ms it retries the
// full sequence, up to 8 times within the overall timeout.
func sendUntil(p *PTYProgram, keys, needle string, timeout time.Duration) bool {
	target := []byte(needle)
	deadline := time.Now().Add(timeout)
	tries := 0
	for time.Now().Before(deadline) {
		baseline := len(p.Snapshot())
		// Send byte-by-byte. Bubble Tea's input parser consumes one
		// keystroke per loop iteration; tiny gaps let the prompt
		// state machine settle between transitions (open / runes /
		// submit) so a `/`-then-`9999`-then-`\r` sequence isn't
		// re-batched into a single multi-rune keystroke that bypasses
		// the prompt opening.
		for i := 0; i < len(keys); i++ {
			p.Send(string(keys[i]))
			time.Sleep(15 * time.Millisecond)
		}
		windowEnd := time.Now().Add(700 * time.Millisecond)
		for time.Now().Before(windowEnd) && time.Now().Before(deadline) {
			snap := p.Snapshot()
			if len(snap) > baseline && bytes.Contains(snap[baseline:], target) {
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		tries++
		if tries >= 8 {
			break
		}
	}
	return false
}
