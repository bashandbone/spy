// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// StdinSource is the [Source] implementation backed by a non-seekable
// stream — typically `os.Stdin` when spy is part of a shell pipeline.
// Per research R5 the stream is read at most once; the loader copies
// bytes into the in-memory ring buffer + sliding window.
//
// These tests pin the contract callers depend on:
//   - DisplayName is the literal "<stdin>" so footers don't try to
//     basename a non-path string.
//   - Open returns a reader exactly once (subsequent calls error out
//     with ErrAlreadyConsumed) so a buggy caller can't accidentally
//     drain the stream twice and silently produce empty content.
//   - Reopen returns ErrNotSeekable so windowed-mode windowing degrades
//     gracefully via the documented WarnStdinNonSeekable advisory
//     instead of panicking.

func TestStdinSource_DisplayName(t *testing.T) {
	src := NewStdinSource(strings.NewReader("hello\n"), "")
	if got := src.DisplayName(); got != "<stdin>" {
		t.Errorf("DisplayName: got %q want %q", got, "<stdin>")
	}
}

func TestStdinSource_OpenReturnsContentOnce(t *testing.T) {
	body := "package main\nfunc main(){}\n"
	src := NewStdinSource(strings.NewReader(body), "go")
	rc, err := src.Open()
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	rc.Close()
	if string(got) != body {
		t.Errorf("read content: got %q want %q", got, body)
	}
}

func TestStdinSource_SecondOpenIsErrAlreadyConsumed(t *testing.T) {
	src := NewStdinSource(strings.NewReader("hi"), "")
	rc, err := src.Open()
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	_, _ = io.ReadAll(rc)
	rc.Close()

	_, err = src.Open()
	if err == nil {
		t.Fatal("second Open should error")
	}
	if !errors.Is(err, ErrAlreadyConsumed) {
		t.Errorf("expected ErrAlreadyConsumed, got %v", err)
	}
}

func TestStdinSource_ReopenIsErrNotSeekable(t *testing.T) {
	src := NewStdinSource(strings.NewReader("hi"), "")
	_, err := src.Reopen()
	if err == nil {
		t.Fatal("Reopen should error for stdin")
	}
	if !errors.Is(err, ErrNotSeekable) {
		t.Errorf("expected ErrNotSeekable, got %v", err)
	}
}

func TestStdinSource_KindFromHint(t *testing.T) {
	// Hint takes priority over content when set, mirroring FileSource.
	body := "totally not a python file\n"
	src := NewStdinSource(strings.NewReader(body), "go")
	if got := src.Kind(); got != KindCode {
		t.Errorf("Kind with hint=go: got %v want KindCode", got)
	}
	if md := src.Metadata(); md.Language == "" {
		t.Errorf("Metadata.Language: empty when hint resolved a Code lexer")
	}
}

func TestStdinSource_KindFromShebang(t *testing.T) {
	// No hint; first line is a shebang. Chroma's per-lexer AnalyzeText
	// picks Python from "#!/usr/bin/env python".
	body := "#!/usr/bin/env python\nprint('hi')\n"
	src := NewStdinSource(strings.NewReader(body), "")
	got := src.Kind()
	if got != KindCode {
		t.Errorf("shebang detection: got %v want KindCode", got)
	}
}

func TestStdinSource_KindPlainTextFallback(t *testing.T) {
	body := "Just a plain note. Nothing interesting.\n"
	src := NewStdinSource(strings.NewReader(body), "")
	if got := src.Kind(); got != KindText {
		t.Errorf("plain text fallback: got %v want %v", got, KindText)
	}
}

func TestStdinSource_OpenReplaysPeekedBytes(t *testing.T) {
	// Detection consumes the first 8 KiB of the underlying reader. The
	// subsequent Open() must replay those bytes so the loader sees the
	// full stream from byte 0 — verifying the bytes.Buffer / MultiReader
	// pattern from T092.
	body := "package main\n" + strings.Repeat("a", 16384) + "\nlast line\n"
	src := NewStdinSource(strings.NewReader(body), "")
	// trigger detection
	_ = src.Kind()
	rc, err := src.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	rc.Close()
	if string(got) != body {
		t.Errorf("Open should replay peeked bytes; len got=%d want=%d", len(got), len(body))
	}
}

// blockingReader is an [io.Reader] that returns one chunk and then
// blocks indefinitely on the next call — modeling `tail -f` and
// other long-lived producers that don't write 8 KiB up front. Used to
// pin the Copilot review PR#12 #6 contract: detectOnce must not
// require the full peek window before returning.
type blockingReader struct {
	first  []byte
	served bool
	block  chan struct{}
}

func newBlockingReader(first []byte) *blockingReader {
	return &blockingReader{first: first, block: make(chan struct{})}
}

func (b *blockingReader) Read(p []byte) (int, error) {
	if !b.served {
		b.served = true
		n := copy(p, b.first)
		return n, nil
	}
	// Block forever — simulates a producer that has more bytes coming
	// but isn't writing them yet (tail -f, long-lived pipe, etc.).
	<-b.block
	return 0, io.EOF
}

func TestStdinSource_PeekDoesNotBlockOnStreamingInput(t *testing.T) {
	// Copilot review PR#12 #6: detectOnce must not call io.ReadFull —
	// that would hang on `tail -f | spy` until 8 KiB are written. A
	// single Read returning the producer's first chunk must be enough
	// for detection.
	first := []byte("#!/usr/bin/env python\nprint('streaming')\n")
	reader := newBlockingReader(first)
	src := NewStdinSource(reader, "")

	done := make(chan Kind, 1)
	go func() { done <- src.Kind() }()

	select {
	case got := <-done:
		if got != KindCode {
			t.Errorf("streaming shebang detection: got %v want %v", got, KindCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Kind() blocked on a streaming producer; detectOnce must not call io.ReadFull")
	}
}

func TestStdinSource_HintSkipsPeekOnStreamingInput(t *testing.T) {
	// Hint short-circuit: when `--lang` (or any explicit hint) already
	// resolves a Kind/lexer, detectOnce should skip the peek entirely
	// so the loader can stream from byte 0. A producer that never
	// writes anything (and never closes) must still produce a valid
	// session if the hint is set (Copilot review PR#12 #6).
	never := newBlockingReader(nil) // never writes anything
	src := NewStdinSource(never, "go")

	done := make(chan Kind, 1)
	go func() { done <- src.Kind() }()

	select {
	case got := <-done:
		if got != KindCode {
			t.Errorf("hint short-circuit: got %v want %v", got, KindCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hint=go must short-circuit the peek; got blocked")
	}
}

func TestStdinSource_Metadata(t *testing.T) {
	src := NewStdinSource(strings.NewReader("hi"), "")
	md := src.Metadata()
	if md.Path != "<stdin>" {
		t.Errorf("Metadata.Path: got %q want %q", md.Path, "<stdin>")
	}
	if md.LineCount != -1 {
		t.Errorf("Metadata.LineCount before streaming: got %d want -1", md.LineCount)
	}
	// Stdin sources have no on-disk size. The loader fills LineCount
	// once streaming completes; Size stays 0.
	if md.Size != 0 {
		t.Errorf("Metadata.Size: stdin should report 0, got %d", md.Size)
	}
}
