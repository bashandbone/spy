// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"fmt"
	"strings"
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

	for _, l := range lines {
		prefix := ""
		if t.deps.LineNumbers {
			prefix = fmt.Sprintf("%*d  ", gutter, l.Number)
		}
		// --no-wrap, no width signal, or wrap disabled: emit verbatim.
		// The viewport widget handles horizontal scrolling.
		if !t.deps.WordWrap || width <= 0 {
			b.WriteString(prefix)
			b.WriteString(l.Raw)
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
