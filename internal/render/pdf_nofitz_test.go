// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !fitz

package render

import (
	"strings"
	"testing"

	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// TestPDFRenderer_NoFitzFallsBackToText covers the documented `!fitz`
// build behavior: even when the user is on a graphics-capable terminal
// the renderer must silently fall back to text extraction because
// rasterizePDFPage returns [ErrPDFGraphicsUnavailable]. This test is
// build-tag-gated so the equivalent fitz path can succeed without
// triggering a contradictory expectation.
func TestPDFRenderer_NoFitzFallsBackToText(t *testing.T) {
	body := loadFixturePDF(t, "dummy.pdf")
	src := &fakePDFSource{name: "dummy.pdf", bytes: body, pageCount: 1}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindPDF, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty},
		Page:         1,
	})
	if !strings.Contains(out, "Dummy PDF file") {
		t.Errorf("default (nofitz) build should fall back to text, got %q", out)
	}
}
