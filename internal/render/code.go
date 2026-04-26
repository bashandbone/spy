// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// codeRenderer is the [KindCode] renderer: passes lines through the
// Chroma highlighter (or trusts pre-tokenised lines) and emits ANSI
// styling per the active [Theme.ChromaStyle]. Falls back to verbatim
// raw text when [Theme.Mono] is set or no [highlight.Highlighter] was
// supplied.
//
// Word-wrap respects [Dependencies.WordWrap]: when on, long lines wrap
// at viewport width using rune-count math (matching the foundational
// [textRenderer]). The styled (ANSI-tagged) line is emitted verbatim;
// when wrap is required, the renderer falls back to raw bytes for that
// line so colour escapes don't straddle wrap boundaries. The mixed
// behaviour is the documented Phase 3 limitation; an ANSI-aware wrap
// arrives in Phase 9 polish.
type codeRenderer struct {
	deps Dependencies
}

// Render walks the resident hot region of the buffer and emits one
// formatted line per source line, with optional gutter line numbers
// and rune-bound soft-wrap.
//
// PERF NOTE (SC-004 gap): this implementation formats every resident
// line on every paint rather than just the lines that fall inside the
// viewport's vertical window. At 10 000 lines that's outside the
// 16 ms p95 theme-swap budget — see tests/perf/theme_swap_bench_test.go.
// Refactoring to a viewport-windowed render is tracked as a Phase 9
// follow-up; the per-frame `viewport.SetContent(Render(ctx))` call site
// in internal/ui/update.go is unchanged once the windowed Render lands.
func (r *codeRenderer) Render(ctx RenderContext) string {
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
	width := ctx.Viewport.Width
	if width <= 0 {
		width = ctx.Capabilities.Cols
	}

	active, hasActive := activeMatch(ctx.Search)
	// In mono mode (`--no-color` / NO_COLOR=1 / TERM=dumb), the match
	// overlay must not emit ANSI — even via lipgloss styles applied
	// post-chroma — or the rendered output becomes a contract
	// violation (Copilot review PR#9 round-3 #1). When mono is active
	// we still mark matched lines but as raw text without colouring.
	mono := ctx.Theme.Mono || ctx.Capabilities.ColorDepth == term.ColorMono
	for _, l := range lines {
		prefix := ""
		if r.deps.LineNumbers {
			prefix = fmt.Sprintf("%*d  ", gutter, l.Number)
		}
		// Lines with search matches use the dedicated overlay path so
		// the highlight wraps tightly around the match span. The
		// documented limitation: matched lines lose chroma syntax
		// colour; the caret/active match is still visible.
		lineMatches := matchesForLine(ctx.Search, l.Number)
		if len(lineMatches) > 0 {
			b.WriteString(prefix)
			if mono {
				// Mono mode: bypass lipgloss styling — emit the raw
				// match line verbatim so no ANSI leaks. The line is
				// still escape-neutralised (T109b.c) so a file whose
				// bytes contain OSC / DCS sequences cannot drive the
				// user's terminal.
				b.WriteString(neutralizeEscapes(l.Raw))
			} else {
				b.WriteString(applyMatchHighlights(l.Raw, lineMatches, active, hasActive, ctx.Theme.SearchHit, ctx.Theme.SearchActive))
			}
			b.WriteByte('\n')
			continue
		}
		if !r.deps.WordWrap || width <= 0 || lineExceedsWidth(l.Raw, len(prefix), width) {
			if r.deps.WordWrap && width > 0 {
				// Long line + wrap on: fall back to raw text so wrap math
				// stays correct. The styled (ANSI) emission is reserved
				// for lines that fit on a single visual row.
				writeWrappedLine(&b, prefix, neutralizeEscapes(l.Raw), width)
				continue
			}
			styled := r.styleLine(l)
			b.WriteString(prefix)
			b.WriteString(styled)
			b.WriteByte('\n')
			continue
		}
		styled := r.styleLine(l)
		b.WriteString(prefix)
		b.WriteString(styled)
		b.WriteByte('\n')
	}
	return b.String()
}

// RowToLine reuses the textRenderer's wrap math so the footer's "Line N"
// stays consistent with the gutter the codeRenderer printed.
func (r *codeRenderer) RowToLine(ctx RenderContext, visualRow int) int64 {
	tr := &textRenderer{deps: r.deps}
	return tr.RowToLine(ctx, visualRow)
}

// styleLine returns the line's ANSI-styled rendition. Falls back to
// raw text when the theme is mono, the highlighter is missing, or
// formatting fails. All raw-text fallback paths run their content
// through [neutralizeEscapes] so a file whose bytes include OSC / DCS
// sequences cannot drive the user's terminal — see T109b.c
// (specs/001-popup-reader/checklists/security-review.md).
func (r *codeRenderer) styleLine(l source.Line) string {
	if r.deps.Theme.Mono {
		return neutralizeEscapes(l.Raw)
	}
	h := r.deps.Highlighter
	if h == nil {
		return neutralizeEscapes(l.Raw)
	}
	tokens := l.Tokens
	if tokens == nil {
		tokens = h.Highlight(r.deps.Language, l.Raw)
	}
	if len(tokens) == 0 {
		return neutralizeEscapes(l.Raw)
	}
	style := styles.Get(r.deps.Theme.ChromaStyle)
	if style == nil {
		style = h.Style()
	}
	if style == nil {
		style = styles.Fallback
	}
	fm := r.formatter()
	if fm == nil {
		return neutralizeEscapes(l.Raw)
	}
	// Common case: Chroma's Text tokens copy bytes verbatim from
	// l.Raw, so we can scan l.Raw once and skip the per-token copy
	// entirely when no ESC / CSI byte is present. Allocates only on
	// the rare line that actually carries an escape (T109b.c).
	safeTokens := tokens
	if needsTokenNeutralisation(l.Raw) {
		safeTokens = neutralizeTokens(tokens)
	}
	iter := chromaIterFromTokens(safeTokens)
	var buf bytes.Buffer
	if err := fm.Format(&buf, style, iter); err != nil {
		return neutralizeEscapes(l.Raw)
	}
	return strings.TrimRight(buf.String(), "\n")
}

// needsTokenNeutralisation reports whether `raw` contains any byte
// that [neutralizeEscapes] would substitute. Used as a fast pre-scan
// so the per-token copy in [neutralizeTokens] only runs on lines that
// actually carry an OSC / DCS / CSI byte. Per-line inputs are typically
// hundreds of bytes; the IndexAny scan is one cache-line read in the
// common case.
func needsTokenNeutralisation(raw string) bool {
	return strings.ContainsAny(raw, "\x1b\x9b")
}

// neutralizeTokens returns a copy of `tokens` with every ESC / CSI byte
// replaced in each token's Value (T109b.c). The Chroma formatter emits
// Text-token Values verbatim, so any 0x1b that survived tokenisation
// would still pass through to the user's terminal. Substitutions are
// byte-for-byte (see neutralizeEscapes) so the formatter's offset math
// stays valid.
//
// Callers should consult [needsTokenNeutralisation] first; this
// function unconditionally allocates a fresh slice and is intended for
// the rare line that does carry an escape.
func neutralizeTokens(tokens []source.Token) []source.Token {
	if len(tokens) == 0 {
		return tokens
	}
	out := make([]source.Token, len(tokens))
	for i, tok := range tokens {
		tok.Value = neutralizeEscapes(tok.Value)
		out[i] = tok
	}
	return out
}

// formatter selects a Chroma terminal formatter matching the active
// [term.ColorDepth]. Returns nil when the depth is mono so callers
// short-circuit to raw text.
func (r *codeRenderer) formatter() chroma.Formatter {
	switch r.deps.Capabilities.ColorDepth {
	case term.ColorMono:
		return nil
	case term.ColorANSI16:
		return getFormatter("terminal16")
	case term.ColorTrueColor:
		return getFormatter("terminal16m")
	case term.ColorANSI256:
		return getFormatter("terminal256")
	default:
		return getFormatter("terminal256")
	}
}

func getFormatter(name string) chroma.Formatter {
	if fm := formatters.Get(name); fm != nil {
		return fm
	}
	return formatters.Fallback
}

// chromaIterFromTokens builds a [chroma.Iterator] closure that yields
// one Chroma token per source.Token in `tokens`. Each call advances by
// one until the slice is drained, after which [chroma.EOF] is returned
// indefinitely.
func chromaIterFromTokens(tokens []source.Token) chroma.Iterator {
	i := 0
	return func() chroma.Token {
		if i >= len(tokens) {
			return chroma.EOF
		}
		t := chroma.Token{Type: tokens[i].Type, Value: tokens[i].Value}
		i++
		return t
	}
}

// lineExceedsWidth reports whether the line content (after the gutter
// prefix) is longer than the available column width — i.e. whether the
// line would wrap. Uses rune count, matching writeWrappedLine.
func lineExceedsWidth(raw string, prefixLen, width int) bool {
	contentWidth := width - prefixLen
	if contentWidth <= 0 {
		return true
	}
	return runeCount(raw) > contentWidth
}

// runeCount counts Unicode code points in `s`. Matches utf8.RuneCountInString
// without importing it transitively at every call site.
func runeCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
