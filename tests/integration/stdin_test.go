// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import "testing"

// TestStdinPipe_HighlightedDiff is the US5 PTY-driven integration test
// (T088): spawn `spy` with stdin connected to a pipe carrying a Go diff
// and stdout connected to a PTY, observe an alt-screen frame whose
// footer reads `<stdin>`, scroll through the buffered content, and
// exit cleanly with `q`.
//
// It ships as a documented `t.Skip` until the PTY harness lands in
// Phase 9 (T104) — same staging strategy as TestTextReview_HighlightedFile
// (T040), TestSearch (T051), TestTheme (T062), and TestSignal (T035b).
//
// When the harness arrives this test should:
//  1. Build the binary into a t.TempDir() and prepare a fixture diff
//     in memory (e.g., `diff --git a/foo.go b/foo.go\n+func bar() {}\n`).
//  2. Spawn `./spy -l go` with stdin connected to a pipe (NOT a PTY)
//     and stdout connected to a PTY sized 80x24.
//  3. Write the diff bytes to the stdin pipe and close it (so the
//     loader sees EOF and the streaming indicator collapses).
//  4. Wait for the alt-screen entry sequence (\x1b[?1049h) on the PTY.
//  5. Wait for the first frame and assert it contains:
//     - A line beginning with the Go keyword `func` highlighted with
//     an SGR escape sequence (any colour from the dark theme).
//     - The literal substring `<stdin>` somewhere in the footer row.
//  6. Send the Down-arrow byte sequence (\x1b[B) and assert that the
//     viewport advances (gutter line number changes).
//  7. Send `q` and assert the process exits with code 0 within 1 s.
//
// Until the harness lands, the assertions are documented here so future
// PRs can lift them into runnable code without re-deriving the
// contract.
func TestStdinPipe_HighlightedDiff(t *testing.T) {
	t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")
}
