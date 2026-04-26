// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"runtime"
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
	if got := run([]string{"--version"}); got != exitOK {
		t.Errorf("--version: got exit %d want %d", got, exitOK)
	}
}

func TestRun_HelpExitsZero(t *testing.T) {
	if got := run([]string{"--help"}); got != exitOK {
		t.Errorf("--help: got exit %d want %d", got, exitOK)
	}
}

func TestRun_UnknownFlagExitsUsage(t *testing.T) {
	if got := run([]string{"--mystery"}); got != exitUsageError {
		t.Errorf("unknown flag: got exit %d want %d", got, exitUsageError)
	}
}

func TestRun_ConflictingConfigFlagsExitsUsage(t *testing.T) {
	if got := run([]string{"--config", "/x", "--no-config"}); got != exitUsageError {
		t.Errorf("conflicting flags: got exit %d want %d", got, exitUsageError)
	}
}

func TestRun_NoInputExitsUsage(t *testing.T) {
	// No FILE arg + go-test runs with stdin/stdout typically not a TTY,
	// but FromArgs sees no args and returns ErrNoInput — exit 2.
	got := run([]string{"--no-config"})
	if got != exitUsageError {
		t.Errorf("no input: got exit %d want %d", got, exitUsageError)
	}
}

func TestRun_MissingFileExitsIO(t *testing.T) {
	got := run([]string{"--no-config", filepath.Join(t.TempDir(), "nope.txt")})
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
	if got := run([]string{"--no-config", p}); got != exitUnsupported {
		t.Errorf("binary file: got exit %d want %d", got, exitUnsupported)
	}
}

func TestRun_DirectoryExitsUnsupported(t *testing.T) {
	dir := t.TempDir()
	if got := run([]string{"--no-config", dir}); got != exitUnsupported {
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
	if got := run([]string{"--no-config", p}); got != exitIOError {
		t.Errorf("permission denied: got exit %d want %d", got, exitIOError)
	}
}

func TestRun_ExplicitMissingConfigExitsUsage(t *testing.T) {
	// --config <missing> is a hard error per contracts/cli.md
	// "Discovery rules" #1 — exit 2.
	got := run([]string{"--config", filepath.Join(t.TempDir(), "no.toml")})
	if got != exitUsageError {
		t.Errorf("missing --config: got exit %d want %d", got, exitUsageError)
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
	got := run([]string{"--config", p, filepath.Join(t.TempDir(), "missing.txt")})
	if got != exitIOError {
		t.Errorf("bad config + missing file: got exit %d want %d", got, exitIOError)
	}
}

func TestRun_NoColorFlag(t *testing.T) {
	// --no-color sets cfg.NoColor = true; pair with a missing file so
	// the run exits before tea.Program starts.
	got := run([]string{"--no-config", "--no-color", filepath.Join(t.TempDir(), "missing.txt")})
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
	got := run([]string{"--no-config", p})
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

func TestBoolPtr(t *testing.T) {
	if boolPtr(false) != nil {
		t.Errorf("boolPtr(false) should be nil to signal 'not set'")
	}
	if got := boolPtr(true); got == nil || *got != true {
		t.Errorf("boolPtr(true): got %v", got)
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
	if got := run([]string{"--no-config", "--vim", p}); got != exitOK {
		t.Errorf("--vim: got exit %d want %d", got, exitOK)
	}
}
