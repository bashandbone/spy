// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestTextReview_HighlightedFile is the US1 PTY-driven integration
// test (T040): start `spy hello.go`, observe an alt-screen frame with
// Go syntax colours, scroll down via arrow key, and exit cleanly with
// `q`.
//
// SGR escape detection is permissive: any non-default foreground SGR
// adjacent to a Go keyword satisfies the "highlighted" contract — the
// exact colour is theme-dependent and not pinned here (theme pinning
// is TestTheme_OverridePrecedence's job).
func TestTextReview_HighlightedFile(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "hello.go")

	var src bytes.Buffer
	src.WriteString("package main\n\nimport \"fmt\"\n\nfunc main() {\n")
	for i := 0; i < 95; i++ {
		fmt.Fprintf(&src, "\tfmt.Println(\"line %d\")\n", i)
	}
	src.WriteString("}\n")
	if err := os.WriteFile(fixture, src.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Force a 256-color-capable PTY env so the highlighter actually
	// emits SGR escapes — GitHub Actions runners don't set TERM by
	// default, and `term.Detect` defaults to mono when COLORTERM /
	// TERM=*-256color are both absent.
	p := NewPTYProgram(t, []string{"--no-config", fixture}, colorTermEnv())
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot=%q", truncTail(p.Snapshot(), 200))
	}
	// Give Bubble Tea a moment to install raw mode + finish first paint.
	time.Sleep(300 * time.Millisecond)

	frame := string(p.Snapshot())
	// Permissive SGR-near-keyword check: at least one SGR escape should
	// appear in the rendered frame. Chroma always emits SGR for any
	// non-mono theme; absence here means highlighting failed.
	if !regexp.MustCompile(`\x1b\[[0-9;]+m`).MatchString(frame) {
		t.Fatalf("first paint contains no SGR escapes — highlighting did not engage")
	}
	// Substring assertions over the rendered text ignore SGR escapes.
	// Chroma splits identifiers across SGR boundaries (e.g.
	// "fmt"+RESET+SET+"Println") so a literal substring search over the
	// raw bytes misses them; strip first.
	stripped := stripANSI(frame)
	if !strings.Contains(stripped, "package") {
		t.Fatalf("first paint missing 'package' keyword; stripped tail=%q", truncTail([]byte(stripped), 400))
	}
	if !strings.Contains(stripped, "fmt.Println") {
		t.Fatalf("first paint missing 'fmt.Println'; stripped tail=%q", truncTail([]byte(stripped), 400))
	}

	// Scroll down via arrow key. Down-arrow in xterm: \x1b[B.
	// Send several to make the change observable past any latency.
	for i := 0; i < 5; i++ {
		p.Send("\x1b[B")
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)

	// Quit. Loop with retries — first keystroke after first paint is
	// known to be dropped occasionally (see pty_sanity_test.go).
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

// truncTail returns the last n bytes of b for snapshot diagnostics in
// failure messages — full snapshots can be tens of KB.
func truncTail(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[len(b)-n:])
}

// stripANSI is shared across integration tests in the same package
// (declared in helpers.go).

// colorTermEnv returns the minimal environment overrides that force
// the spawned spy binary to detect a 256-color-capable terminal
// WITHOUT triggering Bubble Tea's interactive terminal probes.
//
// We set only COLORTERM=truecolor — internal/term.detectColorDepth
// short-circuits to ColorTrueColor when COLORTERM is set, so spy's
// renderer engages without needing TERM. Setting TERM=xterm-256color
// would also work for spy but causes Bubble Tea (via termenv) to
// query OSC 11 + cursor position interactively, hanging indefinitely
// when the test PTY doesn't run a responder. Leaving TERM empty
// keeps Bubble Tea in its conservative no-probe path.
//
// COLORFGBG=15;0 short-circuits any termenv background probe by
// providing the foreground+background colour directly (per the
// xterm-style env contract termenv reads).
//
// Without these overrides, GH Actions runners don't carry COLORTERM
// in their CI shell and `term.Detect` returns ColorMono — every
// highlighter-dependent assertion (US1, US3, US5) fails on CI even
// though the product is correct.
func colorTermEnv() map[string]string {
	return map[string]string{
		"COLORTERM": "truecolor",
		"COLORFGBG": "15;0",
	}
}
