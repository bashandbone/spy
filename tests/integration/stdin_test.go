// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// itoa is a tiny convenience for assembling the per-iteration
// fixture body inline; using strconv keeps the dep surface
// honest.
func itoa(i int) string { return strconv.Itoa(i) }

// TestStdinPipe_HighlightedGoSource is the US5 PTY-driven integration
// test (T088): spawn `spy -l go` with stdin connected to a pipe (NOT
// a PTY) carrying a Go source snippet, and stdout connected to a
// PTY. Assert the alt-screen frame surfaces the highlighted content,
// the footer reads `<stdin>` (not a basename), the down-arrow scroll
// advances the viewport, then quit cleanly.
//
// `-l go` pins the lexer so the assertion isn't sensitive to Chroma's
// content-sniff heuristic for short snippets.
//
// Renamed from TestStdinPipe_HighlightedDiff after Copilot review on
// PR#15: the test never carried diff content, so the prior name was
// misleading.
func TestStdinPipe_HighlightedGoSource(t *testing.T) {
	// 50-line Go source — enough to overflow the default 24-row
	// viewport so down-arrow scroll has somewhere to scroll TO.
	// (Earlier 7-line input fit on one screen; down-arrow was a
	// no-op and the post-scroll diff assertion always failed.)
	var src []byte
	src = append(src, "package main\n\nimport \"fmt\"\n\nfunc bar() {\n"...)
	for i := 0; i < 50; i++ {
		src = append(src, []byte("\tfmt.Println(\"line "+itoa(i)+"\")\n")...)
	}
	src = append(src, "}\n"...)
	input := src

	// Force a 256-color PTY env so the highlighter engages —
	// see the comment on colorTermEnv (text_review_test.go) for why.
	p, stdin := NewPTYProgramWithStdin(t, []string{"--no-config", "-l", "go"}, colorTermEnv(), PTYOptions{})

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
	// is sufficient — exact color is theme-dependent.
	if !regexp.MustCompile(`\x1b\[[0-9;]+m`).MatchString(frame) {
		t.Fatalf("first paint emits no SGR escapes — stdin highlighting did not engage")
	}
	if !strings.Contains(stripped, "func bar") {
		t.Fatalf("first paint missing 'func bar' content; stripped tail=%q", truncTail([]byte(stripped), 400))
	}

	// Down arrow exercises the scroll path on stdin-sourced buffers
	// (FR-002 + FR-005). Capture the pre-scroll content and assert
	// the post-scroll snapshot differs — without this, a broken
	// input handler that silently drops keystrokes would still
	// pass since "the screen renders something" trivially holds.
	preScroll := stripANSI(string(p.Snapshot()))
	for i := 0; i < 5; i++ {
		p.Send("\x1b[B")
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	postScroll := stripANSI(string(p.Snapshot()))
	if postScroll == preScroll {
		t.Fatalf("down-arrow input did not change rendered content; pre-tail=%q post-tail=%q",
			truncTail([]byte(preScroll), 200), truncTail([]byte(postScroll), 200))
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
