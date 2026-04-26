// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build fitz

package render

import (
	"bytes"
	"fmt"
	"image"
	"io"

	"github.com/gen2brain/go-fitz"

	"github.com/knitli/spy/internal/source"
)

// rasterizePDFPage rasterizes the 0-based `page` of the source PDF to
// an [image.Image] using gen2brain/go-fitz (Go bindings for MuPDF).
// Available only when the binary is compiled with `-tags fitz`; the
// stub in `pdf_nofitz.go` returns [ErrPDFGraphicsUnavailable] for the
// default build so static / no-cgo distributors aren't forced to pull
// in MuPDF.
func rasterizePDFPage(src source.Source, page int) (image.Image, error) {
	rs, err := src.Reopen()
	if err != nil {
		return nil, fmt.Errorf("reopen: %w", err)
	}
	if c, ok := rs.(io.Closer); ok {
		defer c.Close()
	}
	body, err := io.ReadAll(rs)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	doc, err := fitz.NewFromMemory(body)
	if err != nil {
		return nil, fmt.Errorf("fitz.NewFromMemory: %w", err)
	}
	defer doc.Close()
	if page < 0 || page >= doc.NumPage() {
		return nil, fmt.Errorf("page %d out of range (0-%d)", page, doc.NumPage()-1)
	}
	img, err := doc.Image(page)
	if err != nil {
		return nil, fmt.Errorf("rasterize page %d: %w", page, err)
	}
	// Force a copy so `doc.Close()` can release MuPDF's internal
	// buffers without invalidating the returned image.
	out := image.NewRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	_ = bytes.NewReader // keep io / bytes imports stable across edits
	return out, nil
}
