// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package term

import (
	"context"
	"math"
	"testing"
)

// resetEnv zeroes every env var Detect inspects so a test only sees the
// inputs it explicitly sets via [testing.T.Setenv]. t.Setenv auto-restores
// after the test, so tests stay isolated even though they all touch the
// process-global environment.
func resetEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"NO_COLOR", "COLORTERM", "TERM", "TERM_PROGRAM",
		"KITTY_WINDOW_ID", "TMUX",
		"SPY_GRAPHICS", "SPY_THEME",
		"COLUMNS", "LINES",
	} {
		t.Setenv(k, "")
	}
}

func TestDetect_LuminanceIsNaNInPhase2(t *testing.T) {
	resetEnv(t)
	caps := Detect(context.Background())
	if !math.IsNaN(caps.BackgroundLuminance) {
		t.Errorf("Phase 2 BackgroundLuminance must be NaN until US3, got %v",
			caps.BackgroundLuminance)
	}
}

func TestDetect_NoColorForcesMono(t *testing.T) {
	resetEnv(t)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLORTERM", "truecolor") // would otherwise win
	t.Setenv("TERM", "xterm-256color") // and so would this
	caps := Detect(context.Background())
	if caps.ColorDepth != ColorMono {
		t.Errorf("NO_COLOR should force Mono, got %v", caps.ColorDepth)
	}
}

func TestDetect_ColorDepthFromColorterm(t *testing.T) {
	cases := []struct {
		colorterm string
		want      ColorDepth
	}{
		{"truecolor", ColorTrueColor},
		{"24bit", ColorTrueColor},
		{"TRUECOLOR", ColorTrueColor}, // case-insensitive
	}
	for _, tc := range cases {
		t.Run(tc.colorterm, func(t *testing.T) {
			resetEnv(t)
			t.Setenv("COLORTERM", tc.colorterm)
			caps := Detect(context.Background())
			if caps.ColorDepth != tc.want {
				t.Errorf("COLORTERM=%q: got %v want %v",
					tc.colorterm, caps.ColorDepth, tc.want)
			}
		})
	}
}

func TestDetect_ColorDepthFromTerm(t *testing.T) {
	cases := []struct {
		term string
		want ColorDepth
	}{
		{"xterm-256color", ColorANSI256},
		{"screen-256color", ColorANSI256},
		{"tmux-256color", ColorANSI256},
		{"xterm-color", ColorANSI16},
		{"linux", ColorANSI16},
		{"dumb", ColorMono},
		{"", ColorMono},
	}
	for _, tc := range cases {
		t.Run(tc.term, func(t *testing.T) {
			resetEnv(t)
			t.Setenv("TERM", tc.term)
			caps := Detect(context.Background())
			if caps.ColorDepth != tc.want {
				t.Errorf("TERM=%q: got %v want %v",
					tc.term, caps.ColorDepth, tc.want)
			}
		})
	}
}

func TestDetect_GraphicsAutoDetect(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Graphics
	}{
		{"kitty by KITTY_WINDOW_ID", map[string]string{"KITTY_WINDOW_ID": "1"}, GraphicsKitty},
		{"kitty by TERM", map[string]string{"TERM": "xterm-kitty"}, GraphicsKitty},
		{"iterm2 app", map[string]string{"TERM_PROGRAM": "iTerm.app"}, GraphicsITerm2},
		{"wezterm uses iterm2 protocol", map[string]string{"TERM_PROGRAM": "WezTerm"}, GraphicsITerm2},
		{"plain xterm", map[string]string{"TERM": "xterm-256color"}, GraphicsNone},
		{"nothing", nil, GraphicsNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			caps := Detect(context.Background())
			if caps.Graphics != tc.want {
				t.Errorf("%s: got %v want %v", tc.name, caps.Graphics, tc.want)
			}
		})
	}
}

func TestDetect_SPYGraphicsOverride(t *testing.T) {
	cases := []struct {
		val  string
		want Graphics
	}{
		{"none", GraphicsNone},
		{"kitty", GraphicsKitty},
		{"iterm2", GraphicsITerm2},
		{"iterm", GraphicsITerm2},
		{"sixel", GraphicsSixel},
		{"NONE", GraphicsNone}, // case-insensitive
	}
	for _, tc := range cases {
		t.Run("SPY_GRAPHICS="+tc.val, func(t *testing.T) {
			resetEnv(t)
			// Set a competing auto-detect signal — override must win.
			t.Setenv("KITTY_WINDOW_ID", "999")
			t.Setenv("SPY_GRAPHICS", tc.val)
			caps := Detect(context.Background())
			if caps.Graphics != tc.want {
				t.Errorf("SPY_GRAPHICS=%q: got %v want %v",
					tc.val, caps.Graphics, tc.want)
			}
		})
	}
}

func TestDetect_TmuxDisablesAutoGraphics(t *testing.T) {
	resetEnv(t)
	t.Setenv("KITTY_WINDOW_ID", "1") // would auto-detect as Kitty outside tmux
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	caps := Detect(context.Background())
	if !caps.InTmux {
		t.Errorf("InTmux should be true when TMUX is set")
	}
	if caps.Graphics != GraphicsNone {
		t.Errorf("tmux must drop auto-detected graphics, got %v", caps.Graphics)
	}
}

func TestDetect_TmuxYieldsToSpyGraphicsOverride(t *testing.T) {
	// Inside tmux, an explicit SPY_GRAPHICS=kitty (e.g., with passthrough
	// enabled) must still win — tmux is heuristic, the user is authoritative.
	resetEnv(t)
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	t.Setenv("SPY_GRAPHICS", "kitty")
	caps := Detect(context.Background())
	if caps.Graphics != GraphicsKitty {
		t.Errorf("explicit SPY_GRAPHICS=kitty must win inside tmux, got %v", caps.Graphics)
	}
}

func TestDetect_DimensionsFromEnvFallback(t *testing.T) {
	// Under `go test` stdout is rarely a TTY, so term.GetSize fails and
	// Detect falls back to COLUMNS / LINES.
	resetEnv(t)
	t.Setenv("COLUMNS", "120")
	t.Setenv("LINES", "40")
	caps := Detect(context.Background())
	if caps.IsTTY {
		t.Skip("test runner has a real TTY; env-fallback path not exercised")
	}
	if caps.Cols != 120 || caps.Rows != 40 {
		t.Errorf("dimensions: got %dx%d want 120x40", caps.Cols, caps.Rows)
	}
}

func TestDetect_DimensionsDefaultWhenAbsent(t *testing.T) {
	resetEnv(t)
	caps := Detect(context.Background())
	if caps.IsTTY {
		t.Skip("test runner has a real TTY; default path not exercised")
	}
	if caps.Cols < 1 || caps.Rows < 1 {
		t.Errorf("dimensions must default to a non-zero terminal-like size, got %dx%d",
			caps.Cols, caps.Rows)
	}
}

func TestDetect_ContextCancelEarlyReturn(t *testing.T) {
	resetEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	caps := Detect(ctx)
	if !math.IsNaN(caps.BackgroundLuminance) {
		t.Errorf("cancelled ctx must yield NaN luminance")
	}
}

func TestDetect_RecordsProgramAndTerm(t *testing.T) {
	resetEnv(t)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "WezTerm")
	caps := Detect(context.Background())
	if caps.Term != "xterm-256color" {
		t.Errorf("Term: got %q want %q", caps.Term, "xterm-256color")
	}
	if caps.Program != "WezTerm" {
		t.Errorf("Program: got %q want %q", caps.Program, "WezTerm")
	}
}

func TestDetect_SPYThemeAcceptsOverride(t *testing.T) {
	// In Phase 2 SPY_THEME does not yet bypass a probe (there is none), but
	// the call must remain a no-op rather than crash and the placeholder
	// luminance must remain NaN. T065 (US3) makes this load-bearing.
	resetEnv(t)
	t.Setenv("SPY_THEME", "dark")
	caps := Detect(context.Background())
	if !math.IsNaN(caps.BackgroundLuminance) {
		t.Errorf("Phase 2 with SPY_THEME=dark: BackgroundLuminance must be NaN")
	}
}

// --- Restore ---

func TestRestore_ReturnsNonNilFunc(t *testing.T) {
	resetEnv(t)
	fn := Restore()
	if fn == nil {
		t.Fatal("Restore returned nil")
	}
}

func TestRestore_Idempotent(t *testing.T) {
	resetEnv(t)
	fn := Restore()
	fn()
	fn() // second call must not panic
	fn() // and a third
}

func TestRestore_NoPanicOnNonTTY(t *testing.T) {
	// In test mode stdout is rarely a TTY; the call must still succeed and
	// the closure must be safe to defer.
	resetEnv(t)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Restore panicked on non-TTY: %v", r)
		}
	}()
	fn := Restore()
	fn()
}
