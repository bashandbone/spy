// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import "testing"

// TestTextReview_HighlightedFile is the US1 PTY-driven integration
// test (T040): start `spy ./hello.go`, observe an alt-screen frame
// with Go syntax colours, scroll down via arrow key, and exit cleanly
// with `q`. It ships as a documented `t.Skip` until the PTY harness
// real implementation lands in Phase 9 (T104) — same staging strategy
// as TestSignal in tests/integration/signal_test.go.
//
// When the harness arrives, this test should:
//  1. Build the binary into a t.TempDir() and copy
//     tests/e2e/fixtures/hello.go into the same dir.
//  2. Spawn `./spy hello.go` under a PTY sized 80x24.
//  3. Wait for the alt-screen entry sequence (\x1b[?1049h) on the PTY.
//  4. Wait for the first frame and assert it contains:
//     - The Go keyword "package" with an SGR escape sequence around
//     it (any colour from the dark theme is acceptable).
//     - The literal substring "fmt.Println".
//     - A line-number gutter row beginning with "1  ".
//  5. Send the Down-arrow byte sequence (\x1b[B) and assert that the
//     next frame's gutter starts with "2  " or higher.
//  6. Send `q` and assert the process exits with code 0 within 1 s.
//
// Until the harness lands, the assertions are documented here so future
// PRs can lift them into runnable code without re-deriving the contract.
func TestTextReview_HighlightedFile(t *testing.T) {
	t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")
}
