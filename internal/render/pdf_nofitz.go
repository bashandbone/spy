// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !fitz

package render

import (
	"image"

	"github.com/knitli/spy/internal/source"
)

// rasterizePDFPage returns the documented sentinel error in non-fitz
// builds (the cgo MuPDF binding is excluded so static / no-cgo binaries
// can ship). The caller falls back to the text-extraction path.
//
// The build-tag-gated companion in `pdf_fitz.go` provides the real
// implementation when `-tags fitz` is set; both files keep the same
// signature so [pdfRenderer.Render] doesn't need to branch on a build
// tag of its own.
func rasterizePDFPage(_ source.Source, _ int) (image.Image, error) {
	return nil, ErrPDFGraphicsUnavailable
}
