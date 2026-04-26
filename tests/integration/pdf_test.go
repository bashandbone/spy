// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestPDF_TextFallback covers the non-graphics half of SC-010 (T072b):
// in a non-graphics terminal (the default for a vanilla PTY), a PDF
// opens with the ledongthuc/pdf text-extraction renderer and the
// rendered frame contains the sentinel string from the document.
//
// The graphics-capable + `-tags fitz` half (Kitty payload + page
// navigation) requires both a fitz-built binary and a Kitty-claiming
// terminal — kept as TestPDF_KittyRasterPath_Skipped below until those
// preconditions are wired into the harness.
func TestPDF_TextFallback(t *testing.T) {
	root := moduleRoot(t)
	fixture := filepath.Join(root, "tests", "fixtures", "pdf", "dummy.pdf")

	p := NewPTYProgram(t, []string{"--no-config", "--graphics", "none", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(400 * time.Millisecond)

	// SC-010 sentinel. The W3C dummy.pdf contains this exact string.
	frame := string(p.Snapshot())
	if !strings.Contains(frame, "Dummy PDF file") {
		t.Fatalf("PDF text-extraction frame missing sentinel 'Dummy PDF file'; snapshot tail=%q", truncTail([]byte(frame), 600))
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

// TestPDF_MultiPage_PageNavigation exercises `]` page-advance against
// the 3-page fixture. Even in the non-graphics text-fallback path,
// `]` must rotate to page 2 and the rendered text from page 2 should
// appear (here: the sentinel still applies because multi-page.pdf is
// the dummy merged 3 times, but page identity is verifiable via the
// "Page" indicator in the footer).
func TestPDF_MultiPage_PageNavigation(t *testing.T) {
	root := moduleRoot(t)
	fixture := filepath.Join(root, "tests", "fixtures", "pdf", "multi-page.pdf")

	p := NewPTYProgram(t, []string{"--no-config", "--graphics", "none", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(400 * time.Millisecond)

	first := string(p.Snapshot())
	if !strings.Contains(first, "Dummy PDF file") {
		t.Fatalf("first paint missing PDF sentinel; snapshot tail=%q", truncTail([]byte(first), 400))
	}

	// `]` advances Page from 1 to 2. We assert on the structured
	// `Page N` footer marker (from internal/render/statusbar.go's
	// PDF format) rather than bare digits — page numbers appear
	// inside gutters / content / file-size strings, which made the
	// prior "contains 2 and 3" check trivially true regardless of
	// whether the page actually advanced.
	//
	// The total-page suffix (`/3`) is not pinned because pdfcpu's
	// page-count discovery on multi-page.pdf isn't always populated
	// by the time the first paint ships in the no-fitz path
	// (statusbar branches to bare `Page N` when Meta.PageCount == 0).
	// Asserting on the prefix alone keeps the test honest about what
	// the v0.1.0 footer guarantees.
	//
	// The `]` keystroke is sent through a small retry loop because
	// the first keystroke after first paint is occasionally dropped
	// (same quirk documented in pty_sanity_test.go's quit loop).
	pageAdvanced := false
	for i := 0; i < 5; i++ {
		p.Send("]")
		time.Sleep(150 * time.Millisecond)
		frame := stripANSI(string(p.Snapshot()))
		if regexp.MustCompile(`Page\s*2\b`).MatchString(frame) {
			pageAdvanced = true
			break
		}
	}
	if !pageAdvanced {
		t.Fatalf("footer did not advance to 'Page 2' after 5 retries; stripped tail=%q",
			truncTail([]byte(stripANSI(string(p.Snapshot()))), 400))
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

// TestPDF_KittyRasterPath documents the still-deferred half of
// SC-010: the Kitty-graphics + `-tags fitz` path. Lifting requires
// the harness to spawn a fitz-built binary and to advertise a
// Kitty-claiming terminal type.
func TestPDF_KittyRasterPath(t *testing.T) {
	t.Skip("SC-010 fitz path: requires `-tags fitz` build + Kitty-capable PTY (harness extension still pending — see acceptance_review/00-summary.md C5/C6)")
}
