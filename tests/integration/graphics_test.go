// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import "testing"

// TestGraphics_KittyPayloadDispatch is the US4 PTY-driven dispatch test
// (T073): drive the renderer with `term.Capabilities.Graphics ==
// GraphicsKitty` and assert the rendered frame contains the **complete**
// Kitty payload (prefix `\x1b_G`, the same base64 chunks the unit-test
// golden produces, terminator `\x1b\\`) — not just the prefix. Diff
// against the same golden file as T068b.
//
// Ships as a documented `t.Skip` until the PTY harness lands in Phase 9
// (T104). When the harness arrives, this test should:
//  1. Build the binary into a t.TempDir() with a fixture image
//     (the same deterministic 16×16 PNG used by
//     internal/graphics/testdata/kitty_input.png).
//  2. Spawn `./spy --graphics kitty <fixture.png>` under a PTY sized
//     80x24.
//  3. Wait for the alt-screen entry sequence (\x1b[?1049h).
//  4. Capture the first paint and assert it matches
//     internal/graphics/testdata/kitty_expected.bin byte-for-byte.
//  5. Send `q` and assert the process exits with code 0 AND the
//     Kitty cleanup escape (`\x1b_Ga=d,d=A;\x1b\\`) was emitted before
//     the alt-screen exit.
//
// Repeat for iTerm2 (--graphics iterm2) and sixel (--graphics sixel)
// against internal/graphics/testdata/{iterm2,sixel}_expected.bin.
//
// Repeat for `--graphics none`: the first paint must be the deterministic
// metadata block (no ANSI escape sequences in the rendered area).
func TestGraphics_KittyPayloadDispatch(t *testing.T) {
	t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")
}

// TestGraphics_CleanupOnQuit asserts the Kitty cleanup escape fires on
// `q`, on SIGINT, AND on panic (research R10). The synthetic panic is
// produced by replacing `pdfRenderer.Render` with a wrapper that panics
// mid-render so the deferred cleanup chain in cmd/spy/main.go is
// exercised across every exit path.
//
// Until the harness lands, the assertions are documented here so a
// future PR can lift them into runnable code without re-deriving the
// contract from research.md.
func TestGraphics_CleanupOnQuit(t *testing.T) {
	t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")
}
