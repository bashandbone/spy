// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package loader

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/knitli/spy/internal/source"
)

// Default configuration values used when [Config] fields are zero.
const (
	defaultInitialChunkLines = 80
	defaultUpdatesBuffer     = 4
	defaultMaxLineBytes      = 100 * 1024 // 100 KiB
	defaultWindowSize        = 4096
	defaultStreamingChunk    = 256 // lines per Updates send after the first
	readerBufferBytes        = 64 * 1024
)

// Config tunes [Open]. Zero fields use the documented defaults.
type Config struct {
	MaxResidentBytes  int64 // beyond this, switch to windowed mode
	WindowSize        int   // resident window size in lines (windowed mode)
	InitialChunkLines int   // lines synchronously read into Stream.First
	UpdatesBuffer     int   // capacity of Stream.Updates; default 4
	MaxLineBytes      int64 // per-line cap; default 102400 (100 KiB)
}

func (c Config) withDefaults() Config {
	if c.InitialChunkLines <= 0 {
		c.InitialChunkLines = defaultInitialChunkLines
	}
	if c.UpdatesBuffer <= 0 {
		c.UpdatesBuffer = defaultUpdatesBuffer
	}
	if c.MaxLineBytes <= 0 {
		c.MaxLineBytes = defaultMaxLineBytes
	}
	if c.WindowSize <= 0 {
		c.WindowSize = defaultWindowSize
	}
	return c
}

// Chunk is a contiguous run of lines emitted by the loader. Lines are
// numbered 1-based; StartLine is the number of Lines[0]. EOF is true on
// the final chunk emitted by a successful read.
type Chunk struct {
	Lines     []source.Line
	StartLine int64
	EOF       bool
}

// Stream is the read-side handle returned by [Open]. The First chunk is
// available immediately; Updates carries continuation chunks (closed on
// EOF or context cancel); Errs carries warnings and fatal errors (closed
// after Updates).
type Stream struct {
	First   Chunk
	Updates <-chan Chunk
	Errs    <-chan error
	Buffer  *LineBuffer // resident lines + windowing state
}

// Sentinel errors emitted on Stream.Errs.
var (
	// ErrLineTruncated indicates a single line exceeded Config.MaxLineBytes
	// and was clipped at the cap. Wrapped with the line number for context.
	ErrLineTruncated = errors.New("line truncated")
	// ErrStdinNonSeekable indicates the loader entered windowed mode for a
	// non-seekable source (typically stdin); subsequent navigation past
	// the resident window cannot re-seek and the user is warned.
	ErrStdinNonSeekable = errors.New("stdin is non-seekable; scroll-back disabled past resident window")
)

// Open begins streaming `src` under the supplied [Config]. The first
// chunk (≥ cfg.InitialChunkLines or until EOF) is read synchronously
// before Open returns so the first viewer frame paints immediately;
// further chunks stream on Stream.Updates. The Updates channel is
// bounded — producer blocks when full so the buffer never holds more
// than UpdatesBuffer chunks worth of Line.Raw bytes.
func Open(ctx context.Context, src source.Source, cfg Config) (*Stream, error) {
	cfg = cfg.withDefaults()

	rc, err := src.Open()
	if err != nil {
		return nil, fmt.Errorf("loader.Open: %w", err)
	}

	reader := bufio.NewReaderSize(rc, readerBufferBytes)

	updates := make(chan Chunk, cfg.UpdatesBuffer)
	errs := make(chan error, cfg.UpdatesBuffer+2)

	buf := newLineBuffer(cfg.MaxResidentBytes, cfg.WindowSize, cfg.MaxLineBytes, src)

	stream := &Stream{
		Updates: updates,
		Errs:    errs,
		Buffer:  buf,
	}
	buf.SetWarningSink(errs)

	// Read the first chunk synchronously.
	first, hitEOF, readErr := readChunk(reader, 1, cfg.InitialChunkLines, cfg.MaxLineBytes, errs)
	first.EOF = hitEOF
	stream.First = first
	buf.Append(first.Lines)

	if readErr != nil {
		_ = rc.Close()
		// Non-blocking send so a buffer that's already full of
		// truncation warnings from readChunk doesn't deadlock Open()
		// (Copilot review PR#7 #25).
		select {
		case errs <- fmt.Errorf("loader.Open: %w", readErr):
		default:
		}
		// Mark the warning sink closed BEFORE closing the underlying
		// channel so any in-flight Slice() that's about to send a
		// windowed-mode warning observes the flag and skips the send
		// (LOW-4). The flag is set under b.mu, the same mutex that
		// guards sendWarning's call site.
		buf.CloseWarningSink()
		close(updates)
		close(errs)
		return stream, nil
	}

	if hitEOF {
		_ = rc.Close()
		buf.MarkComplete(int64(len(first.Lines)))
		buf.CloseWarningSink()
		close(updates)
		close(errs)
		return stream, nil
	}

	// Continue streaming asynchronously. Producer blocks on bounded
	// `updates`; ctx cancellation interrupts both the scan and the send.
	//
	// Defer order is load-bearing: defers run LIFO, so the source
	// order below produces the runtime sequence:
	//   1. buf.CloseWarningSink()  — flips the buffer's flag so
	//                                concurrent Slice() calls observe
	//                                "sink closed" and skip the send
	//                                path (LOW-4)
	//   2. close(errs)             — signals errs consumers
	//   3. close(updates)          — signals updates consumers
	//   4. rc.Close()              — releases the source FD
	//
	// CloseWarningSink MUST run BEFORE close(errs) so the flag is
	// visible to any racing Slice() call by the time close happens —
	// otherwise the recover() in sendWarning is the only thing
	// preventing a send-on-closed panic.
	go func() {
		defer rc.Close()
		defer close(updates)
		defer close(errs)
		defer buf.CloseWarningSink()
		next := first.StartLine + int64(len(first.Lines))
		for {
			if ctx.Err() != nil {
				return
			}
			c, eof, err := readChunk(reader, next, defaultStreamingChunk, cfg.MaxLineBytes, errs)
			c.EOF = eof
			if len(c.Lines) > 0 {
				// Append BEFORE send so the UI consumer never sees an
				// EOF chunk on Updates while the corresponding lines
				// are still missing from Stream.Buffer (Copilot review
				// PR#7 #3).
				buf.Append(c.Lines)
				next += int64(len(c.Lines))
				select {
				case <-ctx.Done():
					return
				case updates <- c:
				}
			}
			if err != nil {
				select {
				case errs <- fmt.Errorf("loader.read: %w", err):
				default:
				}
				return
			}
			if eof {
				buf.MarkComplete(next - 1)
				return
			}
		}
	}()

	return stream, nil
}

// readChunk reads up to `n` lines from the reader starting at line
// number `start`. Lines longer than `maxLineBytes` are truncated and a
// matching [ErrLineTruncated] is sent on `errs` (best-effort: drops the
// warning if `errs` is full to avoid backpressure on the warning path).
//
// Uses [bufio.Reader] (not [bufio.Scanner]) so genuinely enormous lines
// — multi-GB log dumps, minified JSON / JS bundles past the scanner's
// 16 MiB hard limit — get truncated cleanly instead of erroring the
// whole stream (Copilot review PR#7 #5).
//
// Returns the populated chunk plus whether EOF was hit and any read
// error encountered.
func readChunk(r *bufio.Reader, start int64, n int, maxLineBytes int64, errs chan<- error) (Chunk, bool, error) {
	c := Chunk{StartLine: start, Lines: make([]source.Line, 0, n)}
	for len(c.Lines) < n {
		raw, truncated, err := readLine(r, maxLineBytes)
		if errors.Is(err, io.EOF) {
			if len(raw) > 0 {
				c.Lines = append(c.Lines, source.Line{
					Number: start + int64(len(c.Lines)),
					Raw:    string(raw),
				})
			}
			return c, true, nil
		}
		if err != nil {
			return c, false, err
		}
		if truncated {
			lineNo := start + int64(len(c.Lines))
			select {
			case errs <- fmt.Errorf("%w: line %d", ErrLineTruncated, lineNo):
			default:
			}
		}
		c.Lines = append(c.Lines, source.Line{
			Number: start + int64(len(c.Lines)),
			Raw:    string(raw),
		})
	}
	return c, false, nil
}

// readLine reads bytes from `r` until a newline or EOF. The first
// `maxLineBytes` bytes are kept; any remainder is silently consumed
// (so the next call still starts at the right offset) and the
// `truncated` return is set so the caller can emit ErrLineTruncated.
//
// The trailing '\n' (and any preceding '\r') is stripped from the
// returned slice.
func readLine(r *bufio.Reader, maxLineBytes int64) ([]byte, bool, error) {
	var buf []byte
	truncated := false
	gotAny := false
	for {
		chunk, err := r.ReadSlice('\n')
		// A non-nil chunk plus bufio.ErrBufferFull means we read up to
		// the internal buffer boundary without finding '\n'; keep going
		// and accumulate into `buf`.
		if len(chunk) > 0 {
			gotAny = true
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			buf, truncated = appendBounded(buf, chunk, maxLineBytes, truncated)
			continue
		}
		if err != nil {
			// EOF or any other terminal error — return what we have.
			if len(chunk) > 0 {
				// Trim a trailing newline if the input happened to end
				// with one despite the error path (rare).
				chunk = trimTrailingNewline(chunk)
				buf, truncated = appendBounded(buf, chunk, maxLineBytes, truncated)
			}
			if !gotAny {
				return nil, false, err
			}
			// EOF with content read: surface as no-error so the caller
			// records the line, then EOF on the next call.
			if errors.Is(err, io.EOF) {
				return buf, truncated, io.EOF
			}
			return buf, truncated, err
		}
		// Got a complete line ending in '\n'. Strip the newline (and
		// preceding '\r' if any) before returning.
		chunk = trimTrailingNewline(chunk)
		buf, truncated = appendBounded(buf, chunk, maxLineBytes, truncated)
		return buf, truncated, nil
	}
}

// appendBounded copies bytes from `src` into `buf` up to maxBytes; any
// overflow is dropped and `truncated` is set. Returns the (possibly
// re-allocated) buf and updated truncated flag.
func appendBounded(buf, src []byte, maxBytes int64, truncated bool) ([]byte, bool) {
	if int64(len(buf)) >= maxBytes {
		if len(src) > 0 {
			truncated = true
		}
		return buf, truncated
	}
	rem := maxBytes - int64(len(buf))
	if int64(len(src)) > rem {
		buf = append(buf, src[:rem]...)
		truncated = true
	} else {
		buf = append(buf, src...)
	}
	return buf, truncated
}

// trimTrailingNewline strips a single trailing '\n' (and optionally a
// preceding '\r') from `b`. Bytes past the trim are gone — this is
// safe because [bufio.Reader.ReadSlice] returns a slice into its own
// buffer that we then copy into `buf` via [appendBounded].
func trimTrailingNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	if len(b) > 0 && b[len(b)-1] == '\r' {
		b = b[:len(b)-1]
	}
	return b
}
