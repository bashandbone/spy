// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/chroma/v2"
	xterm "golang.org/x/term"
)

// Kind names the high-level category of a [Source]'s content. It drives
// renderer selection in [internal/render]; the per-language Chroma lexer
// is carried alongside on [Source] / [Metadata].
type Kind int

const (
	KindUnknown Kind = iota
	KindCode
	KindMarkdown
	KindText
	KindPDF
	KindImage
	KindBinary
)

// detectionPeekBytes is the up-front read window every detection
// path uses to classify a Source: 8 KiB chosen to capture shebangs,
// magic bytes, and Chroma's `Analyze` window in a single read.
// Shared by [detectKind] (file path) and [StdinSource.detectOnce]
// (stream path) so both stay in sync without a duplicate literal.
const detectionPeekBytes = 8192

// Sentinel errors. Callers identify error categories via [errors.Is].
// All package-internal errors wrap these so tests like
// `errors.Is(err, ErrNotFound)` resolve regardless of the call site.
var (
	ErrNoInput         = errors.New("no input provided")
	ErrBinary          = errors.New("binary content")
	ErrUnsupported     = errors.New("unsupported format")
	ErrNotFound        = errors.New("file not found")
	ErrPermission      = errors.New("permission denied")
	ErrNotSeekable     = errors.New("source does not support seeking")
	ErrAlreadyConsumed = errors.New("source already consumed")
	// ErrAmbiguousArgs covers mutually-exclusive positional
	// combinations from contracts/cli.md: `-` alongside a FILE
	// argument, or multiple FILE arguments. Maps to exit 2 in main.
	ErrAmbiguousArgs = errors.New("ambiguous arguments")
)

// Source is the producer-side abstraction for the byte stream a viewer
// session reads. FileSource may be opened many times (each call yields a
// fresh os.File); the StdinSource added in US5 may be opened only once.
type Source interface {
	Kind() Kind
	DisplayName() string
	// Open returns a reader for the source bytes. Callers must Close.
	Open() (io.ReadCloser, error)
	// Reopen returns a seekable reader for windowed-mode re-reads.
	// Returns ErrNotSeekable for non-seekable sources.
	Reopen() (io.ReadSeeker, error)
	Metadata() Metadata
}

// Metadata is the static description of a Source. LineCount is -1 until
// the loader has finished streaming; PageCount is non-zero only for PDFs.
type Metadata struct {
	Path      string
	Size      int64
	LineCount int64
	PageCount int
	Modified  time.Time
	Language  string
	Encoding  string
}

// Line is a single line of source content. Tokens is populated by the
// highlighter; Wrapped is a per-width wrap cache invalidated on resize.
// Defining Line here (instead of in `loader`) keeps the package DAG
// acyclic — see contracts/internal-apis.md `internal/source`.
type Line struct {
	Number  int64
	Raw     string
	Tokens  []Token
	Wrapped []string
}

// Token is the unit of styled output the renderer consumes. The Type
// uses Chroma's vocabulary so any future highlighter (Treesitter, plain
// regex) can produce them with a known semantics. Living in `source`
// avoids a `source -> highlight` import edge.
type Token struct {
	Type  chroma.TokenType
	Value string
}

// LineProvider is the read-side interface the search and renderer
// packages consume. Implemented by *loader.LineBuffer (implementation
// lands in T021); defining the interface here keeps consumers
// independent of loader's concrete type.
type LineProvider interface {
	Slice(start, end int64) []Line
	Total() int64
}

// FromArgs picks a Source from CLI arguments and stdin per the
// resolution table in contracts/cli.md. The `hint` is an optional
// language hint that short-circuits content-based detection (see
// contracts/cli.md `--lang`).
//
// Resolution rules:
//
//   - file argument present (single positional): FileSource (file
//     always wins; stdin is never read even when piped).
//   - "-" positional (single): StdinSource (forced — blocks on TTY
//     stdin until EOF/Ctrl-D, the documented behavior).
//   - no args + stdin is non-TTY: StdinSource (the `... | spy` shape).
//   - no args + stdin is a TTY (or nil): ErrNoInput (exit 2 from main).
//   - "-" alongside a FILE, or multiple FILEs: ErrAmbiguousArgs
//     (also exit 2). Per contracts/cli.md row "present yes — yes".
//
// `stdin` is normally `os.Stdin`; tests pass an `os.Pipe` read end to
// drive the non-TTY path without a real PTY.
func FromArgs(args []string, stdin *os.File, hint string) (Source, error) {
	if len(args) > 1 {
		// Two positionals are always wrong: either "-" + FILE
		// (mutually exclusive per contract) or multiple FILEs (the
		// synopsis only allows one).
		dashCount := 0
		for _, a := range args {
			if a == "-" {
				dashCount++
			}
		}
		switch {
		case dashCount > 0 && dashCount < len(args):
			return nil, fmt.Errorf("%w: '-' and FILE are mutually exclusive", ErrAmbiguousArgs)
		case dashCount > 1:
			return nil, fmt.Errorf("%w: '-' positional given more than once", ErrAmbiguousArgs)
		default:
			return nil, fmt.Errorf("%w: only one FILE may be given (got %d)", ErrAmbiguousArgs, len(args))
		}
	}
	if len(args) == 1 && args[0] != "-" {
		return newFileSourceWithHint(args[0], hint)
	}
	if len(args) == 1 && args[0] == "-" {
		// `-` always picks stdin, regardless of TTY status. A nil
		// stdin pointer still surfaces as ErrNoInput because there's
		// nothing to read.
		if stdin == nil {
			return nil, fmt.Errorf("%w: '-' positional given but no stdin", ErrNoInput)
		}
		return NewStdinSource(stdin, hint), nil
	}
	// No positional. Auto-pick stdin only when it's clearly a pipe.
	if stdin == nil {
		return nil, fmt.Errorf("%w: missing FILE; nothing on stdin", ErrNoInput)
	}
	if xterm.IsTerminal(int(stdin.Fd())) {
		return nil, fmt.Errorf("%w: missing FILE; stdin is a TTY", ErrNoInput)
	}
	return NewStdinSource(stdin, hint), nil
}

// classifyFSError translates the "raw" filesystem error into one of the
// sentinel errors so callers can categorize via [errors.Is].
func classifyFSError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("%w: %v", ErrPermission, err)
	}
	return err
}

// resolveSymlink resolves the final target of a path. Returns the same
// classification errors as the rest of the file paths so a broken
// symlink surfaces as ErrNotFound rather than the raw EvalSymlinks
// error wording.
func resolveSymlink(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", classifyFSError(err)
	}
	return resolved, nil
}
