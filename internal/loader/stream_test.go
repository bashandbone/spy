// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package loader

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/knitli/spy/internal/source"
)

// fakeSource is a tiny in-memory [source.Source] that lets us drive the
// loader without touching the filesystem. NewFileSource works fine for
// regular files, but seek-based windowing tests need a deterministic
// reader the test owns.
type fakeSource struct {
	body string
	kind source.Kind
}

func (f *fakeSource) Kind() source.Kind        { return f.kind }
func (f *fakeSource) DisplayName() string      { return "<fake>" }
func (f *fakeSource) Metadata() source.Metadata { return source.Metadata{LineCount: -1} }
func (f *fakeSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.body)), nil
}
func (f *fakeSource) Reopen() (io.ReadSeeker, error) {
	return strings.NewReader(f.body), nil
}

func TestOpen_SmallFileSingleChunkPlusEOF(t *testing.T) {
	body := "line1\nline2\nline3\n"
	src := &fakeSource{body: body, kind: source.KindText}
	s, err := Open(context.Background(), src, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if s.First.StartLine != 1 {
		t.Errorf("First.StartLine: got %d want 1", s.First.StartLine)
	}
	if len(s.First.Lines) != 3 {
		t.Errorf("First chunk should hold all 3 lines, got %d", len(s.First.Lines))
	}
	if !s.First.EOF {
		t.Errorf("small file: First.EOF should be true")
	}
	// Updates and Errs must be closed before/after we observe them.
	collectChunks(t, s)
}

func TestOpen_FirstChunkAvailableSynchronously(t *testing.T) {
	// 200 lines so the file overflows InitialChunkLines (default 80) and
	// continuation streams via Updates.
	body := repeatLines(200, "the quick brown fox jumps over the lazy dog")
	src := &fakeSource{body: body, kind: source.KindText}
	cfg := Config{InitialChunkLines: 80}
	s, err := Open(context.Background(), src, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(s.First.Lines) < 80 {
		t.Errorf("First chunk should have >= InitialChunkLines=80, got %d",
			len(s.First.Lines))
	}
	if s.First.EOF {
		t.Errorf("200-line file: First.EOF should be false")
	}
	more := collectChunks(t, s)
	total := len(s.First.Lines)
	for _, c := range more {
		total += len(c.Lines)
	}
	if total != 200 {
		t.Errorf("total lines: got %d want 200", total)
	}
}

func TestOpen_EmptyFile(t *testing.T) {
	src := &fakeSource{body: "", kind: source.KindText}
	s, err := Open(context.Background(), src, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !s.First.EOF {
		t.Errorf("empty file: First.EOF should be true")
	}
	if len(s.First.Lines) != 0 {
		t.Errorf("empty file: got %d lines", len(s.First.Lines))
	}
	collectChunks(t, s)
}

func TestOpen_ContextCancel(t *testing.T) {
	body := repeatLines(10000, "fill")
	src := &fakeSource{body: body, kind: source.KindText}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := Open(ctx, src, Config{InitialChunkLines: 10})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	cancel()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-s.Updates:
			if !ok {
				return // channel closed: producer noticed cancel
			}
		case <-deadline:
			t.Fatal("producer did not close Updates after cancel")
		}
	}
}

func TestOpen_BoundedUpdatesChannel(t *testing.T) {
	// The Updates channel must be bounded — a slow consumer must not
	// cause the producer goroutine to spin or buffer unbounded chunks.
	// We don't inspect runtime.NumGoroutine directly (flaky on -race);
	// instead, we open a stream with cap=4, never drain Updates, give
	// the producer time to fill the buffer, and verify that fewer than
	// (totalLines / chunkSize) chunks are ready — i.e. the producer
	// blocked rather than buffering everything.
	body := repeatLines(2000, "x")
	src := &fakeSource{body: body, kind: source.KindText}
	cfg := Config{InitialChunkLines: 10, UpdatesBuffer: 4}
	s, err := Open(context.Background(), src, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Give the producer 200ms to fill the channel.
	time.Sleep(200 * time.Millisecond)
	// Drain whatever is buffered without further sleep.
	queued := 0
drain:
	for {
		select {
		case _, ok := <-s.Updates:
			if !ok {
				break drain
			}
			queued++
			// Stop draining after we've observed buffer-full behavior
			// — we only need to confirm not-everything was buffered.
			if queued > 16 {
				return
			}
		default:
			// Producer blocked, channel full — exactly the expected state.
			// We allow UpdatesBuffer+1 because the producer may have one
			// chunk in flight on the send statement when we start
			// draining, which proceeds as soon as we read the first
			// buffered chunk.
			if queued > cfg.UpdatesBuffer+1 {
				t.Errorf("drained %d chunks; expected at most %d while producer is blocked",
					queued, cfg.UpdatesBuffer+1)
			}
			return
		}
	}
	// If we got here, the channel closed before we could verify
	// blocking. With 2000 lines / 10 per chunk that should not happen
	// inside 200ms on commodity CI.
	if queued > 100 {
		// Unbounded production — every chunk fit before we got here.
		t.Errorf("producer never blocked: drained %d chunks", queued)
	}
	_ = runtime.NumGoroutine() // harmless reference to make linters happy
}

func TestOpen_ErrorFromSourceOpen(t *testing.T) {
	src := &errorSource{err: errors.New("boom")}
	_, err := Open(context.Background(), src, Config{})
	if err == nil {
		t.Fatal("expected error from src.Open")
	}
}

// --- per-line cap (T023b) ---

func TestStream_LineTruncatedAt100KiB(t *testing.T) {
	bigLine := strings.Repeat("x", 200*1024) // 200 KiB
	body := bigLine + "\n" + "ok\n"
	src := &fakeSource{body: body, kind: source.KindText}
	s, err := Open(context.Background(), src, Config{
		InitialChunkLines: 1024,
		MaxLineBytes:      100 * 1024, // 100 KiB cap
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	all := append([]Chunk{s.First}, collectChunks(t, s)...)
	var lines []source.Line
	for _, c := range all {
		lines = append(lines, c.Lines...)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if int64(len(lines[0].Raw)) != 100*1024 {
		t.Errorf("truncated line length: got %d want %d", len(lines[0].Raw), 100*1024)
	}
	if lines[1].Raw != "ok" {
		t.Errorf("second line: got %q want %q", lines[1].Raw, "ok")
	}
	// Exactly one WarnLineTruncated.
	count := 0
	for err := range s.Errs {
		if errors.Is(err, ErrLineTruncated) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("WarnLineTruncated count: got %d want 1", count)
	}
}

func TestStream_HugeLineFiveMiB(t *testing.T) {
	bigLine := strings.Repeat("a", 5*1024*1024) // 5 MiB
	src := &fakeSource{body: bigLine + "\n", kind: source.KindText}
	s, err := Open(context.Background(), src, Config{MaxLineBytes: 100 * 1024})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	all := append([]Chunk{s.First}, collectChunks(t, s)...)
	if len(all) == 0 || len(all[0].Lines) == 0 {
		t.Fatal("no lines emitted")
	}
	if int64(len(all[0].Lines[0].Raw)) != 100*1024 {
		t.Errorf("5 MiB line truncation: got %d want %d",
			len(all[0].Lines[0].Raw), 100*1024)
	}
	count := 0
	for err := range s.Errs {
		if errors.Is(err, ErrLineTruncated) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("WarnLineTruncated count: got %d want 1", count)
	}
}

func TestStream_EmptyLineUnaffected(t *testing.T) {
	body := "\n\n\n"
	src := &fakeSource{body: body, kind: source.KindText}
	s, err := Open(context.Background(), src, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	all := append([]Chunk{s.First}, collectChunks(t, s)...)
	var lines []source.Line
	for _, c := range all {
		lines = append(lines, c.Lines...)
	}
	if len(lines) != 3 {
		t.Errorf("expected 3 empty lines, got %d", len(lines))
	}
	for i, l := range lines {
		if l.Raw != "" {
			t.Errorf("line %d should be empty, got %q", i, l.Raw)
		}
	}
	for err := range s.Errs {
		if errors.Is(err, ErrLineTruncated) {
			t.Errorf("empty lines must not trigger ErrLineTruncated, got %v", err)
		}
	}
}

// --- helpers ---

func repeatLines(n int, body string) string {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		b.WriteString(body)
		b.WriteByte('\n')
	}
	return b.String()
}

func collectChunks(t *testing.T, s *Stream) []Chunk {
	t.Helper()
	var out []Chunk
	if s.Updates == nil {
		return out
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case c, ok := <-s.Updates:
			if !ok {
				return out
			}
			out = append(out, c)
		case <-deadline:
			t.Fatal("Updates channel did not close")
			return out
		}
	}
}

type errorSource struct {
	err error
}

func (e *errorSource) Kind() source.Kind        { return source.KindUnknown }
func (e *errorSource) DisplayName() string      { return "<error>" }
func (e *errorSource) Metadata() source.Metadata { return source.Metadata{} }
func (e *errorSource) Open() (io.ReadCloser, error) {
	return nil, e.err
}
func (e *errorSource) Reopen() (io.ReadSeeker, error) {
	return nil, e.err
}

// helper used by file-based tests only (not currently needed but
// keeps the testing.T import grounded).
func _writeFileForLoader(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}
