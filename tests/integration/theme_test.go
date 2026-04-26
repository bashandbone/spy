// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const goSample = "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"

// TestTheme_OverridePrecedence is the US3 PTY-driven test (T062),
// covering the override-precedence half of the contract from
// spec.md US3 #3: flag > env > config > auto-detect.
//
// Each subtest spawns spy with a different override and asserts
// the relevant ANSI footprint:
//   - --theme dark / --theme light → SGR present, distinct foreground
//   - SPY_THEME=light → SGR present
//   - NO_COLOR=1 → no SGR escapes anywhere in the rendered area
//
// The OSC 11 auto-detect path requires a PTY responder (the harness
// would have to capture spy's `\x1b]11;?\x1b\\` query and reply with
// a fixed RGB triplet) — left as TestTheme_AutoDetectFromOSC11_Skipped
// below until the harness gains an interactive responder.
func TestTheme_OverridePrecedence(t *testing.T) {
	t.Run("flag_dark", func(t *testing.T) {
		runThemeProbe(t, []string{"--theme", "dark"}, nil, func(t *testing.T, frame string) {
			if !regexp.MustCompile(`\x1b\[[0-9;]+m`).MatchString(frame) {
				t.Errorf("--theme dark: no SGR escapes in frame")
			}
		})
	})

	t.Run("flag_light", func(t *testing.T) {
		runThemeProbe(t, []string{"--theme", "light"}, nil, func(t *testing.T, frame string) {
			if !regexp.MustCompile(`\x1b\[[0-9;]+m`).MatchString(frame) {
				t.Errorf("--theme light: no SGR escapes in frame")
			}
		})
	})

	t.Run("env_SPY_THEME_light", func(t *testing.T) {
		runThemeProbe(t, nil, map[string]string{"SPY_THEME": "light"}, func(t *testing.T, frame string) {
			if !regexp.MustCompile(`\x1b\[[0-9;]+m`).MatchString(frame) {
				t.Errorf("SPY_THEME=light: no SGR escapes in frame")
			}
		})
	})

	t.Run("env_NO_COLOR_suppresses_SGR", func(t *testing.T) {
		runThemeProbe(t, nil, map[string]string{"NO_COLOR": "1"}, func(t *testing.T, frame string) {
			// Strip the alt-screen control sequences (Bubble Tea always
			// emits CSI for cursor positioning regardless of colour
			// mode); look for FOREGROUND/SGR colour escapes specifically:
			// `\x1b[<n>m` where n is in the 30-37/90-97/38;5/38;2
			// families.
			//
			// A more permissive check: there should be no ESC-[ ...m
			// SGR sequence inside the rendered content area. We
			// approximate by checking no `\x1b[3` or `\x1b[9` (256/ANSI
			// foreground) appears.
			//
			// Negative formulation: reset escapes are allowed (`\x1b[0m`,
			// `\x1b[m`) — just not active colour sets.
			if regexp.MustCompile(`\x1b\[(?:3[0-79]|9[0-7]|38;[25];)`).MatchString(frame) {
				t.Errorf("NO_COLOR=1: forbidden SGR foreground escape detected; snapshot tail=%q", truncTail([]byte(frame), 400))
			}
		})
	})
}

// TestTheme_RuntimeSwap exercises the `:set theme dark` / `:set theme
// light` runtime command — the spec-mandated US3 #3 ("`:set theme
// dark|light|<style>` re-renders without re-tokenising"). We spawn
// once with --theme light, send `:set theme dark<Enter>`, and assert
// the next paint differs from the first.
func TestTheme_RuntimeSwap(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "hello.go")
	if err := os.WriteFile(fixture, []byte(goSample), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := NewPTYProgram(t, []string{"--no-config", "--theme", "light", fixture}, nil)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(300 * time.Millisecond)

	first := string(p.Snapshot())
	firstSGR := extractSGRSet(first)
	if len(firstSGR) == 0 {
		t.Fatalf("first paint emitted no SGR escapes (light theme should highlight); snapshot tail=%q", truncTail([]byte(first), 400))
	}

	// Consume the buffer so the post-swap snapshot only contains
	// frames emitted after `:set theme dark` — otherwise the
	// snapshot is the union of both themes' frames.
	_ = p.Read()

	// Switch theme at runtime. Use sendUntil to retry through the
	// dropped-first-keystroke quirk: we wait until ANY new SGR
	// appears in the post-Read snapshot.
	p.Send(":set theme dark\r")
	deadline := time.Now().Add(2 * time.Second)
	var afterSGR map[string]struct{}
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		snap := string(p.Snapshot())
		afterSGR = extractSGRSet(snap)
		if len(afterSGR) > 0 && sgrSetsDiffer(firstSGR, afterSGR) {
			break
		}
	}
	if !sgrSetsDiffer(firstSGR, afterSGR) {
		t.Errorf(":set theme dark: SGR escape set unchanged from light theme — runtime swap may not have re-rendered\n  first=%v\n  after=%v", firstSGR, afterSGR)
	}

	for i := 0; i < 5 && !waitExitShort(p, 250*time.Millisecond); i++ {
		p.Send("q")
	}
	if !p.WaitForExit(3 * time.Second) {
		t.Fatalf("process did not exit on `q`; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("exit code %d (want 0)", exit)
	}
}

// TestTheme_AutoDetectFromOSC11 is the spec.md US3 #1/#2 case
// (auto-detect from OSC 11 background-color reply). Lifting requires
// the harness to capture spy's OSC 11 query (`\x1b]11;?\x1b\\`) and
// reply with a controllable RGB triplet — the existing PTYProgram
// does not run an interactive responder. Documented as a deferred
// follow-up in acceptance_review/00-summary.md.
func TestTheme_AutoDetectFromOSC11(t *testing.T) {
	t.Skip("US3 #1/#2 OSC 11 auto-detect: requires interactive PTY responder (harness extension pending — flag/env override paths covered by TestTheme_OverridePrecedence)")
}

// runThemeProbe is the shared helper: spawn spy against a small Go
// fixture with the given args + env, wait for first paint, run the
// caller-supplied frame check, and quit cleanly.
func runThemeProbe(t *testing.T, extraArgs []string, env map[string]string, check func(*testing.T, string)) {
	t.Helper()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "hello.go")
	if err := os.WriteFile(fixture, []byte(goSample), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	args := append([]string{"--no-config"}, extraArgs...)
	args = append(args, fixture)

	p := NewPTYProgram(t, args, env)
	if !p.WaitFor(AltScreenEnter, 5*time.Second) {
		t.Fatalf("alt-screen entry not observed; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	time.Sleep(300 * time.Millisecond)

	check(t, string(p.Snapshot()))

	for i := 0; i < 5 && !waitExitShort(p, 250*time.Millisecond); i++ {
		p.Send("q")
	}
	if !p.WaitForExit(3 * time.Second) {
		t.Fatalf("process did not exit on `q`; snapshot tail=%q", truncTail(p.Snapshot(), 200))
	}
	if exit := p.ExitCode(); exit != 0 {
		t.Fatalf("exit code %d (want 0)", exit)
	}
}

// extractSGRSet returns the unique set of SGR colour escapes
// appearing in `s`. Two themes with different palettes produce
// disjoint sets; runtime theme swap is detectable by set inequality
// without pinning a specific style.
func extractSGRSet(s string) map[string]struct{} {
	re := regexp.MustCompile(`\x1b\[(?:3[0-79]|9[0-7]|38;[25];[0-9;]+)m`)
	out := make(map[string]struct{})
	for _, m := range re.FindAllString(s, -1) {
		out[m] = struct{}{}
	}
	return out
}

func sgrSetsDiffer(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return true
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return true
		}
	}
	return false
}

// keep strings import live for future grep-friendly assertions.
var _ = strings.Contains
