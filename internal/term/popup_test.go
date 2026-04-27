// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package term

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
		{"a'b'c", `'a'\''b'\''c'`},
		{"/usr/bin/spy", "'/usr/bin/spy'"},
		{"--flag=value", "'--flag=value'"},
	}
	for _, tc := range cases {
		got := shellQuote(tc.in)
		if got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestShellQuote_SafeForShell(t *testing.T) {
	// Verify that single-quoting a string with embedded single quotes
	// produces a valid shell token: starts and ends with ', and any
	// interior single quote is escaped via the '\'' sequence.
	tricky := "don't stop; rm -rf / # evil"
	q := shellQuote(tricky)
	if !strings.HasPrefix(q, "'") {
		t.Errorf("shellQuote result must start with single quote, got %q", q)
	}
	// Strip all '\'' escape sequences, then the outer quotes; no bare
	// single quote should remain.
	stripped := strings.ReplaceAll(q, `'\''`, "")
	stripped = strings.TrimPrefix(stripped, "'")
	stripped = strings.TrimSuffix(stripped, "'")
	if strings.Contains(stripped, "'") {
		t.Errorf("shellQuote result contains unescaped single quote: %q", q)
	}
}

func TestPopupSentinelEnv(t *testing.T) {
	// Validate that PopupSentinelEnv is a legal POSIX shell variable name:
	// must be non-empty, start with a letter or underscore, and contain
	// only letters, digits, or underscores. This is stricter than the old
	// check so it would catch a change that breaks the shell assignment
	// prefix used in LaunchTmuxPopup.
	if PopupSentinelEnv == "" {
		t.Fatal("PopupSentinelEnv must not be empty")
	}
	for i, c := range PopupSentinelEnv {
		valid := unicode.IsLetter(c) || c == '_' || (i > 0 && unicode.IsDigit(c))
		if !valid {
			t.Errorf("PopupSentinelEnv %q: character %q at position %d is not a valid POSIX identifier character",
				PopupSentinelEnv, c, i)
		}
	}
	// First character must be letter or underscore (not digit).
	first := rune(PopupSentinelEnv[0])
	if !unicode.IsLetter(first) && first != '_' {
		t.Errorf("PopupSentinelEnv %q must start with a letter or underscore", PopupSentinelEnv)
	}
}

// fakeTmux writes a minimal shell script to dir/tmux that exits with the
// given code and returns the directory prepended PATH.
func fakeTmux(t *testing.T, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	script := []byte("#!/bin/sh\nexit " + itoa(exitCode) + "\n")
	path := filepath.Join(dir, "tmux")
	if err := os.WriteFile(path, script, 0o755); err != nil {
		t.Fatalf("fakeTmux: write script: %v", err)
	}
	return dir + string(filepath.ListSeparator) + os.Getenv("PATH")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func TestLaunchTmuxPopup_NoTmux(t *testing.T) {
	// With an empty PATH, tmux cannot be found; LaunchTmuxPopup should
	// return a non-nil error so the caller falls through to pager mode.
	t.Setenv("PATH", t.TempDir()) // directory exists but holds no binaries
	code, err := LaunchTmuxPopup([]string{"test-arg"})
	if err == nil {
		t.Errorf("expected error when tmux is not on PATH, got nil (code %d)", code)
	}
}

func TestLaunchTmuxPopup_CleanExit(t *testing.T) {
	// A fake tmux that exits 0 simulates a clean popup close (user pressed q).
	// LaunchTmuxPopup should return (0, nil).
	t.Setenv("PATH", fakeTmux(t, 0))
	code, err := LaunchTmuxPopup([]string{"README.md"})
	if err != nil {
		t.Fatalf("unexpected error on clean exit: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestLaunchTmuxPopup_NonZeroExit(t *testing.T) {
	// A fake tmux that exits 130 simulates the inner spy receiving Ctrl-C.
	// LaunchTmuxPopup should propagate the code and return nil error so
	// the caller returns 130 without re-running spy in the current pane.
	t.Setenv("PATH", fakeTmux(t, 130))
	code, err := LaunchTmuxPopup([]string{})
	if err != nil {
		t.Fatalf("non-zero popup exit must not be a launch error, got: %v", err)
	}
	if code != 130 {
		t.Errorf("expected exit code 130, got %d", code)
	}
}
