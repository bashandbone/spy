// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import "testing"

// TestFooter_LineCounterAdvancesOnScroll is the US6 PTY-driven
// integration test (T096): start `spy <multi-line file>`, scroll down,
// and assert the status-bar's "Line N" counter advances correctly. It
// ships as a documented `t.Skip` until the PTY harness real
// implementation lands in Phase 9 (T104) — same staging strategy as
// TestTextReview_HighlightedFile and TestSignal.
//
// When the harness arrives, this test should:
//  1. Build the binary into a t.TempDir() and write a 100-line text
//     file (each line carries its own line number for easy assertions:
//     "line 001\n", "line 002\n", …).
//  2. Spawn `./spy <file>` under a PTY sized 100x24.
//  3. Wait for the alt-screen entry sequence (\x1b[?1049h) on the PTY.
//  4. Wait for the first frame and assert the footer matches the
//     contracts/cli.md status-bar shape:
//     - Contains the file basename.
//     - Contains "100 lines" (no streaming "…" once the loader has
//     marked the source complete).
//     - Contains "Line 1" (the top of the buffer).
//  5. Send PageDown (\x1b[6~) twice to scroll past the first viewport.
//  6. Wait for the next frame and assert the footer's "Line N" advanced
//     to a value > 1 and consistent with the visible top-row content
//     (e.g. if the screen now leads with "line 040", the footer says
//     "Line 40").
//  7. Send a SIGWINCH-equivalent resize to 60x24 (below the 80-column
//     collapse boundary) and assert the next frame's footer collapses
//     to "<basename> · L<N>" form (no " | " separators).
//  8. Send `q` and assert the process exits with code 0 within 1 s.
//
// Until the harness lands, the assertions are documented here so future
// PRs can lift them into runnable code without re-deriving the contract.
func TestFooter_LineCounterAdvancesOnScroll(t *testing.T) {
	t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")
}
