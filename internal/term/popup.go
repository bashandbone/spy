// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PopupSentinelEnv is the environment variable injected by LaunchTmuxPopup
// into the shell command it passes to tmux. The re-exec'd spy process checks
// this to avoid infinite recursion. Using an env var rather than a CLI flag
// keeps the flag surface clean.
const PopupSentinelEnv = "_SPY_POPUP_ACTIVE"

// LaunchTmuxPopup re-execs the current spy binary inside a tmux
// display-popup overlay that floats over all panes at full terminal size.
// It blocks until the user dismisses the popup.
//
// originalArgs is argv[1:] from the calling process. LaunchTmuxPopup
// prepends the PopupSentinelEnv assignment to the shell command so the
// inner process skips re-launch.
//
// Returns (exitCode, nil) when the popup ran to completion — including
// non-zero inner exits such as Ctrl-C (130) or signal-driven exits. The
// caller should propagate exitCode directly.
//
// Returns (0, err) only when tmux itself could not be found or the
// subprocess failed to start; the caller should fall through to normal
// pager mode.
func LaunchTmuxPopup(originalArgs []string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("popup: resolve executable: %w", err)
	}

	// Build a POSIX shell command string:
	//   _SPY_POPUP_ACTIVE=1 '/path/to/spy' 'arg1' 'arg2' ...
	// The leading assignment sets the sentinel for the re-exec'd process
	// without requiring tmux's -e flag (which was added in tmux 3.2 for
	// display-popup but is not universally available in older 3.2 point
	// releases). Shell variable assignment before a command is POSIX.
	parts := make([]string, 0, len(originalArgs)+2)
	parts = append(parts, PopupSentinelEnv+"=1")
	parts = append(parts, shellQuote(exe))
	for _, a := range originalArgs {
		parts = append(parts, shellQuote(a))
	}

	cmd := exec.Command("tmux",
		"display-popup",
		"-E", // close popup when the command exits
		"-w", "100%",
		"-h", "100%",
		strings.Join(parts, " "),
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// tmux launched and ran the popup; the inner spy process
			// exited non-zero (e.g. Ctrl-C → 130). Return that code so
			// the caller can propagate it without re-running spy.
			return exitErr.ExitCode(), nil
		}
		// tmux not found or could not start — let the caller fall
		// through to normal pager mode.
		return 0, err
	}
	return 0, nil
}

// shellQuote wraps s in single quotes with interior single-quote characters
// replaced by the POSIX '\” escape sequence. Safe for any string value.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
