// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// fakePDFSource yields an in-memory PDF without touching the
// filesystem. Mirrors fakeImageSource from image_test.go.
type fakePDFSource struct {
	name      string
	bytes     []byte
	pageCount int
}

func (f *fakePDFSource) Kind() source.Kind   { return source.KindPDF }
func (f *fakePDFSource) DisplayName() string { return f.name }
func (f *fakePDFSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.bytes)), nil
}
func (f *fakePDFSource) Reopen() (io.ReadSeeker, error) { return bytes.NewReader(f.bytes), nil }
func (f *fakePDFSource) Metadata() source.Metadata {
	return source.Metadata{
		Path:      f.name,
		Size:      int64(len(f.bytes)),
		PageCount: f.pageCount,
		Modified:  time.Time{},
	}
}

// loadFixturePDF reads the W3C dummy PDF (or the merged 3-page variant)
// shipped under tests/e2e/fixtures/. The file is tracked in git so the
// unit test runs without a network dep — it's the same fixture the
// integration suite (T072b) drives via the PTY harness.
func loadFixturePDF(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "tests", "e2e", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func TestPDFRenderer_TextExtractionShowsSentinel(t *testing.T) {
	body := loadFixturePDF(t, "dummy.pdf")
	src := &fakePDFSource{name: "dummy.pdf", bytes: body, pageCount: 1}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindPDF, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsNone},
		Page:         1,
	})
	if !strings.Contains(out, "[pdf: dummy.pdf") {
		t.Errorf("PDF header missing in output: %q", out)
	}
	// The W3C dummy PDF carries the literal `Dummy PDF file` text
	// rendered via Tj — assert it survived extraction. SC-010 names
	// this exact path.
	if !strings.Contains(out, "Dummy PDF file") {
		t.Errorf("PDF text fallback should contain 'Dummy PDF file', got %q", out)
	}
}

func TestPDFRenderer_ErrPDFGraphicsUnavailableSentinel(t *testing.T) {
	if !errors.Is(ErrPDFGraphicsUnavailable, ErrPDFGraphicsUnavailable) {
		t.Fatal("ErrPDFGraphicsUnavailable should equal itself via errors.Is")
	}
}

func TestPDFRenderer_NilSourceProducesPlaceholder(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark()}
	r := ForKind(source.KindPDF, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty},
	})
	if !strings.Contains(out, "no source attached") {
		t.Errorf("expected nil-source placeholder, got %q", out)
	}
}

func TestPDFRenderer_BadPDFFallsBack(t *testing.T) {
	src := &fakePDFSource{name: "bogus.pdf", bytes: []byte("not a pdf"), pageCount: 0}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindPDF, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsNone},
		Page:         1,
	})
	if !strings.Contains(out, "[pdf: bogus.pdf]") {
		t.Errorf("expected metadata header, got %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "fail") &&
		!strings.Contains(strings.ToLower(out), "no extractable") {
		t.Errorf("expected failure note in fallback, got %q", out)
	}
}

func TestPDFRenderer_OpenFailureProducesPlaceholder(t *testing.T) {
	src := &errPDFSource{name: "lockedout.pdf"}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindPDF, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsNone},
		Page:         1,
	})
	if !strings.Contains(out, "[pdf: lockedout.pdf]") {
		t.Errorf("expected metadata header, got %q", out)
	}
}

func TestPDFRenderer_PageZeroDefaultsToOne(t *testing.T) {
	body := loadFixturePDF(t, "dummy.pdf")
	src := &fakePDFSource{name: "dummy.pdf", bytes: body, pageCount: 1}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindPDF, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsNone},
		Page:         0,
	})
	if !strings.Contains(out, "page 1/1") {
		t.Errorf("expected 'page 1/1' in header, got %q", out)
	}
}

func TestPDFRenderer_OutOfRangePageProducesNote(t *testing.T) {
	body := loadFixturePDF(t, "dummy.pdf")
	src := &fakePDFSource{name: "dummy.pdf", bytes: body, pageCount: 1}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindPDF, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsNone},
		Page:         42,
	})
	if !strings.Contains(out, "[pdf: dummy.pdf]") {
		t.Errorf("expected metadata header, got %q", out)
	}
	if !strings.Contains(out, "out of range") {
		t.Errorf("expected out-of-range note, got %q", out)
	}
}

func TestPDFRenderer_MultiPageNavigation(t *testing.T) {
	// Page 2 of multi-page.pdf should also contain the sentinel
	// (multi-page.pdf was built by merging dummy.pdf 3×).
	body := loadFixturePDF(t, "multi-page.pdf")
	src := &fakePDFSource{name: "multi-page.pdf", bytes: body, pageCount: 3}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindPDF, deps)
	for _, page := range []int{1, 2, 3} {
		out := r.Render(RenderContext{
			Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsNone},
			Page:         page,
		})
		if !strings.Contains(out, "Dummy PDF file") {
			t.Errorf("page %d: expected sentinel, got %q", page, out)
		}
	}
}

func TestPDFRenderer_RowToLineAlwaysZero(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark()}
	r := ForKind(source.KindPDF, deps)
	if r.RowToLine(RenderContext{}, 0) != 0 {
		t.Errorf("RowToLine should return 0 for PDF kind")
	}
	if r.RowToLine(RenderContext{}, 9999) != 0 {
		t.Errorf("RowToLine should return 0 for any row")
	}
}

// errPDFSource always errors on Open / Reopen so we can exercise the
// PDF fallback paths.
type errPDFSource struct{ name string }

func (e *errPDFSource) Kind() source.Kind              { return source.KindPDF }
func (e *errPDFSource) DisplayName() string            { return e.name }
func (e *errPDFSource) Open() (io.ReadCloser, error)   { return nil, os.ErrNotExist }
func (e *errPDFSource) Reopen() (io.ReadSeeker, error) { return nil, os.ErrNotExist }
func (e *errPDFSource) Metadata() source.Metadata {
	return source.Metadata{Path: filepath.Join(".", e.name)}
}
