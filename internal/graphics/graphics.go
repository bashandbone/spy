// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"image"

	"github.com/knitli/spy/internal/term"
)

// Renderer is the per-protocol encoder selected at startup based on
// [term.Capabilities]. The Phase 2 foundational viewer never emits
// images — the per-protocol implementations land in US4 (T075–T079).
// This skeleton returns a no-op Renderer for every protocol so render
// and ui can wire dependencies through their constructors now.
type Renderer interface {
	Render(img image.Image, cols, rows int) (string, error)
	Cleanup() string
}

// RendererFor picks the Renderer matching `proto`. Idempotent; in Phase
// 2 every protocol returns the no-op Renderer because the encoders are
// not yet implemented.
func RendererFor(_ term.Graphics) Renderer { return noopRenderer{} }

// Render encodes `img` into the active graphics protocol. Phase 2
// returns an empty string for every protocol; US4 fills it in.
func Render(_ term.Graphics, _ image.Image, _, _ int) (string, error) {
	return "", nil
}

// Cleanup emits any escape sequences needed to clear residual images
// (Kitty "delete all images" in particular). No-op in Phase 2; the
// Kitty path is wired in T083 (US4) so the cleanup defer chain in
// cmd/spy/main is load-bearing then.
func Cleanup(_ term.Graphics) string { return "" }

// CleanupFunc returns a closure that writes the cleanup sequence
// directly to os.Stdout. Safe to defer in main(); the closure is
// idempotent and a no-op in Phase 2. T083 (US4) replaces the body for
// Kitty so the cleanup actually fires on quit / signal / panic.
func CleanupFunc(_ term.Graphics) func() { return func() {} }

// noopRenderer is the placeholder Renderer used while the per-protocol
// encoders are still stubs.
type noopRenderer struct{}

func (noopRenderer) Render(_ image.Image, _, _ int) (string, error) { return "", nil }
func (noopRenderer) Cleanup() string                                { return "" }
