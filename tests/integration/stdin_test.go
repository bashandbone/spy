// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestStdinPipe_HighlightedDiff is the US5 PTY-driven integration
// test (T088): spawn `spy -l go` with stdin connected to a pipe (NOT
// a PTY) carrying a Go source snippet, and stdout connected to a
// PTY. Assert the alt-screen frame surfaces the highlighted content
// and the footer reads `<stdin>` (not a basename), then quit cleanly.
//
// `-l go` pins the lexer so the assertion isn't sensitive to Chroma's
// content-sniff heuristic for short snippets.
func TestStdinPipe_HighlightedDiff(t *testing.T) {
	input := []byte("package main\n\nimport \"fmt\"\n\nfunc bar() {\n\tfmt.Println(\"hi\")\n}\n")

	p, stdin := NewPTYProgramWithStdin(t, []string{"--no-config", "-l", "go"}, nil, PTYOptions{})

	// Write input synchronously, close to signal EOF (the loader
	// then collapses the streaming "…" to the final line count).
	if _, err := stdin.Write(input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = stdin.Close()

	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(400 * time.Millisecond)

	frame := string(p.Snapshot())
	stripped := stripANSI(frame)

	// US5 acceptance #1: the footer shows `<stdin>` (literal, not a
	// basename derived from the spawn path).
	if !strings.Contains(stripped, "<stdin>") {
		t.Fatalf("footer missing '<stdin>' marker; stripped tail=%q", truncTail([]byte(stripped), 400))
	}

	// US5 acceptance #2: highlighting engaged. Chroma emits SGR for
	// any non-mono theme, so any SGR escape near the rendered content
	// is sufficient — exact colour is theme-dependent.
	if !regexp.MustCompile(`\x1b\[[0-9;]+m`).MatchString(frame) {
		t.Fatalf("first paint emits no SGR escapes — stdin highlighting did not engage")
	}
	if !strings.Contains(stripped, "func bar") {
		t.Fatalf("first paint missing 'func bar' content; stripped tail=%q", truncTail([]byte(stripped), 400))
	}

	// Down arrow exercises the scroll path on stdin-sourced buffers
	// (FR-002 plus FR-005). The viewport is short, so any movement
	// is fine — we just want to confirm the input handler reaches
	// the renderer through the stdin source path.
	for i := 0; i < 3; i++ {
		p.Send("\x1b[B")
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)

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
