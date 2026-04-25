// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import "testing"

// TestSearch_NavigatesAndJumps is the US2 PTY-driven integration test
// (T051): drive the search prompt against `big.txt`, navigate through
// matches, and exercise `:`-jump aliases. Ships as a documented
// `t.Skip` until the PTY harness lands in Phase 9 (T104) — same
// staging strategy as TestTextReview_HighlightedFile in this package.
//
// When the harness arrives, this test should:
//  1. Build the binary into a t.TempDir() and prepare big.txt with
//     `seq 1 10000 > big.txt` in the same dir.
//  2. Spawn `./spy big.txt` under a PTY sized 80x24.
//  3. Wait for the alt-screen entry sequence (\x1b[?1049h).
//  4. Send `/9999\r` and assert the next frame contains "9999"
//     visibly inside the viewport (not below the fold).
//  5. Send `:1\r` and assert the next frame starts back at line 1.
//  6. Send `:$\r` and assert the next frame ends at line 10000.
//  7. Send `n` and assert the search-wrap status appears within 5
//     frames (since there's only one literal "9999" and `n` will wrap).
//  8. Send `q` and assert the process exits with code 0.
//
// Vim mode coverage is layered on top:
//  9. Re-spawn `./spy --vim big.txt` and assert that:
//     - `gg` jumps to line 1 (one source line).
//     - `G` jumps to the last line.
//     - `Ctrl-D` advances by half a page.
//     - `Ctrl-U` retreats by half a page.
//
// Until the harness lands, the assertions are documented here so a
// future PR can lift them into runnable code without re-deriving the
// contract from contracts/keys.md.
func TestSearch_NavigatesAndJumps(t *testing.T) {
	t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")
}
