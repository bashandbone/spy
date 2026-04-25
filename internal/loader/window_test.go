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

func (n *nonSeekableSource) Kind() source.Kind        { return source.KindText }
func (n *nonSeekableSource) DisplayName() string      { return "<stdin>" }
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
