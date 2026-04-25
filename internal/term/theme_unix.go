// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package term

import (
	"context"
	"os"

	xterm "golang.org/x/term"
)

// probeOSC11Background performs the OSC 11 round-trip against
// /dev/tty. The strategy:
//
//  1. Open /dev/tty so the probe still works when stdin/stdout are
//     redirected to pipes (a piped session would have aborted earlier
//     in [detectBackgroundLuminance] via the !IsTTY bypass, but
//     /dev/tty is the only correct receiver for a controlling
//     terminal's OSC reply).
//  2. Switch the FD into raw mode via [golang.org/x/term.MakeRaw] so
//     the reply doesn't echo and Read returns the moment a byte
//     arrives — the same approach termenv uses, just without the
//     hardcoded 5 s OSCTimeout we can't override.
//  3. Write the OSC 11 query.
//  4. Hand the FD to [raceReadOSCReply] which reads the reply on a
//     goroutine and races completion against `ctx.Done()` so the 50 ms
//     budget is respected even when the terminal never replies.
//
// Defensive parsing — including rejecting CSI-embedded replies — is
// the responsibility of [parseOSC11Reply]; this function only owns the
// IO and the "don't echo what we read" promise (achieved trivially by
// returning the buffered string instead of writing it).
//
// Coverage note: the open + raw-mode + write triple genuinely requires
// a controlling terminal and isn't exercised by `go test`; the read
// loop is tested through [readOSCReply] and the goroutine race through
// [raceReadOSCReply].
func probeOSC11Background(ctx context.Context) string {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return ""
	}
	defer f.Close()

	fd := int(f.Fd())
	state, err := xterm.MakeRaw(fd)
	if err != nil {
		return ""
	}
	defer func() {
		_ = xterm.Restore(fd, state)
	}()

	if _, err := f.Write([]byte("\x1b]11;?\x1b\\")); err != nil {
		return ""
	}

	buf := raceReadOSCReply(ctx, f)
	if len(buf) == 0 {
		return ""
	}
	return string(buf)
}
