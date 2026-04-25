// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"image"
	"testing"

	"github.com/knitli/spy/internal/term"
)

// Phase 2 graphics is a no-op skeleton; the per-protocol encoders land
// in US4 (T075–T079). These smoke tests pin the contract surface so a
// future change that breaks the no-op semantics is caught quickly.

func TestRendererFor_AllProtocols(t *testing.T) {
	for _, p := range []term.Graphics{
		term.GraphicsNone, term.GraphicsKitty,
		term.GraphicsITerm2, term.GraphicsSixel,
	} {
		r := RendererFor(p)
		if r == nil {
			t.Errorf("RendererFor(%v) returned nil", p)
		}
	}
}

func TestRender_NoopReturnsEmpty(t *testing.T) {
	out, err := Render(term.GraphicsKitty, image.NewRGBA(image.Rect(0, 0, 1, 1)), 1, 1)
	if err != nil {
		t.Errorf("Render: %v", err)
	}
	if out != "" {
		t.Errorf("Phase 2 Render must return \"\", got %q", out)
	}
}

func TestCleanup_NoopReturnsEmpty(t *testing.T) {
	if got := Cleanup(term.GraphicsKitty); got != "" {
		t.Errorf("Phase 2 Cleanup must return \"\", got %q", got)
	}
}

func TestCleanupFunc_NoopDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CleanupFunc no-op panicked: %v", r)
		}
	}()
	fn := CleanupFunc(term.GraphicsKitty)
	if fn == nil {
		t.Fatal("CleanupFunc returned nil")
	}
	fn()
	fn() // idempotent
}

func TestNoopRenderer_RenderReturnsEmpty(t *testing.T) {
	r := RendererFor(term.GraphicsNone)
	out, err := r.Render(image.NewRGBA(image.Rect(0, 0, 1, 1)), 80, 24)
	if err != nil {
		t.Errorf("noopRenderer.Render: %v", err)
	}
	if out != "" {
		t.Errorf("noopRenderer.Render: got %q want \"\"", out)
	}
	if got := r.Cleanup(); got != "" {
		t.Errorf("noopRenderer.Cleanup: got %q want \"\"", got)
	}
}
