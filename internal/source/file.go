// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileSource is the [Source] implementation backed by a regular file on
// disk. Construction stat()s the path so missing/permission/symlink
// errors surface immediately; the actual byte detection happens lazily
// on the first [Open] call.
type FileSource struct {
	path        string
	displayName string
	hint        string
	size        int64
	modified    int64 // unix nanos
	kind        Kind
	lexerName   string
	detectErr   error
	detected    bool
}

// NewFileSource opens the path metadata-side: it stat()s the path,
// resolves symlinks, classifies the FS error category, but does not yet
// read the content. Detection runs on first [FileSource.Open].
func NewFileSource(path string) (*FileSource, error) {
	return newFileSourceWithHint(path, "")
}

func newFileSourceWithHint(path, hint string) (*FileSource, error) {
	resolved, err := resolveSymlink(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, classifyFSError(err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory", ErrUnsupported, path)
	}
	fs := &FileSource{
		path:        resolved,
		displayName: filepath.Base(path),
		hint:        hint,
		size:        info.Size(),
		modified:    info.ModTime().UnixNano(),
		kind:        KindUnknown,
	}
	return fs, nil
}

// Kind returns the cached [Kind]; it triggers a lazy [detectKind] on
// first call. Detection errors are surfaced via [Open] / [Reopen] so
// constructors stay infallible for already-present files.
func (s *FileSource) Kind() Kind {
	s.detectOnce()
	return s.kind
}

// DisplayName returns the basename used in status bars and footers.
func (s *FileSource) DisplayName() string { return s.displayName }

// Open returns a fresh [io.ReadCloser] each call, plus any deferred
// detection error encountered the first time content was inspected.
func (s *FileSource) Open() (io.ReadCloser, error) {
	if err := s.detectOnce(); err != nil {
		return nil, err
	}
	f, err := os.Open(s.path)
	if err != nil {
		return nil, classifyFSError(err)
	}
	return f, nil
}

// Reopen returns a seekable reader so the loader's windowed mode can
// re-read previously-evicted line ranges. Caller is responsible for
// closing — the returned [io.ReadSeeker] is also an [io.Closer] when
// the source is a real file (so callers commonly type-assert).
func (s *FileSource) Reopen() (io.ReadSeeker, error) {
	if err := s.detectOnce(); err != nil {
		return nil, err
	}
	f, err := os.Open(s.path)
	if err != nil {
		return nil, classifyFSError(err)
	}
	return f, nil
}

// Metadata returns a snapshot of the file's static description. The
// LineCount field is -1 until the loader has finished streaming; the
// loader is responsible for updating it via the metaUpdated message in
// US6.
func (s *FileSource) Metadata() Metadata {
	s.detectOnce()
	return Metadata{
		Path:      s.path,
		Size:      s.size,
		LineCount: -1,
		Language:  s.lexerName,
		Modified:  asTime(s.modified),
	}
}

func (s *FileSource) detectOnce() error {
	if s.detected {
		return s.detectErr
	}
	s.detected = true
	// Use the hint plus the displayName so detection by extension picks
	// up the original path — the hint takes priority per detectKind.
	hint := s.hint
	if hint == "" {
		hint = s.displayName
	}
	f, err := os.Open(s.path)
	if err != nil {
		s.detectErr = classifyFSError(err)
		return s.detectErr
	}
	defer f.Close()
	kind, lex, err := detectKind(f, hint)
	s.kind = kind
	s.lexerName = lex
	s.detectErr = err
	return err
}
