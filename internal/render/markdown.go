// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"strings"

	"github.com/charmbracelet/glamour"

	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// markdownRenderer is the [KindMarkdown] renderer: passes the buffer's
// raw lines through Glamour for prose-aware styling. Falls back to a
// passthrough textRenderer when Glamour can't be initialised, the
// theme is mono, or the active color depth is mono.
//
// Glamour does its own word-wrap when given a non-zero width, so the
// codeRenderer's rune-bound wrap helper isn't reused here.
type markdownRenderer struct {
	deps     Dependencies
	fallback Renderer
}

func newMarkdownRenderer(deps Dependencies) *markdownRenderer {
	return &markdownRenderer{
		deps:     deps,
		fallback: &textRenderer{deps: deps},
	}
}

// Render builds a Glamour-rendered string for the resident hot region.
// Glamour's renderer is constructed per-frame so width changes from a
// resize land in the next paint without leaking state.
func (r *markdownRenderer) Render(ctx RenderContext) string {
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

	// Glamour bypass when colour is suppressed: either the user / config
	// flagged the theme Mono (NO_COLOR=1, --no-color, mono profile) or
	// the terminal advertises ColorMono (TERM=dumb, no SGR support).
	// Without this second check Glamour would still emit ANSI escapes
	// against a terminal that can't render them (Copilot review PR#8
	// #4).
	if r.deps.Theme.Mono || r.deps.Capabilities.ColorDepth == term.ColorMono {
		return r.fallback.Render(ctx)
	}

	width := ctx.Viewport.Width
	if width <= 0 {
		width = ctx.Capabilities.Cols
	}
	if width <= 0 {
		width = 80
	}
	if r.deps.LineNumbers {
		// Reserve a few columns for the gutter so headings don't bleed
		// into the line-number column. Glamour handles the wrap.
		gutter := lineNumberWidth(total) + 2
		if width > gutter {
			width -= gutter
		}
	}

	style := glamourStyleForTheme(r.deps.Theme)
	gr, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return r.fallback.Render(ctx)
	}

	body := assembleRaw(lines)
	rendered, err := gr.Render(body)
	if err != nil {
		return r.fallback.Render(ctx)
	}

	if !r.deps.LineNumbers {
		return rendered
	}
	return prependGutter(rendered, lines, total)
}

// RowToLine delegates to the textRenderer's wrap math. Glamour's row
// inflation isn't perfectly mappable to source lines (it adds blank
// rows around headings, etc.); the footer's "Line N" therefore tracks
// the underlying source row, not the rendered visual row. This is the
// documented Phase 3 behaviour — a perfect mapping arrives in the US6
// status-bar polish (T098).
func (r *markdownRenderer) RowToLine(ctx RenderContext, visualRow int) int64 {
	return r.fallback.RowToLine(ctx, visualRow)
}

// glamourStyleForTheme picks a Glamour built-in style matching the
// active [Theme]. "auto" themes fall back to dark.
func glamourStyleForTheme(theme Theme) string {
	switch strings.ToLower(theme.Name) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	}
	// Named Chroma styles or "auto" → dark by default.
	return "dark"
}

// assembleRaw joins the raw line content with newlines so Glamour sees
// the document as one Markdown blob.
//
// Each line is funnelled through [Neutralize] before concatenation:
// Glamour's goldmark backend does not strip `\x1b` / `\x9b` bytes
// from non-code-block content, so a markdown file with an embedded
// OSC sequence (e.g. ` <!-- \x1b]2;evil\x07 --> `) would otherwise
// reach the terminal verbatim. Acceptance review C4.
func assembleRaw(lines []source.Line) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(Neutralize(l.Raw))
	}
	b.WriteByte('\n')
	return b.String()
}

// prependGutter walks the Glamour output's visual rows and prepends a
// blank gutter the same width as a line-number column would occupy,
// matching the codeRenderer's convention. We don't try to map Glamour
// rows back to source lines (that requires a full Glamour AST walk);
// the gutter exists only as visual padding so the markdown output
// aligns with the code/text panes.
func prependGutter(rendered string, _ []source.Line, total int64) string {
	gutter := strings.Repeat(" ", lineNumberWidth(total)+2)
	if !strings.Contains(rendered, "\n") {
		return gutter + rendered
	}
	parts := strings.Split(rendered, "\n")
	for i := range parts {
		parts[i] = gutter + parts[i]
	}
	return strings.Join(parts, "\n")
}
