// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package term

import "context"

// probeOSC11Background is a no-op on platforms that don't have a unix
// /dev/tty + termios story we can drive cheaply. The OSC 11 path stays
// dark (returns ""), [detectBackgroundLuminance] then falls back to
// COLORFGBG and finally NaN — the same outcome the user would see on
// an emulator that ignores the query.
//
// Windows is the obvious candidate: the kernel doesn't expose
// /dev/tty, modern Windows terminals (Windows Terminal, ConEmu) handle
// theme detection differently, and the install base is small enough
// for v0.1.0 that "auto theme falls back to dark" is acceptable
// behavior.
func probeOSC11Background(_ context.Context) string {
	return ""
}
