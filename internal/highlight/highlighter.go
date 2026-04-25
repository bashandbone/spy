// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package highlight

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// Warning is the structured advisory the [Highlighter] emits on its
// [Highlighter.Warns] side channel. The UI surfaces it in the status
// bar with a 5 s auto-clear (per contracts/internal-apis.md "internal/
// highlight").
type Warning struct {
	Kind WarnKind
	Cap  int64
}

// WarnKind enumerates the side-channel advisory categories.
type WarnKind int

const (
	// WarnHighlightDisabled fires once per session when the cumulative
	// processed bytes exceed the active cap (or the cap was 0 from the
	// start). Subsequent lines short-circuit to a single Text token.
	WarnHighlightDisabled WarnKind = iota
)

// Highlighter is the per-session syntax-highlighting engine. It wraps
// Chroma lexers, caches them per language, and enforces the
// HighlightCap byte budget. The zero value is unsafe; construct via
// [New].
//
// All exported methods are goroutine-safe; the lexer cache is guarded
// by a mutex, the cap and budget counters use atomics, and the warning
// side channel is one-shot via [Highlighter.warned].
type Highlighter struct {
	style *chroma.Style
	depth term.ColorDepth

	cap            atomic.Int64
	bytesProcessed atomic.Int64
	disabled       atomic.Bool
	warned         atomic.Bool

	mu     sync.Mutex
	lexers map[string]chroma.Lexer
	lang   string

	warns chan Warning
}

// New builds a [Highlighter] for the supplied theme/depth/cap triple.
// A nil `style` falls back to the bundled "monokai" Chroma style;
// `capBytes == 0` disables highlighting entirely (a single
// [WarnHighlightDisabled] fires on first use); `capBytes > 0` is the
// cumulative byte budget above which the highlighter downgrades to
// pass-through Text tokens.
func New(style *chroma.Style, depth term.ColorDepth, capBytes int64) *Highlighter {
	if style == nil {
		style = styles.Get("monokai")
		if style == nil {
			style = styles.Fallback
		}
	}
	h := &Highlighter{
		style:  style,
		depth:  depth,
		lexers: make(map[string]chroma.Lexer),
		warns:  make(chan Warning, 1),
	}
	h.cap.Store(capBytes)
	return h
}

// SetLang records the active language used by [Highlighter.HighlightStream]
// when chunks arrive without a per-call language. The language follows
// Chroma's lexer-name convention (e.g. "go", "python", "markdown").
func (h *Highlighter) SetLang(lang string) {
	h.mu.Lock()
	h.lang = lang
	h.mu.Unlock()
}

// Lang returns the active language set via [Highlighter.SetLang], or
// "" when none has been set.
func (h *Highlighter) Lang() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lang
}

// SetCap adjusts the byte budget at runtime. A new cap of 0 immediately
// disables highlighting (firing [WarnHighlightDisabled] once); a positive
// cap below the current cumulative byte count also disables.
func (h *Highlighter) SetCap(bytes int64) {
	h.cap.Store(bytes)
	if bytes == 0 {
		h.markDisabled(0)
		return
	}
	if bytes > 0 && h.bytesProcessed.Load() > bytes {
		h.markDisabled(bytes)
	}
}

// Cap returns the active byte budget.
func (h *Highlighter) Cap() int64 {
	return h.cap.Load()
}

// Warns returns the advisory side channel for [Warning] values. The
// channel is buffered to 1 and drops further sends when full so the
// producer never blocks. It is intended to remain open for the
// lifetime of the highlighter / session rather than being closed to
// signal completion; consumers should drain it non-blockingly on
// every Update tick (Copilot review PR#8 #6 — earlier docs claimed
// the channel closed when the highlighter was done, but no path
// closed it and consumers don't depend on the close signal).
func (h *Highlighter) Warns() <-chan Warning {
	return h.warns
}

// Disabled reports whether the highlighter has irrevocably switched
// to pass-through mode for this session.
func (h *Highlighter) Disabled() bool {
	if h == nil {
		return false
	}
	return h.disabled.Load()
}

// Highlight runs Chroma against a single line. Returns one or more
// tokens; an unknown language, lex error, or exhausted cap yields a
// single Text token carrying the line verbatim.
func (h *Highlighter) Highlight(lang, line string) []source.Token {
	if h == nil {
		return []source.Token{{Type: chroma.Text, Value: line}}
	}
	if h.disabled.Load() {
		return []source.Token{{Type: chroma.Text, Value: line}}
	}
	cap := h.cap.Load()
	if cap == 0 {
		h.markDisabled(0)
		return []source.Token{{Type: chroma.Text, Value: line}}
	}
	if cap > 0 {
		n := h.bytesProcessed.Add(int64(len(line)))
		if n > cap {
			h.markDisabled(cap)
			return []source.Token{{Type: chroma.Text, Value: line}}
		}
	}
	lex := h.lookupLexer(lang)
	if lex == nil {
		return []source.Token{{Type: chroma.Text, Value: line}}
	}
	iter, err := lex.Tokenise(nil, line)
	if err != nil {
		return []source.Token{{Type: chroma.Text, Value: line}}
	}
	return tokensFromIterator(iter)
}

// HighlightStream consumes chunks from `in` and forwards them on `out`
// with [source.Line.Tokens] populated for any line that arrived without
// pre-computed tokens. Closes `out` when `in` closes or `ctx` is
// cancelled.
//
// The active language comes from [Highlighter.SetLang]; lines whose
// Tokens slice is already non-nil are passed through unchanged so a
// caller can pre-populate without re-lexing.
func (h *Highlighter) HighlightStream(ctx context.Context, in <-chan loader.Chunk, out chan<- loader.Chunk) {
	defer close(out)
	lang := h.Lang()
	for {
		select {
		case <-ctx.Done():
			return
		case c, ok := <-in:
			if !ok {
				return
			}
			for i := range c.Lines {
				if c.Lines[i].Tokens != nil {
					continue
				}
				c.Lines[i].Tokens = h.Highlight(lang, c.Lines[i].Raw)
			}
			select {
			case <-ctx.Done():
				return
			case out <- c:
			}
		}
	}
}

// Style returns the Chroma style this highlighter was constructed with.
// Renderers consume it when formatting tokens to ANSI.
func (h *Highlighter) Style() *chroma.Style {
	if h == nil || h.style == nil {
		return styles.Fallback
	}
	return h.style
}

// Depth returns the configured color depth so renderers can choose the
// matching Chroma terminal formatter.
func (h *Highlighter) Depth() term.ColorDepth {
	if h == nil {
		return term.ColorANSI256
	}
	return h.depth
}

func (h *Highlighter) markDisabled(cap int64) {
	if h.disabled.Swap(true) {
		return
	}
	if h.warned.Swap(true) {
		return
	}
	select {
	case h.warns <- Warning{Kind: WarnHighlightDisabled, Cap: cap}:
	default:
	}
}

func (h *Highlighter) lookupLexer(lang string) chroma.Lexer {
	if lang == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if lex, ok := h.lexers[lang]; ok {
		return lex
	}
	lex := lexers.Get(lang)
	if lex == nil {
		return nil
	}
	lex = chroma.Coalesce(lex)
	h.lexers[lang] = lex
	return lex
}

// tokensFromIterator drains a Chroma iterator into a slice of
// source.Token. The iterator is consumed in full.
func tokensFromIterator(iter chroma.Iterator) []source.Token {
	var tokens []source.Token
	for {
		t := iter()
		if t == chroma.EOF {
			break
		}
		tokens = append(tokens, source.Token{Type: t.Type, Value: t.Value})
	}
	return tokens
}
