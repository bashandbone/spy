// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package loader

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"

	"github.com/knitli/spy/internal/source"
)

func TestLineBuffer_AppendAndSlice(t *testing.T) {
	body := repeatLines(50, "line")
	src := &fakeSource{body: body, kind: source.KindText}
	s, err := Open(context.Background(), src, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	collectChunks(t, s)
	buf := s.Buffer
	if buf == nil {
		t.Fatal("Stream.Buffer is nil")
	}
	if buf.Total() != 50 {
		t.Errorf("Total: got %d want 50", buf.Total())
	}
	out := buf.Slice(0, 10)
	if len(out) != 10 {
		t.Errorf("Slice(0,10): got %d lines", len(out))
	}
	if out[0].Number != 1 {
		t.Errorf("first line number: got %d want 1", out[0].Number)
	}
}

func TestLineBuffer_SliceClampsOutOfRange(t *testing.T) {
	src := &fakeSource{body: repeatLines(20, "x"), kind: source.KindText}
	s, err := Open(context.Background(), src, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	collectChunks(t, s)
	buf := s.Buffer
	got := buf.Slice(15, 100)
	if len(got) != 5 {
		t.Errorf("Slice clamp: got %d want 5", len(got))
	}
}

func TestWindowedMode_TriggeredByMaxResidentBytes(t *testing.T) {
	// Use a small MaxResidentBytes so we cross the threshold quickly.
	body := repeatLines(500, strings.Repeat("a", 200))
	src := &fakeSource{body: body, kind: source.KindText}
	cfg := Config{
		MaxResidentBytes: 10 * 1024, // 10 KiB
		WindowSize:       50,
	}
	s, err := Open(context.Background(), src, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	collectChunks(t, s)
	buf := s.Buffer
	if !buf.Windowed() {
		t.Errorf("buffer should have flipped into windowed mode")
	}
	if buf.Total() != 500 {
		t.Errorf("Total even in windowed mode: got %d want 500", buf.Total())
	}
}

func TestWindowedMode_SliceBeyondResidentTriggersReseek(t *testing.T) {
	body := repeatLines(500, strings.Repeat("b", 200))
	src := &fakeSource{body: body, kind: source.KindText}
	cfg := Config{
		MaxResidentBytes: 10 * 1024,
		WindowSize:       50,
	}
	s, err := Open(context.Background(), src, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	collectChunks(t, s)
	buf := s.Buffer
	if !buf.Windowed() {
		t.Skip("threshold not crossed under this test setup")
	}
	// Ask for a window near the start — this should re-seek via
	// src.Reopen() in windowed mode.
	out := buf.Slice(0, 20)
	if len(out) == 0 {
		t.Errorf("Slice in windowed mode returned 0 lines (re-seek path broken?)")
	}
}

func TestWindowedMode_NonSeekableSourceWarns(t *testing.T) {
	// A source whose Reopen returns ErrNotSeekable triggers
	// WarnStdinNonSeekable on Errs the first time the buffer would have
	// re-seeked; subsequent slices fall back to "current resident only".
	body := repeatLines(500, strings.Repeat("c", 200))
	src := &nonSeekableSource{body: body}
	cfg := Config{
		MaxResidentBytes: 10 * 1024,
		WindowSize:       50,
	}
	s, err := Open(context.Background(), src, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	collectChunks(t, s)
	buf := s.Buffer
	if !buf.Windowed() {
		t.Skip("threshold not crossed")
	}
	// Ask for a slice at the start — should attempt re-seek which fails.
	_ = buf.Slice(0, 20)
	// Errs may already be closed by the producer; warnings emitted from
	// post-streaming Slice() calls accumulate on Buffer.Warnings().
	saw := false
	for _, err := range buf.Warnings() {
		if errors.Is(err, ErrStdinNonSeekable) {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected ErrStdinNonSeekable in Buffer.Warnings(), got %v", buf.Warnings())
	}
}

// nonSeekableSource is a [source.Source] whose Reopen returns
// [source.ErrNotSeekable], modelling stdin in windowed mode.
type nonSeekableSource struct {
	body string
}

func (n *nonSeekableSource) Kind() source.Kind         { return source.KindText }
func (n *nonSeekableSource) DisplayName() string       { return "<stdin>" }
func (n *nonSeekableSource) Metadata() source.Metadata { return source.Metadata{LineCount: -1} }
func (n *nonSeekableSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(n.body)), nil
}
func (n *nonSeekableSource) Reopen() (io.ReadSeeker, error) {
	return nil, source.ErrNotSeekable
}

// --- compile-time assertion: T017's deferred LineProvider check. ---
// The interface is defined in `source`; the buffer that implements it
// lives in `loader`. Asserting it here (rather than inside the source
// test package) avoids a transient import cycle and keeps the assertion
// alongside its implementation.
var _ source.LineProvider = (*LineBuffer)(nil)

func TestLineBuffer_SetTokensPropagatesToSlice(t *testing.T) {
	// SetTokens must update the buffer's stored copies so Slice() picks
	// up the new Tokens — the contract Copilot flagged in PR#8 #1/#3.
	buf := NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{
		{Number: 1, Raw: "alpha"},
		{Number: 2, Raw: "beta"},
		{Number: 3, Raw: "gamma"},
	})
	tagged := []source.Line{
		{Number: 2, Tokens: []source.Token{{Type: chroma.Keyword, Value: "beta"}}},
	}
	buf.SetTokens(tagged)
	out := buf.Slice(0, 3)
	if len(out) != 3 {
		t.Fatalf("Slice(0,3): got %d lines", len(out))
	}
	if out[0].Tokens != nil {
		t.Errorf("line 1 should retain nil Tokens (untouched)")
	}
	if len(out[1].Tokens) != 1 || out[1].Tokens[0].Value != "beta" {
		t.Errorf("line 2 Tokens not updated by SetTokens: %+v", out[1].Tokens)
	}
	if out[2].Tokens != nil {
		t.Errorf("line 3 should retain nil Tokens (untouched)")
	}
}

func TestLineBuffer_SetTokensSilentForOutOfRange(t *testing.T) {
	// Lines whose Number falls outside the resident hot region are
	// silently skipped (post windowed-mode eviction).
	buf := NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{
		{Number: 1, Raw: "a"},
	})
	// Line 999 is far outside; should not panic, should not modify line 1.
	buf.SetTokens([]source.Line{
		{Number: 999, Tokens: []source.Token{{Type: chroma.Keyword, Value: "x"}}},
	})
	out := buf.Slice(0, 1)
	if len(out) != 1 || out[0].Tokens != nil {
		t.Errorf("out-of-range SetTokens should not affect resident lines: %+v", out)
	}
}

func TestLineBuffer_SetTokensEmptyInputIsNoOp(t *testing.T) {
	buf := NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{{Number: 1, Raw: "a"}})
	buf.SetTokens(nil)
	buf.SetTokens([]source.Line{})
	out := buf.Slice(0, 1)
	if len(out) != 1 || out[0].Tokens != nil {
		t.Errorf("empty SetTokens should be no-op: %+v", out)
	}
}

func TestLineBuffer_SetTokensOnEmptyBufferIsNoOp(t *testing.T) {
	buf := NewLineBuffer(0, 0, nil)
	// No Append; buf is empty. With no Append/MarkComplete the buffer
	// reports the -1 "unknown" sentinel so SetTokens silently skipping
	// is observable separately from "we know there are zero lines"
	// (Copilot review acceptance M5).
	buf.SetTokens([]source.Line{
		{Number: 1, Tokens: []source.Token{{Type: chroma.Keyword, Value: "x"}}},
	})
	if buf.Total() != -1 {
		t.Errorf("pre-stream buffer Total: got %d want -1 (unknown sentinel)", buf.Total())
	}
}

func TestLineBuffer_SetTokensRespectsWindowedStartLine(t *testing.T) {
	// Trigger windowed mode: maxResidentBytes very small.
	buf := NewLineBuffer(8, 2, nil)
	buf.Append([]source.Line{
		{Number: 1, Raw: "aaaaa"}, // 5 bytes
		{Number: 2, Raw: "bbbbb"}, // 10 bytes total → over cap → evict 1
		{Number: 3, Raw: "ccccc"},
	})
	if !buf.Windowed() {
		t.Skip("buffer did not flip to windowed; skip this scenario")
	}
	residentStart := buf.ResidentStartLine()
	// Trying to SetTokens for an evicted line is silently skipped.
	if residentStart > 1 {
		buf.SetTokens([]source.Line{
			{Number: 1, Tokens: []source.Token{{Type: chroma.Keyword, Value: "evicted"}}},
		})
	}
	// SetTokens for a resident line should still take.
	residentEnd := residentStart + int64(buf.Total()-residentStart+1)
	tagged := source.Line{
		Number: residentEnd - 1,
		Tokens: []source.Token{{Type: chroma.Keyword, Value: "live"}},
	}
	buf.SetTokens([]source.Line{tagged})
	out := buf.Slice(tagged.Number-1, tagged.Number)
	if len(out) != 1 {
		t.Fatalf("Slice for resident tagged line: got %d", len(out))
	}
	if len(out[0].Tokens) != 1 || out[0].Tokens[0].Value != "live" {
		t.Errorf("SetTokens did not propagate to resident line: %+v", out[0].Tokens)
	}
}

// TestLineBuffer_ClearWrapCaches verifies the wrap-cache invalidation
// contract used by the UI's Ctrl-W toggle (T100c) and future
// width-change handlers. Seeds [source.Line.Wrapped] on every resident
// line via direct field access (same package), then confirms
// [LineBuffer.ClearWrapCaches] resets every entry to nil. Living in
// the loader package means we don't have to expose a test-only
// SeedWrapped affordance on the production [LineBuffer] API
// (Copilot review PR#13 #2).
func TestLineBuffer_ClearWrapCaches(t *testing.T) {
	src := &fakeSource{body: repeatLines(8, "alpha beta gamma"), kind: source.KindText}
	s, err := Open(context.Background(), src, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	collectChunks(t, s)
	buf := s.Buffer
	if buf == nil {
		t.Fatal("expected non-nil Buffer")
	}
	// Seed Wrapped on every resident line via direct field access.
	buf.mu.Lock()
	if len(buf.lines) == 0 {
		buf.mu.Unlock()
		t.Fatal("buffer is empty; can't seed wrap caches")
	}
	for i := range buf.lines {
		buf.lines[i].Wrapped = []string{"cached"}
	}
	buf.mu.Unlock()

	buf.ClearWrapCaches()

	buf.mu.Lock()
	defer buf.mu.Unlock()
	for i, l := range buf.lines {
		if l.Wrapped != nil {
			t.Errorf("line %d Wrapped not cleared: %#v", i, l.Wrapped)
		}
	}
}

// TestLineBuffer_ClearWrapCachesEmpty exercises the no-op path: an
// empty buffer must not panic when the toggle handler invokes
// ClearWrapCaches before the first chunk lands.
func TestLineBuffer_ClearWrapCachesEmpty(t *testing.T) {
	buf := newLineBuffer(0, 0, 0, nil)
	buf.ClearWrapCaches() // must not panic.
}

// TestLineBuffer_TotalPreStreamReturnsUnknownSentinel pins the M5
// contract: until the loader has called either [Append] or
// [MarkComplete], [Total] returns -1 (the "unknown" sentinel) so the
// search loop can distinguish "no chunks yet" from "confirmed empty"
// (Copilot review acceptance M5).
func TestLineBuffer_TotalPreStreamReturnsUnknownSentinel(t *testing.T) {
	buf := NewLineBuffer(0, 0, nil)
	if got := buf.Total(); got != -1 {
		t.Errorf("pre-stream Total: got %d want -1", got)
	}
}

// TestLineBuffer_TotalAfterEmptyAppendIsZero verifies the boundary
// between -1 and 0: once Append has fired (even with zero lines, as
// happens when the loader sends a placeholder chunk), Total() flips to
// the running count rather than staying on the unknown sentinel.
func TestLineBuffer_TotalAfterEmptyAppendIsZero(t *testing.T) {
	buf := NewLineBuffer(0, 0, nil)
	buf.Append(nil) // empty append still counts as "stream started".
	if got := buf.Total(); got != 0 {
		t.Errorf("post-empty-Append Total: got %d want 0", got)
	}
}

// TestLineBuffer_TotalAfterMarkCompleteZeroIsZero confirms that a
// confirmed-empty source (loader read EOF on the first chunk and
// called MarkComplete(0)) reports 0, not the unknown sentinel.
func TestLineBuffer_TotalAfterMarkCompleteZeroIsZero(t *testing.T) {
	buf := NewLineBuffer(0, 0, nil)
	buf.MarkComplete(0)
	if got := buf.Total(); got != 0 {
		t.Errorf("post-MarkComplete(0) Total: got %d want 0", got)
	}
}

// TestLineBuffer_TotalAfterAppend reports the running count once Append
// has fed the buffer.
func TestLineBuffer_TotalAfterAppend(t *testing.T) {
	buf := NewLineBuffer(0, 0, nil)
	buf.Append([]source.Line{
		{Number: 1, Raw: "alpha"},
		{Number: 2, Raw: "beta"},
	})
	if got := buf.Total(); got != 2 {
		t.Errorf("post-Append Total: got %d want 2", got)
	}
}
