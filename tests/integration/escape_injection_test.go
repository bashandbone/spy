// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
	"github.com/knitli/spy/internal/ui"
)

// TestEscapeInjection_OSCSequenceNeutralized is the SC-T109b.c gate:
// a file whose bytes contain `\x1b]2;malicious\x07` (the OSC 2
// "set window title" sequence) must NOT have those bytes survive
// through to the rendered output. The renderer neutralizes ESC bytes
// at emission boundaries (see internal/render/sanitize.go).
//
// The test exercises the model directly so the assertion is on the
// in-memory frame rather than a PTY-captured byte stream — that
// distinction matters: the in-memory frame still carries Chroma's
// SGR escapes (those are produced BY the renderer), and we only want
// to assert that USER-supplied ESCs don't leak through.
func TestEscapeInjection_OSCSequenceNeutralized(t *testing.T) {
	const malicious = "\x1b]2;hijacked title\x07"
	body := "before " + malicious + " after\nharmless\n"
	src := &injectionMemSource{body: body, kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	for range stream.Updates {
	}
	DrainStreamErrs(t, stream.Errs)
	cfg := config.Defaults()
	caps := term.Capabilities{Cols: 80, Rows: 24}
	hl := highlight.New(nil, term.ColorTrueColor, 5*1024*1024)
	m := ui.NewModel(ui.ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: caps,
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  hl,
	})
	mu, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := mu.(ui.Model).View().Content

	// The rendered frame must NOT contain the OSC payload bytes
	// (`\x1b]` followed by `2;`). The benign suffix "hijacked title"
	// is fine — it's just ASCII; the security property is that the
	// active control byte (`\x1b`) is gone.
	if strings.Contains(view, "\x1b]2;") {
		t.Errorf("rendered frame contains live OSC payload — escape was not neutralized; view tail=%q",
			view[max(0, len(view)-200):])
	}
	if !strings.Contains(view, "before") || !strings.Contains(view, "after") {
		t.Errorf("escape neutralization dropped surrounding content; view=%q", view)
	}
	// No `\x1b]` sequences at all (DCS / OSC / SOS) — only SGR
	// escapes (`\x1b[...m`) from the renderer's chroma color are
	// allowed. Walk the frame and assert any `\x1b` is followed by
	// `[` (CSI introducer) and nothing more dangerous.
	for i := 0; i < len(view); i++ {
		if view[i] != 0x1b {
			continue
		}
		if i+1 >= len(view) {
			t.Errorf("dangling ESC at offset %d", i)
			break
		}
		next := view[i+1]
		if next != '[' {
			t.Errorf("ESC at offset %d followed by %q (only CSI '[' allowed in rendered frames)", i, next)
			break
		}
	}
}

// injectionMemSource adapts a string body into a [source.Source]
// without touching the filesystem.
type injectionMemSource struct {
	body string
	kind source.Kind
}

func (m *injectionMemSource) Kind() source.Kind   { return m.kind }
func (m *injectionMemSource) DisplayName() string { return "evil.txt" }
func (m *injectionMemSource) Metadata() source.Metadata {
	return source.Metadata{Path: "evil.txt", LineCount: -1}
}
func (m *injectionMemSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(m.body)), nil
}
func (m *injectionMemSource) Reopen() (io.ReadSeeker, error) {
	return strings.NewReader(m.body), nil
}
