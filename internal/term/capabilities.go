// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package term

import (
	"context"
	"math"
	"os"
	"strconv"
	"strings"

	xterm "golang.org/x/term"
)

// ColorDepth is the highest-fidelity colour space the terminal accepts.
type ColorDepth int

const (
	ColorMono ColorDepth = iota
	ColorANSI16
	ColorANSI256
	ColorTrueColor
)

// Graphics names the inline-image protocol the terminal supports.
type Graphics int

const (
	GraphicsNone Graphics = iota
	GraphicsKitty
	GraphicsITerm2
	GraphicsSixel
)

// Capabilities is the result of a single Detect call. Field semantics
// match contracts/internal-apis.md `internal/term`.
type Capabilities struct {
	IsTTY               bool
	Cols, Rows          int
	ColorDepth          ColorDepth
	Graphics            Graphics
	BackgroundLuminance float64 // NaN until US3 wires the OSC 11 probe
	Program, Term       string
	InTmux              bool
}

// Detect probes the current process's terminal. Honours SPY_GRAPHICS,
// SPY_THEME, NO_COLOR, COLORTERM. The total probe budget is bounded by
// `ctx`; cancellation causes an early return with a default-shaped
// [Capabilities]. The OSC 11 luminance probe is the only currently
// time-sensitive component (50 ms budget per research R6) and the
// caller-supplied ctx propagates into it.
//
// The function is goroutine-safe: it reads global env vars and does
// not mutate them. The reads are not snapshotted — the order is
// Detect → detectColorDepth → detectGraphics → detectBackgroundLuminance,
// each calling [os.Getenv] for the keys it needs — so a concurrent
// process that mutates the env between probes can technically see
// inconsistent values, but spy itself never does that.
func Detect(ctx context.Context) Capabilities {
	caps := Capabilities{
		BackgroundLuminance: math.NaN(),
	}
	if ctx.Err() != nil {
		return caps
	}

	caps.Term = os.Getenv("TERM")
	caps.Program = os.Getenv("TERM_PROGRAM")
	caps.InTmux = os.Getenv("TMUX") != ""

	fd := int(os.Stdout.Fd())
	caps.IsTTY = xterm.IsTerminal(fd)
	if caps.IsTTY {
		if cols, rows, err := xterm.GetSize(fd); err == nil {
			caps.Cols = cols
			caps.Rows = rows
		}
	}
	if caps.Cols == 0 {
		caps.Cols = parseIntEnv("COLUMNS", 80)
	}
	if caps.Rows == 0 {
		caps.Rows = parseIntEnv("LINES", 24)
	}

	caps.ColorDepth = detectColorDepth()
	caps.Graphics = detectGraphics(caps.InTmux)
	caps.BackgroundLuminance = detectBackgroundLuminance(
		ctx, envFromOS{}, caps.IsTTY, probeOSC11Background)

	return caps
}

func detectColorDepth() ColorDepth {
	if os.Getenv("NO_COLOR") != "" {
		return ColorMono
	}
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return ColorTrueColor
	}
	t := strings.ToLower(os.Getenv("TERM"))
	switch {
	case t == "" || t == "dumb":
		return ColorMono
	case strings.Contains(t, "256color"), strings.Contains(t, "256-color"):
		return ColorANSI256
	case strings.Contains(t, "color"):
		return ColorANSI16
	}
	// linux console, generic xterm, etc. — assume 16-colour ANSI.
	return ColorANSI16
}

func detectGraphics(inTmux bool) Graphics {
	// Explicit override wins everywhere — even inside tmux, because the
	// user (or wrapping tooling) may have configured passthrough.
	switch strings.ToLower(os.Getenv("SPY_GRAPHICS")) {
	case "none":
		return GraphicsNone
	case "kitty":
		return GraphicsKitty
	case "iterm", "iterm2":
		return GraphicsITerm2
	case "sixel":
		return GraphicsSixel
	case "":
		// fall through to auto-detect
	}
	// Inside tmux, drop auto-detected protocols. Most tmux versions strip
	// the escapes silently and the user gets a corrupted stream.
	if inTmux {
		return GraphicsNone
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return GraphicsKitty
	}
	if t := strings.ToLower(os.Getenv("TERM")); strings.Contains(t, "kitty") {
		return GraphicsKitty
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm":
		// WezTerm implements the iTerm2 inline-image protocol; treat it
		// as iTerm2 for emit purposes.
		return GraphicsITerm2
	}
	return GraphicsNone
}

func parseIntEnv(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// Restore captures the current terminal mode and returns a closure that
// restores it. Safe to defer; the closure is idempotent and a no-op when
// stdout is not a TTY (or when the state could not be captured).
//
// Callers should hold the closure for the lifetime of any code that may
// alter terminal modes (alt-screen, raw mode, mouse reporting) so that
// panics, signal-driven exits, and normal returns all leave the user's
// terminal in the captured state.
func Restore() func() {
	fd := int(os.Stdout.Fd())
	isTTY := xterm.IsTerminal(fd)
	// xterm.GetState returns (nil, err) on non-TTY; we ignore the error
	// so the closure body is uniform — the nil-state check inside guards
	// the no-TTY case without an early-return branch that would split the
	// closure logic into two functions.
	state, _ := xterm.GetState(fd)
	var done bool
	return func() {
		if done {
			return
		}
		done = true
		if isTTY && state != nil {
			_ = xterm.Restore(fd, state)
		}
	}
}
