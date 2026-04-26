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
//
// The renderer maintains a per-line formatted-output cache (lineNum →
// styled string) so subsequent repaints in the same theme avoid
// re-invoking Chroma for lines that were already visible. The cache is
// implicitly invalidated whenever the renderer is rebuilt (theme swap,
// word-wrap toggle, line-number toggle, etc.).
type codeRenderer struct {
	deps              Dependencies
	cache             map[int64]string // lineNum → Chroma-formatted string; populated on demand
	lastResidentStart int64            // last known resident start line; used to prune stale cache entries
}

// Render walks the resident buffer and emits one formatted line per
// source line, with optional gutter line numbers and rune-bound
// soft-wrap.
//
// Only lines within the viewport's visible window are syntax-highlighted
// (SC-004); lines outside that window are emitted as raw text so
// re-render cost is bounded by the viewport height, not by the total
// buffer size. Wrap is still applied to out-of-window lines so the
// viewport's row-count arithmetic and max-scroll position stay correct.
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

	// Viewport window in visual-row coordinates. When height is zero
	// (first paint before WindowSizeMsg arrives), treat every line as
	// visible so the initial render populates the cache.
	yOffset := ctx.Viewport.YOffset
	height := ctx.Viewport.Height
	viewportKnown := height > 0
	viewEnd := yOffset + height

	active, hasActive := activeMatch(ctx.Search)
	// In mono mode (`--no-color` / NO_COLOR=1 / TERM=dumb), the match
	// overlay must not emit ANSI — even via lipgloss styles applied
	// post-chroma — or the rendered output becomes a contract
	// violation (Copilot review PR#9 round-3 #1). When mono is active
	// we still mark matched lines but as raw text without colouring.
	mono := ctx.Theme.Mono || ctx.Capabilities.ColorDepth == term.ColorMono

	// Prune cache entries for lines that the LineBuffer has already
	// evicted from its resident window. This keeps the cache bounded by
	// the live resident range rather than the cumulative set of all
	// lines ever scrolled through.
	residentStart := ctx.Buffer.ResidentStartLine()
	if residentStart > r.lastResidentStart {
		r.pruneCacheBefore(residentStart)
		r.lastResidentStart = residentStart
	}

	visualRow := 0
	for _, l := range lines {
		prefix := ""
		if r.deps.LineNumbers {
			prefix = fmt.Sprintf("%*d  ", gutter, l.Number)
		}
		prefixLen := len(prefix)

		// Lines with search matches use the dedicated overlay path so
		// the highlight wraps tightly around the match span. The
		// documented limitation: matched lines lose chroma syntax
		// colour; the caret/active match is still visible.
		lineMatches := matchesForLine(ctx.Search, l.Number)
		hasMatches := len(lineMatches) > 0

		// Compute the visual rows this source line will actually occupy in
		// the rendered output. Match-overlay lines always emit exactly one
		// row (no wrapping); all other lines may wrap when WordWrap is on.
		// Using the actual row count here keeps visualRow in sync with the
		// rendered output so inViewport stays accurate for subsequent lines.
		lineRows := 1
		if !hasMatches && r.deps.WordWrap && width > prefixLen {
			lineRows = visualRowsForLine(l.Raw, width-prefixLen)
		}
		lineVisualEnd := visualRow + lineRows

		// Is this line within the visible viewport window?
		// Interval overlap: line [visualRow, lineVisualEnd) overlaps viewport
		// [yOffset, viewEnd) when lineVisualEnd > yOffset && visualRow < viewEnd.
		// A line whose last row equals yOffset is just above the window (excluded);
		// a line whose first row equals viewEnd is just below (excluded).
		inViewport := !viewportKnown || (lineVisualEnd > yOffset && visualRow < viewEnd)

		switch {
		case hasMatches:
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

		case inViewport && r.deps.WordWrap && width > 0 && lineExceedsWidth(l.Raw, prefixLen, width):
			// In viewport, long line, wrap on: fall back to raw text so
			// wrap math stays correct. The styled emission is reserved for
			// lines that fit on a single visual row.
			writeWrappedLine(&b, prefix, neutralizeEscapes(l.Raw), width)

		case inViewport:
			// In viewport, fits or no-wrap mode: emit syntax-highlighted.
			styled := r.styledCached(l)
			b.WriteString(prefix)
			b.WriteString(styled)
			b.WriteByte('\n')

		case r.deps.WordWrap && width > 0 && lineExceedsWidth(l.Raw, prefixLen, width):
			// Outside viewport, long line, wrap on: emit raw wrapped to
			// preserve the correct visual row count for scroll arithmetic.
			writeWrappedLine(&b, prefix, neutralizeEscapes(l.Raw), width)

		default:
			// Outside viewport, short line or no-wrap: emit raw text.
			b.WriteString(prefix)
			b.WriteString(neutralizeEscapes(l.Raw))
			b.WriteByte('\n')
		}

		visualRow = lineVisualEnd
	}
	return b.String()
}

// styledCached returns the Chroma-formatted rendition of l, using the
// renderer's per-line cache to avoid re-invoking Chroma for lines that
// have already been painted in this renderer's lifetime. The cache is
// implicitly invalidated when the renderer is rebuilt (theme swap,
// word-wrap toggle, line-number toggle, etc.).
func (r *codeRenderer) styledCached(l source.Line) string {
	if r.cache != nil {
		if cached, ok := r.cache[l.Number]; ok {
			return cached
		}
	}
	styled := r.styleLine(l)
	if r.cache == nil {
		r.cache = make(map[int64]string)
	}
	r.cache[l.Number] = styled
	return styled
}

// pruneCacheBefore deletes all cache entries for line numbers strictly
// less than `firstResident`. Called when the LineBuffer's resident
// window advances (windowed-mode eviction) so styled strings for
// evicted lines don't accumulate indefinitely.
func (r *codeRenderer) pruneCacheBefore(firstResident int64) {
	for lineNum := range r.cache {
		if lineNum < firstResident {
			delete(r.cache, lineNum)
		}
	}
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
// hundreds of bytes; the byte scan is one cache-line read in the
// common case. Uses containsRawEscByte (not strings.ContainsAny) to
// avoid the false positive where a literal U+FFFD in source content
// would match the rune-decoded form of standalone 0x9b (invalid
// UTF-8) and force a pointless full re-scan.
func needsTokenNeutralisation(raw string) bool {
	return containsRawEscByte(raw)
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
