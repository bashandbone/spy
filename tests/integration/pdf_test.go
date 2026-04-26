// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import "testing"

// TestPDF_GraphicsAndTextFallback is the US4 PTY-driven PDF test
// (T072b — covers SC-010): two scenarios driven through the PTY harness
// against tests/e2e/fixtures/dummy.pdf (the W3C single-page sample with
// the sentinel string "Dummy PDF file") and
// tests/e2e/fixtures/multi-page.pdf (3 pages built by merging the W3C
// dummy 3 times via pdfcpu's Merge API).
//
// Ships as a documented `t.Skip` until the PTY harness lands in Phase 9
// (T104). When the harness arrives, this test should:
//
//	(a) Under `-tags fitz` in a Kitty-capable PTY:
//	    1. Spawn `./spy --graphics kitty tests/e2e/fixtures/multi-page.pdf`.
//	    2. Wait for the alt-screen entry sequence (\x1b[?1049h).
//	    3. Capture the first paint and assert it contains the Kitty
//	       graphics protocol prefix `\x1b_G`.
//	    4. Decode the emitted Kitty payload through the harness's
//	       reference decoder (chunked base64 → PNG). Assert the result
//	       is a non-empty `image.Image` with non-zero dimensions.
//	    5. Send `]` to advance Page from 1 to 2; assert the next paint
//	       re-renders (the cached frame must invalidate on page change).
//	    6. Send `q`, assert exit 0, and verify resident memory ≤ 250 MB.
//
//	(b) In a non-graphics PTY (any build tag):
//	    1. Spawn `./spy --graphics none tests/e2e/fixtures/dummy.pdf`.
//	    2. Wait for alt-screen entry.
//	    3. Assert the rendered frame contains the literal substring
//	       "Dummy PDF file" (the pdfcpu page-text extraction path; on
//	       the lebowsky/pdf reader the substring survives).
//	    4. Send `q`, assert exit 0, and verify resident memory ≤ 250 MB.
//
// Until the harness lands, the assertions are documented here so a
// future PR can lift them into runnable code without re-deriving the
// contract from spec.md (SC-010) or research R3.
func TestPDF_GraphicsAndTextFallback(t *testing.T) {
	t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")
}
