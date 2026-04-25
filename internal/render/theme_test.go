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
