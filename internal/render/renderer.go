// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package render owns the per-Kind frame producer the UI calls each
// tick. The dependency direction is render → source, loader, term,
// graphics, highlight, search; render does NOT import internal/ui.
// internal/ui builds a [RenderContext] from its session state and
// passes it in.
package render

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"

	"github.com/knitli/spy/internal/graphics"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/search"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// Status is the high-level state of the active viewer session that the
// status bar surfaces.
type Status int

const (
	StatusIdle Status = iota
	StatusLoading
	StatusStreaming
	StatusError
)

// RenderContext carries the per-frame state a [Renderer] needs. Built
// fresh by internal/ui on each Update tick and passed into Render.
type RenderContext struct {
	Buffer       *loader.LineBuffer
	Viewport     viewport.Model
	Theme        Theme
	Capabilities term.Capabilities
	Search       search.State
	Status       Status
	LastError    error
	Page         int // 1-indexed; non-zero only for KindPDF
}

// Renderer is the per-Kind frame producer. The same Renderer handles
// every frame for the lifetime of a [Source]; renderers are reusable
// and stateless apart from explicit [RenderContext] inputs.
type Renderer interface {
	Render(ctx RenderContext) string
}

// Dependencies bundles the cross-package collaborators a Renderer is
// constructed with. The struct lets [ForKind] inject what each per-Kind
// renderer needs without long parameter lists.
type Dependencies struct {
	Theme        Theme
	Capabilities term.Capabilities
	Graphics     graphics.Renderer
	Highlighter  *highlight.Highlighter

	// LineNumbers and WordWrap mirror the active config for the current
	// session; renderers branch on them per frame so toggles
	// (Ctrl-L / Ctrl-W) take effect on the next tick.
	LineNumbers bool
	WordWrap    bool
}

// ForKind picks a [Renderer] for the supplied [source.Kind]. Unknown /
// pending kinds receive a stub that prints a "pending USx" frame; the
// stubs are replaced in their respective story phases (US1: Code,
// Markdown; US4: PDF, Image).
func ForKind(k source.Kind, deps Dependencies) Renderer {
	switch k {
	case source.KindText:
		return &textRenderer{deps: deps}
	case source.KindCode:
		return &stubRenderer{name: "Code", pending: "US1 (T043)"}
	case source.KindMarkdown:
		return &stubRenderer{name: "Markdown", pending: "US1 (T044)"}
	case source.KindPDF:
		return &stubRenderer{name: "PDF", pending: "US4 (T081)"}
	case source.KindImage:
		return &stubRenderer{name: "Image", pending: "US4 (T080)"}
	case source.KindBinary:
		return &binaryRenderer{deps: deps}
	default:
		return &stubRenderer{name: "Unknown", pending: "US1 (T045)"}
	}
}

// stubRenderer prints a deterministic "pending USx" frame so the Phase
// 2 viewer doesn't crash when fed a Kind whose real renderer hasn't
// been written yet.
type stubRenderer struct {
	name    string
	pending string
}

func (s *stubRenderer) Render(_ RenderContext) string {
	return fmt.Sprintf("[%s renderer is pending — see %s]\n", s.name, s.pending)
}

// binaryRenderer is what we emit when content was rejected as binary
// content. The user already saw a stderr error from main; the in-app
// frame is a polite reminder rather than a crash.
type binaryRenderer struct {
	deps Dependencies
}

func (b *binaryRenderer) Render(_ RenderContext) string {
	return "[binary content — refusing to display]\n"
}
