// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// textRenderer is the foundational pass-through renderer for [Kind]Text
// content. It produces line-numbered output without syntax highlighting
// — the latter lands in US1 via the [KindCode] renderer.
type textRenderer struct {
	deps Dependencies
}

// Render walks the resident hot region of the buffer and emits one
// formatted line per source line. The viewport's scrolling concerns
// are handled outside the renderer; we always emit the full resident
// range and let bubbles/viewport clip to the visible window.
//
// Empty input (per contracts/cli.md "Empty input") renders a single
// "(empty)" placeholder so the viewer has something to display. The
// 0-line footer is produced by [internal/ui/view.go]'s footerLine.
//
// Word-wrap honours [Dependencies.WordWrap]: when true (the contract
// default), lines longer than the active viewport width are wrapped
// across multiple visual rows with the gutter blanked on continuation
// rows. When false (--no-wrap), long lines emit verbatim so the
// viewport can horizontally scroll them.
//
// Phase 2 limitation: when the buffer has flipped into windowed mode,
// Slice(0, total) returns only the resident overlap — lines evicted
// from memory are not shown. A future polish task plumbs visible-range
// re-seek into the renderer (Copilot review PR#7 #14, deferred).
func (t *textRenderer) Render(ctx RenderContext) string {
	if ctx.Buffer == nil {
		return "(empty)\n"
	}
	total := ctx.Buffer.Total()
	if total <= 0 {
		return "(empty)\n"
	}
	lines := ctx.Buffer.Slice(0, total)
	if len(lines) == 0 {
		return "(empty)\n"
	}

	var b strings.Builder
	gutter := lineNumberWidth(total)
	// Prefer the viewport's live width (it reflects post-resize state)
	// and fall back to the capabilities-side terminal column count when
	// the viewport hasn't been initialized yet (first paint before
	// WindowSizeMsg lands).
	width := ctx.Viewport.Width
	if width <= 0 {
		width = ctx.Capabilities.Cols
	}

	active, hasActive := activeMatch(ctx.Search)
	for _, l := range lines {
		prefix := ""
		if t.deps.LineNumbers {
			prefix = fmt.Sprintf("%*d  ", gutter, l.Number)
		}
		lineMatches := matchesForLine(ctx.Search, l.Number)
		// --no-wrap, no width signal, or wrap disabled: emit verbatim.
		// The viewport widget handles horizontal scrolling.
		if !t.deps.WordWrap || width <= 0 {
			b.WriteString(prefix)
			if len(lineMatches) > 0 {
				b.WriteString(applyMatchHighlights(l.Raw, lineMatches, active, hasActive, ctx.Theme.SearchHit, ctx.Theme.SearchActive))
			} else {
				b.WriteString(l.Raw)
			}
			b.WriteByte('\n')
			continue
		}
		// Wrap on: the wrap helper still owns rune-bound layout; if the
		// line has matches we splice them in over the raw text *before*
		// wrapping so each visual row carries its own highlight ANSI.
		if len(lineMatches) > 0 {
			styled := applyMatchHighlights(l.Raw, lineMatches, active, hasActive, ctx.Theme.SearchHit, ctx.Theme.SearchActive)
			// writeWrappedLine wraps by rune count, which counts the
			// embedded ANSI bytes — for wrapped + match-highlighted
			// lines we therefore emit unwrapped to keep the highlight
			// intact (documented Phase 4 limitation).
			b.WriteString(prefix)
			b.WriteString(styled)
			b.WriteByte('\n')
			continue
		}
		writeWrappedLine(&b, prefix, l.Raw, width)
	}
	return b.String()
}

// writeWrappedLine emits `raw` with `prefix` on the first row and a
// blank gutter of the same width on continuation rows, breaking at
// `total - len(prefix)` runes per row. Empty `raw` still emits a single
// row (with prefix only) so blank source lines don't collapse.
//
// Phase 2 limitation: width here is rune-count, not terminal cell
// width — wide-character runs (CJK, emoji) over-fill rows by one
// cell per wide rune, and any embedded ANSI escapes count toward
// the rune budget despite consuming zero cells. KindText itself has
// no ANSI in Phase 2 (no syntax highlighting); US1's KindCode renderer
// (T043) replaces this with a chroma/lipgloss-aware wrapper that
// honours [lipgloss.Width] (Copilot review PR#7 #26 deferred).
func writeWrappedLine(b *strings.Builder, prefix, raw string, total int) {
	contentWidth := total - len(prefix)
	if contentWidth <= 0 {
		// Prefix already fills the row — emit prefix-only and bail so we
		// don't produce a zero-width content row that loops forever.
		b.WriteString(prefix)
		b.WriteString(raw)
		b.WriteByte('\n')
		return
	}
	if raw == "" {
		b.WriteString(prefix)
		b.WriteByte('\n')
		return
	}
	continuation := strings.Repeat(" ", len(prefix))
	runes := []rune(raw)
	for start := 0; start < len(runes); start += contentWidth {
		end := start + contentWidth
		if end > len(runes) {
			end = len(runes)
		}
		if start == 0 {
			b.WriteString(prefix)
		} else {
			b.WriteString(continuation)
		}
		b.WriteString(string(runes[start:end]))
		b.WriteByte('\n')
	}
}

// RowToLine walks the rendered output's visual rows and returns the
// source-line number whose content occupies row `visualRow`. With
// wrap on, one source line can span multiple visual rows; this method
// keeps the footer's "Line N" consistent with the gutter the renderer
// printed (Copilot review PR#7 #24).
//
// Width and prefix are computed identically to [textRenderer.Render]
// so the row → line mapping matches the actual frame.
//
// Phase 2 limitation: width here is rune-width, not terminal cell
// width — wide-character / ANSI runs may give an off-by-one mapping.
// US1's chroma-aware renderer replaces this with a cell-aware version
// (Copilot review PR#7 #26 deferred).
func (t *textRenderer) RowToLine(ctx RenderContext, visualRow int) int64 {
	if ctx.Buffer == nil {
		return 0
	}
	total := ctx.Buffer.Total()
	if total <= 0 {
		return 0
	}
	lines := ctx.Buffer.Slice(0, total)
	if len(lines) == 0 {
		return 0
	}
	if visualRow < 0 {
		return lines[0].Number
	}

	// No-wrap fast path: each source line is one visual row.
	width := ctx.Viewport.Width
	if width <= 0 {
		width = ctx.Capabilities.Cols
	}
	if !t.deps.WordWrap || width <= 0 {
		if visualRow >= len(lines) {
			return lines[len(lines)-1].Number
		}
		return lines[visualRow].Number
	}

	prefixWidth := 0
	if t.deps.LineNumbers {
		prefixWidth = lineNumberWidth(total) + 2 // "%*d  "
	}
	contentWidth := width - prefixWidth
	if contentWidth <= 0 {
		// Pathologically narrow viewport — fall back to one row per line
		// to avoid an infinite mapping loop.
		if visualRow >= len(lines) {
			return lines[len(lines)-1].Number
		}
		return lines[visualRow].Number
	}

	consumed := 0
	for _, l := range lines {
		rows := visualRowsForLine(l.Raw, contentWidth)
		if visualRow < consumed+rows {
			return l.Number
		}
		consumed += rows
	}
	return lines[len(lines)-1].Number
}

// visualRowsForLine returns the number of wrapped rows `raw` will
// occupy at content-width `width`. Empty lines still occupy one row
// so blanks don't collapse — matches [writeWrappedLine].
func visualRowsForLine(raw string, width int) int {
	if width <= 0 {
		return 1
	}
	if raw == "" {
		return 1
	}
	n := utf8.RuneCountInString(raw)
	rows := n / width
	if n%width != 0 {
		rows++
	}
	if rows == 0 {
		return 1
	}
	return rows
}

// lineNumberWidth returns the digit-width needed to display every line
// number up to `total`. A buffer with 1234 lines uses 4-wide gutters.
func lineNumberWidth(total int64) int {
	if total < 10 {
		return 1
	}
	w := 0
	for total > 0 {
		total /= 10
		w++
	}
	return w
}
