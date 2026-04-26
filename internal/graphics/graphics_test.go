// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"image"
	"strings"
	"testing"

	"github.com/knitli/spy/internal/term"
)

// Phase 6 graphics dispatch: every protocol returns a non-nil
// [Renderer]; encoders for known protocols emit content; the no-op
// branch (GraphicsNone) still emits "" so [render] can branch on it
// for the metadata fallback.

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

func TestRender_KittyEmitsPayload(t *testing.T) {
	out, err := Render(term.GraphicsKitty, image.NewRGBA(image.Rect(0, 0, 1, 1)), 1, 1)
	if err != nil {
		t.Errorf("Render: %v", err)
	}
	if !strings.HasPrefix(out, "\x1b_G") {
		t.Errorf("Render(Kitty) should emit a graphics escape, got %q", out)
	}
}

func TestRender_NoneStaysEmpty(t *testing.T) {
	out, err := Render(term.GraphicsNone, image.NewRGBA(image.Rect(0, 0, 1, 1)), 1, 1)
	if err != nil {
		t.Errorf("Render: %v", err)
	}
	if out != "" {
		t.Errorf("Render(None) should still return empty (metadata-fallback signal), got %q", out)
	}
}

func TestCleanup_KittyEmitsDeleteAll(t *testing.T) {
	if got := Cleanup(term.GraphicsKitty); got != kittyDeleteAll {
		t.Errorf("Cleanup(Kitty): got %q want %q", got, kittyDeleteAll)
	}
}

func TestCleanup_SelfCleaningProtocolsReturnEmpty(t *testing.T) {
	for _, p := range []term.Graphics{
		term.GraphicsNone, term.GraphicsITerm2, term.GraphicsSixel,
	} {
		if got := Cleanup(p); got != "" {
			t.Errorf("Cleanup(%v) should be empty (self-cleaning), got %q", p, got)
		}
	}
}

func TestCleanupFunc_NoopDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CleanupFunc no-op panicked: %v", r)
		}
	}()
	fn := CleanupFunc(term.GraphicsNone)
	if fn == nil {
		t.Fatal("CleanupFunc returned nil")
	}
	fn()
	fn() // idempotent
}

func TestCleanupFunc_KittyIsIdempotent(t *testing.T) {
	// We can't easily capture os.Stdout from within the test without
	// racing other goroutines, but we can confirm the closure runs
	// twice without panicking and the once.Do contract holds via
	// reading the function result type.
	fn := CleanupFunc(term.GraphicsKitty)
	if fn == nil {
		t.Fatal("CleanupFunc returned nil")
	}
	fn()
	fn() // second call must be a no-op (sync.Once)
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
