// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package highlight

import (
	"context"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

func TestNew_ReturnsNonNil(t *testing.T) {
	h := New(nil, term.ColorANSI256, 5*1024*1024)
	if h == nil {
		t.Fatal("New returned nil")
	}
	if h.Style() == nil {
		t.Errorf("Style() should fall back to a real chroma.Style on nil input")
	}
}

func TestNew_ZeroCapDoesNotImmediatelyDisable(t *testing.T) {
	// The cap is enforced lazily on the first Highlight call so the UI
	// can construct the highlighter before any lines arrive.
	h := New(nil, term.ColorANSI256, 0)
	if h.Disabled() {
		t.Errorf("Disabled() should remain false until first Highlight call")
	}
}

func TestDisabled_NilSafe(t *testing.T) {
	var h *Highlighter
	if h.Disabled() {
		t.Errorf("nil Highlighter must report Disabled=false (defensive)")
	}
}

func TestHighlight_NilHighlighterReturnsTextToken(t *testing.T) {
	var h *Highlighter
	toks := h.Highlight("go", "anything")
	if len(toks) != 1 || toks[0].Type != chroma.Text || toks[0].Value != "anything" {
		t.Errorf("nil Highlighter should pass-through as Text token; got %+v", toks)
	}
}

func TestHighlight_LangAutoSelectsLexer(t *testing.T) {
	h := New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024)
	toks := h.Highlight("go", "func main() {}")
	if len(toks) <= 1 {
		t.Fatalf("expected lexer to produce multiple tokens for go code, got %d", len(toks))
	}
	foundKeyword := false
	for _, tok := range toks {
		if tok.Type.InCategory(chroma.Keyword) {
			foundKeyword = true
			break
		}
	}
	if !foundKeyword {
		t.Errorf("no keyword token in highlighted go code: %+v", toks)
	}
}

func TestHighlight_UnknownLanguageFallback(t *testing.T) {
	h := New(nil, term.ColorANSI256, 5*1024*1024)
	toks := h.Highlight("xyzpdq-not-a-real-lang", "anything")
	if len(toks) != 1 {
		t.Fatalf("unknown lang should produce 1 fallback token, got %d", len(toks))
	}
	if toks[0].Type != chroma.Text {
		t.Errorf("fallback token should be Text, got %v", toks[0].Type)
	}
	if toks[0].Value != "anything" {
		t.Errorf("fallback token value: got %q want %q", toks[0].Value, "anything")
	}
}

func TestHighlight_EmptyLangReturnsTextToken(t *testing.T) {
	h := New(nil, term.ColorANSI256, 5*1024*1024)
	toks := h.Highlight("", "alpha beta gamma")
	if len(toks) != 1 || toks[0].Type != chroma.Text {
		t.Errorf("empty lang should pass-through as Text; got %+v", toks)
	}
}

func TestHighlight_CapZeroDisablesAndWarns(t *testing.T) {
	h := New(nil, term.ColorANSI256, 0)
	toks := h.Highlight("go", "func main() {}")
	if len(toks) != 1 || toks[0].Type != chroma.Text {
		t.Errorf("cap=0 should produce a single Text token, got %+v", toks)
	}
	if !h.Disabled() {
		t.Errorf("Highlighter.Disabled() should be true after cap=0 first call")
	}
	select {
	case w := <-h.Warns():
		if w.Kind != WarnHighlightDisabled {
			t.Errorf("warning kind: got %v want WarnHighlightDisabled", w.Kind)
		}
		if w.Cap != 0 {
			t.Errorf("warning cap: got %d want 0", w.Cap)
		}
	default:
		t.Errorf("expected a WarnHighlightDisabled on cap=0")
	}
}

func TestHighlight_CapPositiveSwitchesAfterByteBudget(t *testing.T) {
	// 10-byte budget; a longer line trips the cap mid-session.
	h := New(styles.Get("monokai"), term.ColorANSI256, 10)
	// First small line: highlighted (well under the budget).
	first := h.Highlight("go", "x")
	if len(first) == 0 {
		t.Errorf("first line under cap should produce tokens")
	}
	if h.Disabled() {
		t.Fatalf("should not be disabled after a 1-byte line under a 10-byte cap")
	}
	// Process a line longer than the remaining budget.
	h.Highlight("go", "this line is more than ten bytes long")
	if !h.Disabled() {
		t.Errorf("expected Highlighter to disable once cap exceeded")
	}
	select {
	case w := <-h.Warns():
		if w.Cap != 10 {
			t.Errorf("warning cap: got %d want 10", w.Cap)
		}
	default:
		t.Errorf("expected WarnHighlightDisabled on cap exceeded")
	}
	// Subsequent lines stay disabled.
	toks := h.Highlight("go", "x")
	if len(toks) != 1 || toks[0].Type != chroma.Text {
		t.Errorf("post-disable line should produce Text token, got %+v", toks)
	}
}

func TestHighlight_WarnsExactlyOnce(t *testing.T) {
	h := New(nil, term.ColorANSI256, 0)
	// Three triggers; only the first emits.
	for i := 0; i < 3; i++ {
		h.Highlight("go", "a")
	}
	select {
	case <-h.Warns():
	default:
		t.Errorf("expected at least one warning after cap=0 triggers")
	}
	select {
	case w := <-h.Warns():
		t.Errorf("got duplicate warning: %+v", w)
	default:
	}
}

func TestHighlightStream_PopulatesTokens(t *testing.T) {
	h := New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024)
	h.SetLang("go")
	in := make(chan loader.Chunk, 2)
	out := make(chan loader.Chunk, 2)
	in <- loader.Chunk{
		StartLine: 1,
		Lines: []source.Line{
			{Number: 1, Raw: "package main"},
			{Number: 2, Raw: "func main() {}"},
		},
	}
	close(in)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.HighlightStream(ctx, in, out)
	var got []loader.Chunk
	for c := range out {
		got = append(got, c)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk on out, got %d", len(got))
	}
	for i, l := range got[0].Lines {
		if l.Tokens == nil {
			t.Errorf("line %d Tokens is nil; HighlightStream did not populate", i)
		}
	}
}

func TestHighlightStream_PreservesPrecomputedTokens(t *testing.T) {
	h := New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024)
	h.SetLang("go")
	already := []source.Token{{Type: chroma.Text, Value: "preset"}}
	in := make(chan loader.Chunk, 1)
	out := make(chan loader.Chunk, 1)
	in <- loader.Chunk{
		StartLine: 1,
		Lines: []source.Line{
			{Number: 1, Raw: "preset", Tokens: already},
		},
	}
	close(in)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.HighlightStream(ctx, in, out)
	c := <-out
	if &c.Lines[0].Tokens[0] != &already[0] {
		// Best-effort identity check — slice header may be copied but the
		// backing storage must be preserved (no re-lex).
		if len(c.Lines[0].Tokens) != 1 || c.Lines[0].Tokens[0].Value != "preset" {
			t.Errorf("HighlightStream re-lexed pre-tokenised line: %+v", c.Lines[0].Tokens)
		}
	}
}

func TestHighlightStream_RespectCancel(t *testing.T) {
	h := New(nil, term.ColorANSI256, 5*1024*1024)
	h.SetLang("go")
	in := make(chan loader.Chunk)
	out := make(chan loader.Chunk)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.HighlightStream(ctx, in, out)
		close(done)
	}()
	cancel()
	<-done
	// out should now be closed.
	if _, ok := <-out; ok {
		t.Errorf("out should be closed after ctx cancel")
	}
}

func TestSetCap_ImmediatelyDisablesAtZero(t *testing.T) {
	h := New(nil, term.ColorANSI256, 100)
	h.Highlight("go", "x")
	if h.Disabled() {
		t.Fatalf("should not be disabled before SetCap(0)")
	}
	h.SetCap(0)
	if !h.Disabled() {
		t.Errorf("SetCap(0) should immediately disable")
	}
	select {
	case <-h.Warns():
	default:
		t.Errorf("SetCap(0) should fire WarnHighlightDisabled")
	}
}

func TestSetCap_BelowProcessedDisables(t *testing.T) {
	h := New(nil, term.ColorANSI256, 1024*1024)
	// Process a line; bytesProcessed advances.
	h.Highlight("go", "package main")
	// Drop the cap below current processed bytes.
	h.SetCap(5)
	if !h.Disabled() {
		t.Errorf("SetCap below processed bytes should disable")
	}
}

func TestSetCap_PreservesEnabledWhenStillRoom(t *testing.T) {
	h := New(nil, term.ColorANSI256, 1024)
	h.Highlight("go", "x")
	h.SetCap(2048)
	if h.Disabled() {
		t.Errorf("raising the cap with room remaining should not disable")
	}
}

func TestLang_DefaultEmpty(t *testing.T) {
	h := New(nil, term.ColorANSI256, 1024)
	if got := h.Lang(); got != "" {
		t.Errorf("Lang() default: got %q want empty string", got)
	}
}

func TestSetLang_RoundTrip(t *testing.T) {
	h := New(nil, term.ColorANSI256, 1024)
	h.SetLang("python")
	if got := h.Lang(); got != "python" {
		t.Errorf("SetLang round-trip: got %q want python", got)
	}
}

func TestStyleAndDepth_Accessors(t *testing.T) {
	h := New(styles.Get("monokai"), term.ColorTrueColor, 1024)
	if h.Style() == nil {
		t.Errorf("Style() should not be nil")
	}
	if h.Depth() != term.ColorTrueColor {
		t.Errorf("Depth: got %v want ColorTrueColor", h.Depth())
	}
}

func TestStyleAndDepth_NilSafe(t *testing.T) {
	var h *Highlighter
	if h.Style() == nil {
		t.Errorf("nil Highlighter Style() should fall back to a real style")
	}
	if h.Depth() != term.ColorANSI256 {
		t.Errorf("nil Highlighter Depth() should default to ANSI256, got %v", h.Depth())
	}
}
