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

// ErrUnsupportedDecoder signals that a graphics / PDF decoder either
// failed at the cgo / panic boundary or refused to decode an
// attacker-controlled blob. Callers fall back to the metadata block
// (image) or text extraction (PDF) instead of tearing down the
// alt-screen. Wrapped via fmt.Errorf with %w; tests rely on
// errors.Is(err, ErrUnsupportedDecoder) to detect the recovery path.
var ErrUnsupportedDecoder = errors.New("render: image / PDF decoder rejected the input")

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
//
// Concurrency invariant (acceptance review M3): all methods on
// *pdfRenderer MUST be called from the Bubble Tea event-loop
// goroutine. The cache fields below are unprotected — concurrent
// access from a reader goroutine or background tea.Cmd will race.
// pdfRenderer instances are constructed on the event-loop goroutine
// via [render.ForKind] (called from `rebuildRenderer` /
// model construction in internal/ui) and consumed on the same
// goroutine via [pdfRenderer.Render] from internal/ui/update.go
// (every call site of `m.renderer.Render(...)`). No
// synchronization is needed today; if a future refactor moves
// renderer construction or invocation onto another goroutine, this
// invariant must be re-evaluated and a mutex (or per-renderer
// channel handoff) added before that lands.
type pdfRenderer struct {
	deps Dependencies
	src  source.Source

	// Cache fields below are protected by the event-loop-goroutine
	// invariant documented on the struct comment above. No mutex.

	// cachedFrame memoizes the rendered output keyed on (page, proto,
	// cols, rows). Stored regardless of whether the graphics path
	// succeeded or fell back to the text/metadata block — both
	// outputs are stable for a given key, so repeat renders (every
	// key press triggers one) skip both the rasterize-encode pipeline
	// AND the per-page text extraction (Copilot review PR#11
	// round-3). Encoding takes 100 ms+ for a typical page on slow
	// hardware; without a hard cache hit the user feels every keypress.
	cachedPage  int
	cachedProto term.Graphics
	cachedCols  int
	cachedRows  int
	cachedFrame string
	cacheValid  bool

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
		// nil-source placeholder is constant — no need to cache.
		return r.metadataBlock("no source attached", 0, 0)
	}
	page := ctx.Page
	if page <= 0 {
		page = 1
	}
	proto := ctx.Capabilities.Graphics
	cols := ctx.Capabilities.Cols
	rows := ctx.Capabilities.Rows

	// Cache hit: the frame for this (page, proto, cols, rows) was
	// already computed (success OR fallback). Repeat renders skip
	// both the rasterize / encode pipeline AND the per-page text
	// extraction.
	if r.cacheValid && r.cachedPage == page && r.cachedProto == proto &&
		r.cachedCols == cols && r.cachedRows == rows {
		return r.cachedFrame
	}

	out := r.renderFresh(page, proto, cols, rows)
	r.cachedFrame = out
	r.cachedPage = page
	r.cachedProto = proto
	r.cachedCols = cols
	r.cachedRows = rows
	r.cacheValid = true
	return out
}

// renderFresh produces the frame for the given key without consulting
// the cache. The graphics path is tried first when the terminal
// supports it; otherwise (or on rasterize/encode failure / fitz-disabled
// build) the text-extraction fallback runs unconditionally.
func (r *pdfRenderer) renderFresh(page int, proto term.Graphics, cols, rows int) string {
	if proto != term.GraphicsNone {
		img, err := rasterizePDFPage(r.src, page-1)
		switch {
		case err == nil:
			out, encErr := graphics.Render(proto, img, cols, rows)
			if encErr == nil && out != "" {
				return out
			}
			// fall through to text fallback on encode error
		case errors.Is(err, ErrPDFGraphicsUnavailable):
			// fitz disabled in this build — fall through silently to
			// the text path. The fallback message in metadataBlock
			// surfaces the reason if the text path also fails.
		default:
			// fall through silently to the text fallback. If the text
			// path also fails, that later error is surfaced via
			// metadataBlock; the rasterize-side error is intentionally
			// swallowed because the user already gets useful content
			// (Copilot review PR#11 round-2 #2).
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
	// `ledongthuc/pdf` requires an io.ReaderAt + size. We materialize
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
//
// Both the PDF text-extraction output (`text`) and the source
// display name are funneled through [Neutralize] — `ledongthuc/pdf`
// returns the raw PDF content stream bytes, which a hostile document
// can use to embed OSC / DCS escapes. Acceptance review C4.
func (r *pdfRenderer) formatTextPage(text string, page, total int) string {
	var b strings.Builder
	name := Neutralize(r.src.DisplayName())
	if total > 0 {
		fmt.Fprintf(&b, "[pdf: %s — page %d/%d]\n", name, page, total)
	} else {
		fmt.Fprintf(&b, "[pdf: %s — page %d]\n", name, page)
	}
	b.WriteString(Neutralize(strings.TrimRight(text, " \n\r\t")))
	b.WriteString("\n")
	return b.String()
}

// metadataBlock formats the deterministic fallback message.
//
// DisplayName, the optional `note`, and (defensively) the size string
// pass through [Neutralize] so a hostile filename containing OSC
// payload bytes cannot reach the terminal. Acceptance review C4.
func (r *pdfRenderer) metadataBlock(note string, page, total int) string {
	if r.src == nil {
		return "[pdf: no source attached]\n"
	}
	md := r.src.Metadata()
	var b strings.Builder
	fmt.Fprintf(&b, "[pdf: %s]\n", Neutralize(r.src.DisplayName()))
	if total > 0 {
		fmt.Fprintf(&b, "  page: %d/%d\n", page, total)
	} else if page > 0 {
		fmt.Fprintf(&b, "  page: %d\n", page)
	}
	if md.Size > 0 {
		fmt.Fprintf(&b, "  size: %s\n", humanSize(md.Size))
	}
	if note != "" {
		fmt.Fprintf(&b, "  note: %s\n", Neutralize(note))
	}
	return b.String()
}
