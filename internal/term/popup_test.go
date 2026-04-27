// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package term

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	// Validate that PopupSentinelEnv is a legal env-var name: non-empty,
	// starts with a letter or underscore, contains only letters, digits,
	// or underscores. This ensures it is valid for tmux's -e NAME=value
	// flag syntax as well as a standard OS environment variable.
	if PopupSentinelEnv == "" {
		t.Fatal("PopupSentinelEnv must not be empty")
	}
	// POSIX env var names are [A-Za-z_][A-Za-z0-9_]* — ASCII only.
	for i, c := range PopupSentinelEnv {
		isASCIILetter := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		isASCIIDigit := c >= '0' && c <= '9'
		valid := isASCIILetter || c == '_' || (i > 0 && isASCIIDigit)
		if !valid {
			t.Errorf("PopupSentinelEnv %q: character %q at position %d is not a valid POSIX identifier character",
				PopupSentinelEnv, c, i)
		}
	}
	// First character must be an ASCII letter or underscore (not a digit).
	first := rune(PopupSentinelEnv[0])
	isASCIILetter := (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')
	if !isASCIILetter && first != '_' {
		t.Errorf("PopupSentinelEnv %q must start with an ASCII letter or underscore", PopupSentinelEnv)
	}
}

// fakeTmux writes a shell script to dir/tmux that:
//   - responds to "list-commands" by printing "display-popup" and exiting 0
//   - responds to any other subcommand by exiting with exitCode
//
// It returns the directory prepended to PATH so exec.LookPath finds it first.
func fakeTmux(t *testing.T, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  list-commands) printf 'display-popup\\n'; exit 0 ;;\n" +
		"  *) exit " + itoa(exitCode) + " ;;\n" +
		"esac\n"
	path := filepath.Join(dir, "tmux")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
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

func TestLaunchTmuxPopup_NoDisplayPopup(t *testing.T) {
	// A fake tmux that is present but does not advertise display-popup in
	// list-commands output (simulates tmux < 3.2). LaunchTmuxPopup should
	// return a non-nil error so the caller falls through to pager mode.
	dir := t.TempDir()
	script := []byte("#!/bin/sh\ncase \"$1\" in\n  list-commands) exit 0 ;;\n  *) exit 0 ;;\nesac\n")
	if err := os.WriteFile(filepath.Join(dir, "tmux"), script, 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", dir+string(filepath.ListSeparator)+os.Getenv("PATH"))
	code, err := LaunchTmuxPopup([]string{})
	if err == nil {
		t.Errorf("expected error when display-popup not available, got nil (code %d)", code)
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

func TestLaunchTmuxPopup_SentinelViaEnvFlag(t *testing.T) {
	// LaunchTmuxPopup must pass the sentinel via tmux -e NAME=val, not as
	// a shell prefix assignment in the command string. This ensures the
	// sentinel reaches the inner process even on platforms (e.g. WSL2)
	// where the popup PTY fails IsTerminal checks before the shell runs.
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "tmux-args")
	// Write a fake tmux that records each argument on its own line.
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  list-commands) printf 'display-popup\\n'; exit 0 ;;\n" +
		"  *) for arg in \"$@\"; do printf '%s\\n' \"$arg\"; done > " + argsFile + "; exit 0 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", dir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	if _, err := LaunchTmuxPopup([]string{"file.txt"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Verify -e PopupSentinelEnv=1 is present as consecutive arguments.
	foundSentinel := false
	for i, arg := range args {
		if arg == "-e" && i+1 < len(args) && args[i+1] == PopupSentinelEnv+"=1" {
			foundSentinel = true
			break
		}
	}
	if !foundSentinel {
		t.Errorf("tmux args missing -e %s=1; got: %v", PopupSentinelEnv, args)
	}

	// Verify the command string (last arg) does NOT start with the sentinel
	// as a shell prefix assignment.
	if len(args) > 0 {
		last := args[len(args)-1]
		if strings.HasPrefix(last, PopupSentinelEnv+"=") {
			t.Errorf("command string contains shell prefix assignment %q; sentinel must use -e flag", last)
		}
	}
}
