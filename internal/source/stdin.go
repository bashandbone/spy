// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"bytes"
	"fmt"
	"io"
)

// stdinDisplayName is the literal footer label for stdin sessions. It
// is intentionally not a path so `filepath.Base` is a no-op (research
// R5; contracts/cli.md "Stdin behavior").
const stdinDisplayName = "<stdin>"

// stdinPeekBytes is how many bytes [StdinSource] inspects up front to
// classify the stream. Mirrors the 8 KiB window used by [detectKind]
// for files; the bytes are buffered and replayed via [io.MultiReader]
// on the first [Open] so the loader sees the full stream from byte 0.
const stdinPeekBytes = 8192

// StdinSource is the [Source] implementation backed by a non-seekable
// stream — typically `os.Stdin` when spy is part of a shell pipeline.
// Per research R5 the stream is read at most once and held only in the
// loader's in-memory ring buffer.
type StdinSource struct {
	reader io.Reader
	hint   string

	// detect-once state. peekedHead carries the first
	// [stdinPeekBytes] bytes of the stream; once detection has run
	// it's prepended (via [io.MultiReader]) on the next [Open] so the
	// loader sees the full stream from byte 0.
	detected   bool
	detectErr  error
	kind       Kind
	lexerName  string
	peekedHead []byte
	peekErr    error // io.EOF / io.ErrUnexpectedEOF — used to suppress double-peeks

	// consumed flips on the first successful [Open] to enforce the
	// "stdin is single-use" contract from T085.
	consumed bool
}

// NewStdinSource wires a stream + optional language hint into a
// [StdinSource]. Detection (kind + lexer name) is deferred until the
// first [StdinSource.Kind] / [StdinSource.Open] call so the constructor
// stays infallible for the FromArgs caller.
//
// `r` is normally `os.Stdin`; tests pass an `os.Pipe` read end or a
// `strings.Reader` to drive the path without a real fd. The hint
// matches `--lang` semantics: a Chroma lexer name (e.g., "go", "py")
// or empty for content-based detection.
func NewStdinSource(r io.Reader, hint string) *StdinSource {
	return &StdinSource{reader: r, hint: hint}
}

// Kind triggers a one-shot 8 KiB peek + [detectKind] on first call and
// returns the cached classification thereafter.
func (s *StdinSource) Kind() Kind {
	s.detectOnce()
	return s.kind
}

// DisplayName returns the literal "<stdin>" so footer/status code can
// route stdin sessions through the same DisplayName plumbing as files
// without special-casing the path-vs-label distinction.
func (s *StdinSource) DisplayName() string { return stdinDisplayName }

// Open returns a one-shot [io.ReadCloser] over the full stream — the
// peeked head bytes (consumed during detection) are replayed via
// [io.MultiReader] before the live tail of the underlying reader.
//
// The second call returns [ErrAlreadyConsumed]; stdin is a single-use
// resource and re-opening would give the loader a partial stream.
func (s *StdinSource) Open() (io.ReadCloser, error) {
	if s.consumed {
		return nil, fmt.Errorf("stdin: %w", ErrAlreadyConsumed)
	}
	if err := s.detectOnce(); err != nil {
		return nil, err
	}
	s.consumed = true
	combined := io.MultiReader(bytes.NewReader(s.peekedHead), s.reader)
	return io.NopCloser(combined), nil
}

// Reopen always returns [ErrNotSeekable] — stdin can't rewind. The
// loader's windowed mode handles this branch via the documented
// `WarnStdinNonSeekable` advisory.
func (s *StdinSource) Reopen() (io.ReadSeeker, error) {
	return nil, fmt.Errorf("stdin: %w", ErrNotSeekable)
}

// Metadata returns a snapshot suitable for the foundational footer.
// Path is the literal "<stdin>" label so the footer code can use it
// without a separate "is this a real path" check; Size is 0 because we
// don't know the stream length up front; LineCount is -1 until the
// loader finishes streaming.
func (s *StdinSource) Metadata() Metadata {
	s.detectOnce()
	return Metadata{
		Path:      stdinDisplayName,
		Size:      0,
		LineCount: -1,
		Language:  s.lexerName,
	}
}

// detectOnce reads up to [stdinPeekBytes] from the underlying reader
// the first time any caller asks for the stream's classification, runs
// [detectKind] over the buffered bytes, and caches the result so
// subsequent Kind / Open / Metadata calls are O(1).
//
// The peeked bytes are kept in `s.peekedHead` so the next [Open] can
// hand them back via [io.MultiReader] — without that replay the loader
// would see a stream that's missing its first 8 KiB.
func (s *StdinSource) detectOnce() error {
	if s.detected {
		return s.detectErr
	}
	s.detected = true
	if s.reader == nil {
		s.detectErr = fmt.Errorf("%w: stdin reader is nil", ErrNoInput)
		return s.detectErr
	}
	buf := make([]byte, stdinPeekBytes)
	n, err := io.ReadFull(s.reader, buf)
	switch {
	case err == nil, err == io.EOF, err == io.ErrUnexpectedEOF:
		// All three are expected — the stream may be shorter than the
		// peek window or close to it. detectKind handles the empty case.
	default:
		s.peekErr = err
		s.detectErr = fmt.Errorf("stdin: peek: %w", err)
		return s.detectErr
	}
	s.peekedHead = buf[:n]
	kind, lex, derr := detectKind(bytes.NewReader(s.peekedHead), s.hint)
	s.kind = kind
	s.lexerName = lex
	s.detectErr = derr
	return s.detectErr
}
