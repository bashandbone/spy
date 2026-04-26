// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package term

import (
	"context"
	"os"
	"syscall"
	"time"

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
//  4. Read the reply via [pollReadOSC], which uses O_NONBLOCK +
//     ctx-aware spin so it terminates when the context deadline fires.
//     This replaces the goroutine-based [raceReadOSCReply] to avoid a
//     goroutine that stays alive in a blocking read(2) after the 50 ms
//     budget expires: on Linux, close(fd) does NOT interrupt a blocked
//     read() on another OS thread, so the leaked goroutine competed
//     with Bubble Tea's cancelreader for the first keystroke the test
//     harness sent, causing intermittent dropped-keystroke failures
//     (see specs/001-popup-reader/acceptance_review/
//     pty_flake_investigation.md for the full diagnosis).
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

	buf := pollReadOSC(ctx, fd)
	if len(buf) == 0 {
		return ""
	}
	return string(buf)
}

// pollReadOSC reads the OSC 11 reply one byte at a time using
// O_NONBLOCK + a 5 ms spin so it terminates promptly when ctx is done.
// Unlike [raceReadOSCReply] this runs inline on the calling goroutine
// and leaves no goroutine alive after the context deadline fires.
//
// Implementation: set the fd to O_NONBLOCK, loop calling syscall.Read;
// on EAGAIN sleep 5 ms and retry; on any other error or ctx expiry
// break. Restore blocking mode before returning so callers can use the
// fd normally (e.g. xterm.Restore ioctl).
//
// The 5 ms spin adds at most 5 ms of latency beyond the budget (within
// the 50 ms [oscProbeBudget] ceiling); on terminals that never reply
// the overhead is one extra EAGAIN + sleep per iteration.
func pollReadOSC(ctx context.Context, fd int) []byte {
	// Switch to non-blocking so Read returns EAGAIN instead of blocking.
	if err := syscall.SetNonblock(fd, true); err != nil {
		return nil
	}
	// Restore blocking mode. Failure is safe to ignore here: the caller
	// (probeOSC11Background) closes fd immediately via defer f.Close(),
	// so a non-blocking fd would only affect the subsequent Close ioctl,
	// which is unaffected by the O_NONBLOCK flag.
	defer syscall.SetNonblock(fd, false) //nolint:errcheck

	out := make([]byte, 0, oscReplyMaxBytes)
	var b [1]byte
	for len(out) < oscReplyMaxBytes {
		if ctx.Err() != nil {
			break
		}
		n, err := syscall.Read(fd, b[:])
		if n > 0 {
			out = append(out, b[0])
			if seenTerminator(out) {
				break
			}
		}
		if err == syscall.EAGAIN {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if err != nil {
			break
		}
	}
	return out
}
