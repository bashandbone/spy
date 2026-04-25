// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"math"
	"testing"

	"github.com/knitli/spy/internal/term"
)

// Phase 2 ResolveTheme is the placeholder from T029: the auto-detect
// branch falls back to dark until US3 wires the OSC 11 luminance probe.
// The full implementation lands in T066.

func TestResolveTheme_DefaultIsDark(t *testing.T) {
	tm := ResolveTheme("auto", term.Capabilities{BackgroundLuminance: math.NaN()}, false)
	if tm.Name != "dark" {
		t.Errorf("auto + NaN luminance: got %q want dark", tm.Name)
	}
}

func TestResolveTheme_ExplicitDark(t *testing.T) {
	tm := ResolveTheme("dark", term.Capabilities{}, false)
	if tm.Name != "dark" {
		t.Errorf("explicit dark: got %q want dark", tm.Name)
	}
}

func TestResolveTheme_ExplicitLight(t *testing.T) {
	tm := ResolveTheme("light", term.Capabilities{}, false)
	if tm.Name != "light" {
		t.Errorf("explicit light: got %q want light", tm.Name)
	}
}

func TestResolveTheme_NamedChromaStyle(t *testing.T) {
	tm := ResolveTheme("github", term.Capabilities{}, false)
	// Resolution preserves the requested chroma style verbatim — the
	// theme name is the chroma style name.
	if tm.ChromaStyle != "github" {
		t.Errorf("explicit chroma style: got %q want github", tm.ChromaStyle)
	}
}

func TestResolveTheme_NoColorForcesMono(t *testing.T) {
	tm := ResolveTheme("dark", term.Capabilities{}, true)
	if !tm.Mono {
		t.Errorf("NoColor=true should mark theme Mono")
	}
}

// --- T061: caps-driven auto branch (US3) ---

func TestResolveTheme_AutoLightFromCaps(t *testing.T) {
	caps := term.Capabilities{BackgroundLuminance: 0.8}
	tm := ResolveTheme("auto", caps, false)
	if tm.Name != "light" {
		t.Errorf("auto + light caps (lum=0.8): got %q want light", tm.Name)
	}
}

func TestResolveTheme_AutoDarkFromCaps(t *testing.T) {
	caps := term.Capabilities{BackgroundLuminance: 0.1}
	tm := ResolveTheme("auto", caps, false)
	if tm.Name != "dark" {
		t.Errorf("auto + dark caps (lum=0.1): got %q want dark", tm.Name)
	}
}

func TestResolveTheme_AutoBoundaryAtHalf(t *testing.T) {
	// Luminance == 0.5 is the threshold; per research R6 ≥ 0.5 is light.
	caps := term.Capabilities{BackgroundLuminance: 0.5}
	tm := ResolveTheme("auto", caps, false)
	if tm.Name != "light" {
		t.Errorf("auto + lum=0.5 (≥0.5 boundary): got %q want light", tm.Name)
	}
}

func TestResolveTheme_AutoNaNFallsBackToDark(t *testing.T) {
	caps := term.Capabilities{BackgroundLuminance: math.NaN()}
	tm := ResolveTheme("auto", caps, false)
	if tm.Name != "dark" {
		t.Errorf("auto + NaN luminance: got %q want dark", tm.Name)
	}
}

func TestResolveTheme_ExplicitOverridesCaps(t *testing.T) {
	// An explicit light theme spec must win even when caps say "dark".
	caps := term.Capabilities{BackgroundLuminance: 0.05}
	tm := ResolveTheme("light", caps, false)
	if tm.Name != "light" {
		t.Errorf("explicit light + dark caps: got %q want light", tm.Name)
	}
}

func TestResolveTheme_NoColorWithCapsStillMono(t *testing.T) {
	caps := term.Capabilities{BackgroundLuminance: 0.8}
	tm := ResolveTheme("auto", caps, true)
	if !tm.Mono {
		t.Errorf("NoColor=true + auto + light caps: theme should still be Mono")
	}
	// And the underlying theme should be light, since caps drove the auto
	// resolution before Mono was layered on top.
	if tm.Name != "light" {
		t.Errorf("auto + light caps + NoColor: name=%q want light", tm.Name)
	}
}
