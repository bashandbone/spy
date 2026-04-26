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

// rejectSpecialMode returns a non-nil error when the supplied [os.FileMode]
// describes a kind we will never read as a popup-reader source — FIFOs,
// sockets, character/block devices, or directories. The error is
// classified as [ErrUnsupported] and names the rejected category so the
// CLI can surface a clear message instead of returning a confusing read
// error later in the pipeline. Pseudo-fs paths (/proc, /sys) remain a
// documented FOLLOWUP; this function only covers the mode-bit cases.
func rejectSpecialMode(displayPath string, mode os.FileMode) error {
	switch {
	case mode.IsDir():
		return fmt.Errorf("%w: %s is a directory", ErrUnsupported, displayPath)
	case mode&os.ModeNamedPipe != 0:
		return fmt.Errorf("%w: %s is a named pipe (FIFO)", ErrUnsupported, displayPath)
	case mode&os.ModeSocket != 0:
		return fmt.Errorf("%w: %s is a socket", ErrUnsupported, displayPath)
	case mode&os.ModeDevice != 0:
		// os.ModeDevice covers both character and block devices; we treat
		// them identically because neither is a regular byte stream we
		// can reason about (Copilot review acceptance M1).
		return fmt.Errorf("%w: %s is a device file", ErrUnsupported, displayPath)
	}
	return nil
}

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
	// Lstat the resolved path so we can reject FIFOs/sockets/devices
	// BEFORE attempting open(2) — a Unix-domain socket inode returns
	// ENXIO from open(2) and a regular FIFO would block (or spin on
	// O_NONBLOCK), neither of which surfaces a useful error to the
	// user. After symlink resolution lstat() reports the target's
	// mode, so this also catches a regular path that happens to
	// resolve to a special inode (Copilot review acceptance M1).
	preInfo, err := os.Lstat(resolved)
	if err != nil {
		return nil, classifyFSError(err)
	}
	if rejErr := rejectSpecialMode(path, preInfo.Mode()); rejErr != nil {
		return nil, rejErr
	}
	// Open with O_NOFOLLOW (Unix) so a symlink swap between EvalSymlinks
	// and Open() can't redirect us to a different file (CVE-class TOCTOU
	// — Copilot review acceptance M2). Stat the open fd rather than
	// re-stat()ing the path so the mode we validate matches the inode
	// we'll actually read from.
	f, err := openNoFollow(resolved)
	if err != nil {
		return nil, classifyFSError(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, classifyFSError(err)
	}
	if rejErr := rejectSpecialMode(path, info.Mode()); rejErr != nil {
		return nil, rejErr
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
//
// The file is opened with O_NOFOLLOW (Unix) so a symlink swapped in
// between FileSource construction and this call cannot redirect us
// onto a different inode. The freshly-opened fd is also re-stat()ed so
// a path that turned into a FIFO/socket/device after construction is
// still rejected (Copilot review acceptance M2).
func (s *FileSource) Open() (io.ReadCloser, error) {
	if err := s.detectOnce(); err != nil {
		return nil, err
	}
	return s.openValidated()
}

// Reopen returns a seekable reader so the loader's windowed mode can
// re-read previously-evicted line ranges. Caller is responsible for
// closing — the returned [io.ReadSeeker] is also an [io.Closer] when
// the source is a real file (so callers commonly type-assert).
//
// Same TOCTOU mitigation as [FileSource.Open]: O_NOFOLLOW on Unix plus
// fd-side mode validation on every reopen.
func (s *FileSource) Reopen() (io.ReadSeeker, error) {
	if err := s.detectOnce(); err != nil {
		return nil, err
	}
	return s.openValidated()
}

// openValidated opens s.path with O_NOFOLLOW (Unix) and verifies the
// fd's current mode bits before returning it. Returns a classified
// [ErrUnsupported] if the inode has flipped to a special file in the
// window since FileSource construction.
func (s *FileSource) openValidated() (*os.File, error) {
	f, err := openNoFollow(s.path)
	if err != nil {
		return nil, classifyFSError(err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, classifyFSError(err)
	}
	if rejErr := rejectSpecialMode(s.displayName, info.Mode()); rejErr != nil {
		_ = f.Close()
		return nil, rejErr
	}
	return f, nil
}

// Redetect clears the cached detection state so the next [Open] /
// [Kind] / [Metadata] call re-runs [detectKind] against the current
// file contents. Used by `ActionReload` so a file that was swapped in
// place since the original Open is correctly re-classified — without
// this, a "reload" would silently render the new bytes through the
// stale lexer/Kind (Copilot review acceptance M2).
func (s *FileSource) Redetect() {
	s.detected = false
	s.detectErr = nil
	s.kind = KindUnknown
	s.lexerName = ""
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
	f, err := openNoFollow(s.path)
	if err != nil {
		s.detectErr = classifyFSError(err)
		return s.detectErr
	}
	defer f.Close()
	info, statErr := f.Stat()
	if statErr != nil {
		s.detectErr = classifyFSError(statErr)
		return s.detectErr
	}
	if rejErr := rejectSpecialMode(s.displayName, info.Mode()); rejErr != nil {
		s.detectErr = rejErr
		return rejErr
	}
	kind, lex, err := detectKind(f, hint)
	s.kind = kind
	s.lexerName = lex
	s.detectErr = err
	return err
}
