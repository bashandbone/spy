// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build fitz

package render

import (
	"strings"
	"testing"

	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// TestPDFRenderer_FitzRasterizesToGraphics covers the `fitz`-build
// behavior: on a graphics-capable terminal the renderer rasterizes the
// page and emits the protocol payload (Kitty escape stream) rather
// than the text fallback.
func TestPDFRenderer_FitzRasterizesToGraphics(t *testing.T) {
	body := loadFixturePDF(t, "dummy.pdf")
	src := &fakePDFSource{name: "dummy.pdf", bytes: body, pageCount: 1}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindPDF, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty},
		Page:         1,
	})
	if !strings.HasPrefix(out, "\x1b_G") {
		t.Errorf("fitz + Kitty path should emit graphics protocol bytes, got %q",
			out[:minLen(out, 40)])
	}
	if !strings.HasSuffix(out, "\x1b\\") {
		t.Errorf("fitz + Kitty path missing terminator")
	}
}

func minLen(s string, n int) int {
	if len(s) < n {
		return len(s)
	}
	return n
}
