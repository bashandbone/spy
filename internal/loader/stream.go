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
	scannerBufferBytes       = 64 * 1024
	scannerMaxBytes          = 16 * 1024 * 1024
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

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, scannerBufferBytes), scannerMaxBytes)

	updates := make(chan Chunk, cfg.UpdatesBuffer)
	errs := make(chan error, cfg.UpdatesBuffer+2)

	buf := NewLineBuffer(cfg.MaxResidentBytes, cfg.WindowSize, src)

	stream := &Stream{
		Updates: updates,
		Errs:    errs,
		Buffer:  buf,
	}
	buf.SetWarningSink(errs)

	// Read the first chunk synchronously.
	first, hitEOF, readErr := readChunk(scanner, 1, cfg.InitialChunkLines, cfg.MaxLineBytes, errs)
	first.EOF = hitEOF
	stream.First = first
	buf.Append(first.Lines)

	if readErr != nil {
		_ = rc.Close()
		errs <- fmt.Errorf("loader.Open: %w", readErr)
		close(updates)
		close(errs)
		return stream, nil
	}

	if hitEOF {
		_ = rc.Close()
		buf.MarkComplete(int64(len(first.Lines)))
		close(updates)
		close(errs)
		return stream, nil
	}

	// Continue streaming asynchronously. Producer blocks on bounded
	// `updates`; ctx cancellation interrupts both the scan and the send.
	go func() {
		defer rc.Close()
		defer close(updates)
		defer close(errs)
		next := first.StartLine + int64(len(first.Lines))
		for {
			if ctx.Err() != nil {
				return
			}
			c, eof, err := readChunk(scanner, next, defaultStreamingChunk, cfg.MaxLineBytes, errs)
			c.EOF = eof
			if len(c.Lines) > 0 {
				select {
				case <-ctx.Done():
					return
				case updates <- c:
				}
				buf.Append(c.Lines)
				next += int64(len(c.Lines))
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

// readChunk reads up to `n` lines from the scanner starting at line
// number `start`. Lines longer than `maxLineBytes` are truncated and a
// matching [ErrLineTruncated] is sent on `errs` (best-effort: drops the
// warning if `errs` is full to avoid backpressure on the warning path).
//
// Returns the populated chunk plus whether EOF was hit and any scanner
// error encountered.
func readChunk(scanner *bufio.Scanner, start int64, n int, maxLineBytes int64, errs chan<- error) (Chunk, bool, error) {
	c := Chunk{StartLine: start, Lines: make([]source.Line, 0, n)}
	for len(c.Lines) < n {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
				return c, false, err
			}
			return c, true, nil
		}
		raw := scanner.Bytes()
		if int64(len(raw)) > maxLineBytes {
			lineNo := start + int64(len(c.Lines))
			raw = raw[:maxLineBytes]
			select {
			case errs <- fmt.Errorf("%w: line %d", ErrLineTruncated, lineNo):
			default:
			}
		}
		// Copy the bytes so the scanner can reuse its internal buffer
		// safely. Without a copy, line.Raw aliases the scanner's buffer
		// and contents change on the next Scan().
		out := make([]byte, len(raw))
		copy(out, raw)
		c.Lines = append(c.Lines, source.Line{
			Number: start + int64(len(c.Lines)),
			Raw:    string(out),
		})
	}
	return c, false, nil
}
