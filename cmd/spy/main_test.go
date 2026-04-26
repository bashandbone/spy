// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// Phase 2 cmd/spy mostly hosts runtime wiring (term.Detect, tea.Program
// .Run, etc.) which can't be tested without a PTY. These smoke tests
// cover the testable parts: the early exits for --help / --version and
// the documented usage-error path for unknown flags. The full
// integration coverage lands with the PTY harness in Phase 9.

func TestRun_VersionExitsZero(t *testing.T) {
	if got := run([]string{"--version"}, nil); got != exitOK {
		t.Errorf("--version: got exit %d want %d", got, exitOK)
	}
}

func TestRun_HelpExitsZero(t *testing.T) {
	if got := run([]string{"--help"}, nil); got != exitOK {
		t.Errorf("--help: got exit %d want %d", got, exitOK)
	}
}

func TestRun_UnknownFlagExitsUsage(t *testing.T) {
	if got := run([]string{"--mystery"}, nil); got != exitUsageError {
		t.Errorf("unknown flag: got exit %d want %d", got, exitUsageError)
	}
}

func TestRun_ConflictingConfigFlagsExitsUsage(t *testing.T) {
	if got := run([]string{"--config", "/x", "--no-config"}, nil); got != exitUsageError {
		t.Errorf("conflicting flags: got exit %d want %d", got, exitUsageError)
	}
}

func TestRun_NoInputExitsUsage(t *testing.T) {
	// No FILE arg + go-test runs with stdin/stdout typically not a TTY,
	// but FromArgs sees no args and returns ErrNoInput — exit 2.
	got := run([]string{"--no-config"}, nil)
	if got != exitUsageError {
		t.Errorf("no input: got exit %d want %d", got, exitUsageError)
	}
}

func TestRun_MissingFileExitsIO(t *testing.T) {
	got := run([]string{"--no-config", filepath.Join(t.TempDir(), "nope.txt")}, nil)
	if got != exitIOError {
		t.Errorf("missing file: got exit %d want %d", got, exitIOError)
	}
}

func TestRun_BinaryFileExitsUnsupported(t *testing.T) {
	p := filepath.Join(t.TempDir(), "blob.bin")
	body := make([]byte, 9000)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := run([]string{"--no-config", p}, nil); got != exitUnsupported {
		t.Errorf("binary file: got exit %d want %d", got, exitUnsupported)
	}
}

func TestRun_DirectoryExitsUnsupported(t *testing.T) {
	dir := t.TempDir()
	if got := run([]string{"--no-config", dir}, nil); got != exitUnsupported {
		t.Errorf("directory: got exit %d want %d", got, exitUnsupported)
	}
}

func TestRun_PermissionDeniedExitsIO(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	p := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if got := run([]string{"--no-config", p}, nil); got != exitIOError {
		t.Errorf("permission denied: got exit %d want %d", got, exitIOError)
	}
}

func TestRun_ExplicitMissingConfigExitsUsage(t *testing.T) {
	// --config <missing> is a hard error per contracts/cli.md
	// "Discovery rules" #1 — exit 2.
	got := run([]string{"--config", filepath.Join(t.TempDir(), "no.toml")}, nil)
	if got != exitUsageError {
		t.Errorf("missing --config: got exit %d want %d", got, exitUsageError)
	}
}

// TestC4_StderrSanitizesHostileFilename pins the acceptance-review C4
// contract that user-controlled file paths reach stderr only after
// passing through render.Neutralize. We construct a path containing
// the OSC 2 "set window title" payload, drive run() through the
// missing-file path, and assert stderr carries no `\x1b` / `\x9b`
// bytes.
func TestC4_StderrSanitizesHostileFilename(t *testing.T) {
	hostilePath := "/tmp/evil\x1b]2;injected\x07.txt"

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		_ = r.Close()
	})

	// Reader goroutine drains until EOF so we capture the full
	// stderr stream — the prior fixed 4 KiB buffer + io.ReadFull
	// would silently truncate longer error chains and could leak
	// the FD if the reader blocked waiting for more bytes than
	// the writer ever produced.
	done := make(chan []byte, 1)
	go func() {
		body, _ := io.ReadAll(r)
		done <- body
	}()

	got := run([]string{"--no-config", hostilePath}, nil)
	w.Close() // signal EOF to the reader so io.ReadAll returns

	// Exit code: the path won't resolve so we expect either ErrNotFound
	// (exit 3) or an unsupported-format mapping. Either is fine for
	// this assertion — we only care about the stderr body.
	if got != exitIOError && got != exitUnsupported {
		t.Logf("note: hostile-path run exited %d (acceptable: 3 or 4)", got)
	}

	stderr := <-done
	// Use IndexByte directly to scan for raw ESC (0x1b) / CSI (0x9b)
	// bytes — bytes.ContainsAny decodes the chars argument as runes,
	// and 0x9b on its own is invalid UTF-8 that decodes as U+FFFD,
	// so a stderr line containing a literal U+FFFD would
	// false-positive the ContainsAny scan.
	if bytes.IndexByte(stderr, 0x1b) >= 0 || bytes.IndexByte(stderr, 0x9b) >= 0 {
		t.Errorf("stderr leaked ESC / 8-bit-CSI from hostile filename:\n  %q", stderr)
	}
}

func TestRun_BadConfigSurfacesWarnings(t *testing.T) {
	// Soft warnings (unknown key, type mismatch) write to stderr but
	// don't abort. We pair the bad config with a missing file so run()
	// still exits with exitIOError after surfacing the warning.
	p := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(p, []byte(`unknown_key = 42`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := run([]string{"--config", p, filepath.Join(t.TempDir(), "missing.txt")}, nil)
	if got != exitIOError {
		t.Errorf("bad config + missing file: got exit %d want %d", got, exitIOError)
	}
}

func TestRun_NoColorFlag(t *testing.T) {
	// --no-color sets cfg.NoColor = true; pair with a missing file so
	// the run exits before tea.Program starts.
	got := run([]string{"--no-config", "--no-color", filepath.Join(t.TempDir(), "missing.txt")}, nil)
	if got != exitIOError {
		t.Errorf("--no-color path: got exit %d want %d", got, exitIOError)
	}
}

func TestRun_NoTTYDegenerateCats(t *testing.T) {
	// In `go test`, stdout is rarely a TTY, so a successful path through
	// source/loader degenerate-cats the file content to stdout and
	// exits 0 per contracts/cli.md (Copilot review PR#7 #29).
	p := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := run([]string{"--no-config", p}, nil)
	if got != exitOK {
		t.Errorf("non-TTY stdout with file arg: got exit %d want %d", got, exitOK)
	}
}

func TestRunDegenerate_WritesContentVerbatim(t *testing.T) {
	// runDegenerate copies the source's bytes verbatim — no rendering,
	// no line numbers, no escape sequences. Capture os.Stdout, invoke
	// runDegenerate, and confirm the bytes round-trip (Copilot review
	// PR#7 #29).
	p := filepath.Join(t.TempDir(), "verbatim.txt")
	want := "alpha\nbeta\ngamma\n"
	if err := os.WriteFile(p, []byte(want), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := source.FromArgs([]string{p}, nil, "")
	if err != nil {
		t.Fatalf("FromArgs: %v", err)
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		done <- buf[:n]
	}()

	if got := runDegenerate(src); got != exitOK {
		t.Errorf("runDegenerate: got exit %d want 0", got)
	}
	w.Close()
	got := string(<-done)
	if got != want {
		t.Errorf("runDegenerate output: got %q want %q", got, want)
	}
}

func TestApplyGraphicsOverride(t *testing.T) {
	detected := term.GraphicsKitty
	// "" / "auto" leaves the auto-detected protocol alone.
	if got := applyGraphicsOverride(detected, ""); got != detected {
		t.Errorf("empty override: got %v want detected", got)
	}
	if got := applyGraphicsOverride(detected, "auto"); got != detected {
		t.Errorf("auto override: got %v want detected", got)
	}
	cases := map[string]term.Graphics{
		"none":   term.GraphicsNone,
		"kitty":  term.GraphicsKitty,
		"iterm":  term.GraphicsITerm2,
		"iterm2": term.GraphicsITerm2,
		"sixel":  term.GraphicsSixel,
	}
	for in, want := range cases {
		if got := applyGraphicsOverride(detected, in); got != want {
			t.Errorf("override %q: got %v want %v", in, got, want)
		}
	}
	if got := applyGraphicsOverride(detected, "garbage"); got != detected {
		t.Errorf("unknown override: got %v want detected", got)
	}
}

func TestWantsAutoTheme(t *testing.T) {
	cases := map[string]bool{
		"":         true,
		"auto":     true,
		"AUTO":     true, // case-insensitive
		"  auto  ": true, // trim leading/trailing whitespace
		"dark":     false,
		"light":    false,
		"monokai":  false,
		"github":   false,
		"garbage":  false, // unknown styles still bypass — the renderer falls back to dark
	}
	for in, want := range cases {
		if got := wantsAutoTheme(in); got != want {
			t.Errorf("wantsAutoTheme(%q): got %v want %v", in, got, want)
		}
	}
}

func TestNewHighlighter_KnownStyle(t *testing.T) {
	caps := term.Capabilities{ColorDepth: term.ColorANSI256}
	theme := render.ThemeDark()
	h := newHighlighter(theme, caps, 1024)
	if h == nil {
		t.Fatal("newHighlighter returned nil")
	}
	if h.Style() == nil {
		t.Errorf("Style() should be non-nil for a known chroma style")
	}
	if h.Cap() != 1024 {
		t.Errorf("Cap: got %d want 1024", h.Cap())
	}
}

func TestNewHighlighter_UnknownStyleStillUsable(t *testing.T) {
	// chroma's styles.Get falls back to a non-nil Fallback style for
	// any unknown name; newHighlighter must therefore always produce a
	// usable Highlighter (Style() non-nil) even when --theme is
	// mistyped (Copilot review PR#7 follow-up).
	caps := term.Capabilities{ColorDepth: term.ColorANSI256}
	theme := render.ThemeDark()
	theme.ChromaStyle = "does-not-exist"
	h := newHighlighter(theme, caps, 0)
	if h == nil {
		t.Fatal("newHighlighter returned nil")
	}
	if h.Style() == nil {
		t.Errorf("Style() should fall back to a real style for unknown name")
	}
}

// TestFlagBoolPtr_NilWhenUnset is the smoke test for the LOW-3 fix:
// the historical boolPtr(false) → nil shortcut was replaced by
// flagBoolPtr, which keys off ParsedFlags.FlagWasSet so explicit
// `--vim=false` propagates as &false (not nil). The exhaustive
// behavior matrix lives in TestFlagBoolPtr_DistinguishesUnsetFromExplicitFalse
// (see flags_test.go).
func TestFlagBoolPtr_NilWhenUnset(t *testing.T) {
	pf := &ParsedFlags{}
	if got := flagBoolPtr(pf, "vim", false); got != nil {
		t.Errorf("flagBoolPtr(no-set, false) = &%v, want nil", *got)
	}
	if got := flagBoolPtr(pf, "vim", true); got != nil {
		t.Errorf("flagBoolPtr(no-set, true) = &%v, want nil", *got)
	}
}

func TestRun_AmbiguousArgsExitsUsage(t *testing.T) {
	// Copilot review PR#12 #1: contracts/cli.md row "present yes — yes"
	// is a usage error — `-` alongside a FILE positional is rejected at
	// the source layer and surfaced as exit 2.
	p := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cases := [][]string{
		{"--no-config", p, "-"},
		{"--no-config", "-", p},
		{"--no-config", p, p},
	}
	for _, args := range cases {
		if got := run(args, nil); got != exitUsageError {
			t.Errorf("run(%v): got exit %d want %d", args, got, exitUsageError)
		}
	}
}

func TestRun_StdinPipeDegenerateCats(t *testing.T) {
	// US5: no FILE, non-TTY stdin (pipe) → run() picks StdinSource and,
	// because go-test stdout is also a pipe, falls through to the
	// degenerate-cat path. Content is copied verbatim to stdout, exit 0.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	want := "hello via stdin\n"
	if _, err := w.WriteString(want); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("pipe close: %v", err)
	}
	if got := run([]string{"--no-config"}, r); got != exitOK {
		t.Errorf("stdin pipe: got exit %d want %d", got, exitOK)
	}
}

func TestRun_StdinTTYWithoutFileExitsUsage(t *testing.T) {
	// US5: nil stdin pointer mirrors the "no FILE and stdin is not
	// available" case; run() must surface ErrNoInput → exit 2.
	// Capture stderr to assert that usage is printed alongside the
	// error line (Copilot review PR#12 round-3 #8 — contracts/cli.md
	// row "absent no yes yes" requires "exit 2 (usage printed)").
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	// Drain to EOF in the goroutine. A single Read returns after the
	// first chunk arrives (the error line) — under -race scheduling
	// that's before WriteHelp's larger payload reaches the pipe — so
	// io.ReadAll is required to capture the full usage block.
	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()

	got := run([]string{"--no-config"}, nil)
	w.Close()
	stderr := string(<-done)
	if got != exitUsageError {
		t.Errorf("nil stdin + no FILE: got exit %d want %d", got, exitUsageError)
	}
	if !strings.Contains(stderr, "spy: no input") {
		t.Errorf("stderr missing error line; got %q", stderr)
	}
	if !strings.Contains(stderr, "Usage: spy") {
		t.Errorf("stderr missing usage block; got %q", stderr)
	}
}

func TestRun_VimFlagDegenerateCat(t *testing.T) {
	// --vim flag should parse cleanly and the non-TTY path should still
	// degenerate-cat the file (the keymap only matters on a TTY). Pair
	// with a real file so the run reaches the non-TTY branch.
	p := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(p, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := run([]string{"--no-config", "--vim", p}, nil); got != exitOK {
		t.Errorf("--vim: got exit %d want %d", got, exitOK)
	}
}
