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
	for _, l := range lines {
		if t.deps.LineNumbers {
			fmt.Fprintf(&b, "%*d  ", gutter, l.Number)
		}
		b.WriteString(l.Raw)
		b.WriteByte('\n')
	}
	return b.String()
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
