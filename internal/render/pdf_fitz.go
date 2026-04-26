// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build fitz

package render

import (
	"fmt"
	"image"
	"image/draw"
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
//
// `fitz.NewFromMemory` and `doc.Image` are the cgo trust boundary for
// attacker-controlled PDF bytes. MuPDF panics on certain malformed
// inputs (the underlying C library calls `longjmp` which Go surfaces
// as a runtime panic). The deferred recover surfaces the failure as
// [ErrUnsupportedDecoder] so the calling renderer falls back to text
// extraction instead of tearing down the alt-screen.
func rasterizePDFPage(src source.Source, page int) (img image.Image, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			img = nil
			err = fmt.Errorf("%w: PDF rasterizer panicked: %v", ErrUnsupportedDecoder, rec)
		}
	}()
	rs, oerr := src.Reopen()
	if oerr != nil {
		return nil, fmt.Errorf("reopen: %w", oerr)
	}
	if c, ok := rs.(io.Closer); ok {
		defer c.Close()
	}
	body, rerr := io.ReadAll(rs)
	if rerr != nil {
		return nil, fmt.Errorf("read: %w", rerr)
	}
	doc, derr := fitz.NewFromMemory(body)
	if derr != nil {
		return nil, fmt.Errorf("fitz.NewFromMemory: %w", derr)
	}
	defer doc.Close()
	if page < 0 || page >= doc.NumPage() {
		return nil, fmt.Errorf("page %d out of range (0-%d)", page, doc.NumPage()-1)
	}
	pim, ierr := doc.Image(page)
	if ierr != nil {
		return nil, fmt.Errorf("rasterize page %d: %w", page, ierr)
	}
	// Force a copy so `doc.Close()` can release MuPDF's internal
	// buffers without invalidating the returned image. image/draw's
	// bulk path uses image.RGBA fast-paths when both src and dst are
	// *image.RGBA (which go-fitz returns), so this is a single memcpy
	// per scanline rather than O(w*h) method calls (Copilot review
	// PR#11 #3).
	out := image.NewRGBA(pim.Bounds())
	draw.Draw(out, out.Bounds(), pim, pim.Bounds().Min, draw.Src)
	return out, nil
}
