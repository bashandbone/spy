// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	pdfreader "github.com/ledongthuc/pdf"

	"github.com/knitli/spy/internal/graphics"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// ErrPDFGraphicsUnavailable signals that PDF rasterization is not
// available in the current build (T079: the `nofitz` build excludes
// the cgo MuPDF binding so static / no-cgo binaries still ship). The
// renderer returns the page-text fallback in that case.
var ErrPDFGraphicsUnavailable = errors.New("pdf: rasterization disabled in this build")

// pdfRenderer draws PDF sources. By default — and on every build that
// excludes the `fitz` tag — the renderer falls back to text extraction
// via `ledongthuc/pdf`, displaying the active page's plain text. With
// the `fitz` build tag set AND a graphics-capable terminal, the active
// page is rasterized to PNG via `gen2brain/go-fitz` and routed through
// [graphics.Render] so the user sees an actual rendered page.
//
// Per research R3 the renderer falls back automatically when graphics
// aren't available — capability *or* build configuration. The two
// failure modes are mapped to a single deterministic message so the
// integration tests can assert against it.
type pdfRenderer struct {
	deps Dependencies
	src  source.Source

	// cachedPage / cachedFrame memoize the rasterized page across
	// re-renders. Same logic as [imageRenderer]: encoding takes 100ms+
	// for a typical page on slow hardware, so caching the frame keyed
	// on `(page, proto, cols, rows)` keeps key-press latency low.
	cachedPage   int
	cachedProto  term.Graphics
	cachedCols   int
	cachedRows   int
	cachedFrame  string
	cachedFailed bool

	// cachedText memoizes the per-page plain-text extraction so
	// `]`/`[` page navigation doesn't re-parse the entire file each
	// time. Indexed by 1-based page number.
	cachedText map[int]string

	// cachedTotal stores the parsed page count. Set on the first
	// successful pageText() call so navigation footers
	// (`page 2/3`) and out-of-range checks don't trigger a second
	// re-parse of the whole document. Zero means "not yet known"
	// (Copilot review PR#11 #4).
	cachedTotal int
}

// newPDFRenderer wires the per-source state.
func newPDFRenderer(deps Dependencies, src source.Source) *pdfRenderer {
	return &pdfRenderer{
		deps:       deps,
		src:        src,
		cachedText: make(map[int]string),
	}
}

func (r *pdfRenderer) Render(ctx RenderContext) string {
	if r.src == nil {
		return r.metadataBlock("no source attached", 0, 0)
	}
	page := ctx.Page
	if page <= 0 {
		page = 1
	}
	proto := ctx.Capabilities.Graphics
	cols := ctx.Capabilities.Cols
	rows := ctx.Capabilities.Rows

	// Graphics-capable + fitz-enabled build: rasterize and emit.
	if proto != term.GraphicsNone {
		if !r.cachedFailed && r.cachedFrame != "" &&
			r.cachedPage == page && r.cachedProto == proto &&
			r.cachedCols == cols && r.cachedRows == rows {
			return r.cachedFrame
		}
		img, err := rasterizePDFPage(r.src, page-1)
		switch {
		case err == nil:
			out, encErr := graphics.Render(proto, img, cols, rows)
			if encErr == nil && out != "" {
				r.cachedFrame = out
				r.cachedPage = page
				r.cachedProto = proto
				r.cachedCols = cols
				r.cachedRows = rows
				r.cachedFailed = false
				return out
			}
			// fall through to text fallback on encode error
		case errors.Is(err, ErrPDFGraphicsUnavailable):
			// fitz disabled in this build — fall through silently to
			// the text path. The fallback message in metadataBlock
			// surfaces the reason if the text path also fails.
		default:
			r.cachedFailed = true
			// fall through to text fallback with the error noted in
			// the footer below.
		}
	}

	// Text fallback. Always available; always free.
	text, total, err := r.pageText(page)
	if err != nil {
		return r.metadataBlock(fmt.Sprintf("text extraction failed: %v", err), page, 0)
	}
	if strings.TrimSpace(text) == "" {
		return r.metadataBlock("page contains no extractable text", page, total)
	}
	return r.formatTextPage(text, page, total)
}

func (r *pdfRenderer) RowToLine(_ RenderContext, _ int) int64 { return 0 }

// pageText extracts the human-readable plain text for `page` (1-based)
// using `ledongthuc/pdf`. The result is cached so subsequent renders
// of the same page are O(1); the total page count is cached on the
// first parse so footer headers (`page 2/3`) don't trigger a second
// re-parse of the whole document (Copilot review PR#11 #4).
func (r *pdfRenderer) pageText(page int) (string, int, error) {
	if cached, ok := r.cachedText[page]; ok {
		return cached, r.cachedTotal, nil
	}
	rs, err := r.src.Reopen()
	if err != nil {
		return "", r.cachedTotal, fmt.Errorf("reopen: %w", err)
	}
	if c, ok := rs.(io.Closer); ok {
		defer c.Close()
	}
	// `ledongthuc/pdf` requires an io.ReaderAt + size. We materialise
	// the seekable reader into a bytes.Reader so we can hand it both.
	body, err := io.ReadAll(rs)
	if err != nil {
		return "", r.cachedTotal, fmt.Errorf("read: %w", err)
	}
	reader, err := pdfreader.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", r.cachedTotal, fmt.Errorf("parse: %w", err)
	}
	total := reader.NumPage()
	r.cachedTotal = total
	if page < 1 || page > total {
		return "", total, fmt.Errorf("page %d out of range (1-%d)", page, total)
	}
	p := reader.Page(page)
	if p.V.IsNull() {
		return "", total, fmt.Errorf("page %d has no content", page)
	}
	text, err := p.GetPlainText(nil)
	if err != nil {
		return "", total, fmt.Errorf("extract: %w", err)
	}
	r.cachedText[page] = text
	return text, total, nil
}

// formatTextPage wraps the extracted text in a header so the user can
// see the page indicator + total without checking the status bar.
func (r *pdfRenderer) formatTextPage(text string, page, total int) string {
	var b strings.Builder
	if total > 0 {
		fmt.Fprintf(&b, "[pdf: %s — page %d/%d]\n", r.src.DisplayName(), page, total)
	} else {
		fmt.Fprintf(&b, "[pdf: %s — page %d]\n", r.src.DisplayName(), page)
	}
	b.WriteString(strings.TrimRight(text, " \n\r\t"))
	b.WriteString("\n")
	return b.String()
}

// metadataBlock formats the deterministic fallback message.
func (r *pdfRenderer) metadataBlock(note string, page, total int) string {
	if r.src == nil {
		return "[pdf: no source attached]\n"
	}
	md := r.src.Metadata()
	var b strings.Builder
	fmt.Fprintf(&b, "[pdf: %s]\n", r.src.DisplayName())
	if total > 0 {
		fmt.Fprintf(&b, "  page: %d/%d\n", page, total)
	} else if page > 0 {
		fmt.Fprintf(&b, "  page: %d\n", page)
	}
	if md.Size > 0 {
		fmt.Fprintf(&b, "  size: %s\n", humanSize(md.Size))
	}
	if note != "" {
		fmt.Fprintf(&b, "  note: %s\n", note)
	}
	return b.String()
}
