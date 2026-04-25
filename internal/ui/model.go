// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package ui implements the Bubble Tea Model that drives the viewer.
// The Phase 2 foundational viewer wires loader → render → viewport with
// quit-on-q/esc/Ctrl-C and arrow-key scrolling. Search, vim, command
// line, and graphics cleanup are added in their respective story
// phases (US1–US6).
package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/viewport"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// ModelOptions matches contracts/internal-apis.md `internal/ui`.
type ModelOptions struct {
	Source       source.Source
	Stream       *loader.Stream
	Capabilities term.Capabilities
	Config       *config.Config
	Theme        render.Theme
	KeyMap       keys.KeyMap

	// Cancel cancels the loader's background streaming goroutine; the
	// model fires it on tea.Quit so Open's goroutine exits before the
	// program returns. Optional; nil is safe.
	Cancel context.CancelFunc
}

// Model is the Bubble Tea state for a viewer session. The fields are
// unexported because callers construct via [NewModel]; once constructed,
// the model is owned by tea.NewProgram for the duration of the run.
type Model struct {
	source   source.Source
	stream   *loader.Stream
	caps     term.Capabilities
	cfg      *config.Config
	theme    render.Theme
	keyMap   keys.KeyMap
	cancel   context.CancelFunc
	renderer render.Renderer

	viewport viewport.Model
	width    int
	height   int

	// showFooter mirrors the onResize decision: when the terminal is
	// too short for both viewport + footer (height <= 1), the footer
	// is suppressed so we don't overflow into a phantom row.
	showFooter bool

	// streaming flips false on the first chunkLoadedMsg with EOF=true;
	// before then the footer advertises "loading..." instead of the
	// final line count.
	streaming bool
}

// NewModel constructs the viewer's Bubble Tea model. The first frame
// uses the synchronously-loaded First chunk from the loader stream so
// the alt-screen paints immediately; further chunks arrive via
// chunkLoadedMsg as the producer streams.
func NewModel(opts ModelOptions) Model {
	deps := render.Dependencies{
		Theme:        opts.Theme,
		Capabilities: opts.Capabilities,
		LineNumbers:  opts.Config != nil && opts.Config.LineNumbers,
		WordWrap:     opts.Config != nil && opts.Config.WordWrap,
	}
	kind := source.KindUnknown
	if opts.Source != nil {
		kind = opts.Source.Kind()
	}
	m := Model{
		source:    opts.Source,
		stream:    opts.Stream,
		caps:      opts.Capabilities,
		cfg:       opts.Config,
		theme:     opts.Theme,
		keyMap:    opts.KeyMap,
		cancel:    opts.Cancel,
		renderer:  render.ForKind(kind, deps),
		streaming: true,
	}
	if opts.Stream != nil && opts.Stream.First.EOF {
		m.streaming = false
	}
	return m
}

// chunkLoadedMsg announces that the loader produced another chunk. The
// model's Update routes it through the buffer (already done by the
// loader internally) and re-renders.
type chunkLoadedMsg struct {
	chunk loader.Chunk
}

// streamDoneMsg is sent when the loader's Updates channel closes.
type streamDoneMsg struct{}
