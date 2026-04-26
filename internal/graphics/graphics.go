// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"fmt"
	"image"
	"os"
	"sync"

	"github.com/knitli/spy/internal/term"
)

// Renderer is the per-protocol encoder selected at startup based on
// [term.Capabilities]. Each implementation wraps a single encoder
// function; the dispatcher [RendererFor] picks the matching one. The
// `cols` / `rows` parameters are the renderer's frame size — the Phase
// 6 encoders ignore them (the protocols handle scaling client-side)
// but the parameter is part of the contract because future encoders
// (chafa-style ANSI fallback) may want it.
type Renderer interface {
	Render(img image.Image, cols, rows int) (string, error)
	Cleanup() string
}

// RendererFor picks the [Renderer] matching `proto`. Idempotent;
// returns the no-op Renderer for [term.GraphicsNone] so callers can
// rely on a non-nil result.
func RendererFor(proto term.Graphics) Renderer {
	switch proto {
	case term.GraphicsKitty:
		return kittyRenderer{}
	case term.GraphicsITerm2:
		return iterm2Renderer{}
	case term.GraphicsSixel:
		return sixelRenderer{}
	}
	return noopRenderer{}
}

// Render encodes `img` into the active graphics protocol. The function
// is the package-level convenience around [RendererFor].Render — the
// renderer object is constructed fresh for every call, so it's cheap.
func Render(proto term.Graphics, img image.Image, cols, rows int) (string, error) {
	return RendererFor(proto).Render(img, cols, rows)
}

// Cleanup returns the protocol-specific "delete all images" escape.
// Empty for protocols that self-clean (iTerm2, sixel) or that never
// emitted anything (none).
func Cleanup(proto term.Graphics) string {
	return RendererFor(proto).Cleanup()
}

// CleanupFunc returns a closure that writes the cleanup sequence
// directly to os.Stdout. Safe to defer in main(); the closure is
// idempotent so a panic-driven defer chain that fires twice (once via
// the explicit defer and once via Bubble Tea's recovery) doesn't emit
// the escape twice. A no-op closure is returned for protocols that
// don't need cleanup so callers can defer unconditionally.
func CleanupFunc(proto term.Graphics) func() {
	seq := Cleanup(proto)
	if seq == "" {
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			// Best-effort: we deliberately ignore write errors because
			// this fires from a defer chain — a panicking program with
			// a closed stdout can't usefully react to a second error.
			_, _ = fmt.Fprint(os.Stdout, seq)
		})
	}
}

// noopRenderer is the placeholder Renderer used when the active
// protocol is [term.GraphicsNone]. It produces no escape sequences so
// the [render] package can branch on `out == ""` to fall back to the
// metadata-block path.
type noopRenderer struct{}

func (noopRenderer) Render(_ image.Image, _, _ int) (string, error) { return "", nil }
func (noopRenderer) Cleanup() string                                { return "" }

// kittyRenderer wraps [encodeKitty] in the [Renderer] contract. Cleanup
// emits Kitty's "delete all images" escape so residual frames don't
// outlive the session.
type kittyRenderer struct{}

func (kittyRenderer) Render(img image.Image, _, _ int) (string, error) {
	return encodeKitty(img)
}
func (kittyRenderer) Cleanup() string { return kittyDeleteAll }

// iterm2Renderer wraps [encodeITerm2]. iTerm2 self-cleans on alt-screen
// exit; the cleanup escape is a no-op.
type iterm2Renderer struct{}

func (iterm2Renderer) Render(img image.Image, _, _ int) (string, error) {
	return encodeITerm2(img)
}
func (iterm2Renderer) Cleanup() string { return "" }

// sixelRenderer wraps [encodeSixel]. Sixel is a one-shot raster blob;
// the terminal scrolls it like text, so there's nothing to "delete".
type sixelRenderer struct{}

func (sixelRenderer) Render(img image.Image, _, _ int) (string, error) {
	return encodeSixel(img)
}
func (sixelRenderer) Cleanup() string { return "" }
