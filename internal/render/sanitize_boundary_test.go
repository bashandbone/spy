// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
)

// The tests below pin the acceptance-review C4 contract: every emit
// boundary that can carry user-controlled bytes (markdown content,
// PDF text extraction, image / PDF / status-bar DisplayName) MUST
// neutralize ESC (0x1b) and CSI (0x9b) bytes before they reach the
// terminal. Without this guard, a hostile filename or content blob
// like `\x1b]2;evil\x07` drives the terminal title (or worse).

const oscPayload = "\x1b]2;evil\x07"
const oscPayloadShort = "\x1b]2;evil"

// TestC4_StatusBar_NeutralizesDisplayNameAndAdvisory verifies that
// hostile bytes in StatusInput.DisplayName and StatusInput.Advisory
// are stripped before reaching the rendered string.
func TestC4_StatusBar_NeutralizesDisplayNameAndAdvisory(t *testing.T) {
	in := StatusInput{
		DisplayName: "evil" + oscPayload + ".txt",
		Meta:        source.Metadata{LineCount: 10},
		Width:       100,
		Current:     1,
		Advisory:    "warning" + oscPayload,
		Mono:        true, // bypass lipgloss theme styling for clean assertion
	}
	out := StatusBarRender(in, Theme{})
	if strings.ContainsAny(out, "\x1b\x9b") {
		t.Errorf("status bar emitted ESC/CSI byte from DisplayName/Advisory:\n  %q", out)
	}
	if !strings.Contains(out, "evil") {
		t.Errorf("status bar dropped surrounding (benign) DisplayName content; got %q", out)
	}
	if !strings.Contains(out, "warning") {
		t.Errorf("status bar dropped surrounding (benign) Advisory content; got %q", out)
	}
}

// TestC4_StatusBar_CollapsedAlsoNeutralizes covers the sub-80-col
// branch — StatusBarRender entry-point neutralization runs before
// the wide/collapsed split, so both code paths are protected.
func TestC4_StatusBar_CollapsedAlsoNeutralizes(t *testing.T) {
	in := StatusInput{
		DisplayName: "evil" + oscPayload + ".txt",
		Width:       60,
		Current:     1,
		Mono:        true,
	}
	out := StatusBarRender(in, Theme{})
	if strings.ContainsAny(out, "\x1b\x9b") {
		t.Errorf("collapsed status bar emitted ESC/CSI byte:\n  %q", out)
	}
}

// TestC4_PDF_FormatTextPage_NeutralizesText verifies the PDF text
// extraction path doesn't leak hostile bytes from the document body.
// formatTextPage is the only render path for non-fitz builds.
func TestC4_PDF_FormatTextPage_NeutralizesText(t *testing.T) {
	r := &pdfRenderer{src: &fakeC4Source{name: "doc.pdf"}}
	out := r.formatTextPage("benign content with "+oscPayload+" inside", 1, 3)
	if strings.ContainsAny(out, "\x1b\x9b") {
		t.Errorf("formatTextPage leaked ESC/CSI from PDF body:\n  %q", out)
	}
	if !strings.Contains(out, "benign content with") {
		t.Errorf("formatTextPage dropped surrounding content; got %q", out)
	}
}

// TestC4_PDF_FormatTextPage_NeutralizesDisplayName covers the "hostile
// filename" half: the source name is emitted into the page header.
func TestC4_PDF_FormatTextPage_NeutralizesDisplayName(t *testing.T) {
	r := &pdfRenderer{src: &fakeC4Source{name: "evil" + oscPayload + ".pdf"}}
	out := r.formatTextPage("body", 1, 1)
	if strings.ContainsAny(out, "\x1b\x9b") {
		t.Errorf("formatTextPage leaked ESC/CSI from DisplayName:\n  %q", out)
	}
}

// TestC4_PDF_MetadataBlock_NeutralizesDisplayName covers the
// graphics-unavailable / fitz-disabled fallback path.
func TestC4_PDF_MetadataBlock_NeutralizesDisplayName(t *testing.T) {
	r := &pdfRenderer{src: &fakeC4Source{name: "evil" + oscPayload + ".pdf", size: 1024}}
	out := r.metadataBlock("note text "+oscPayload, 1, 1)
	if strings.ContainsAny(out, "\x1b\x9b") {
		t.Errorf("PDF metadata block leaked ESC/CSI:\n  %q", out)
	}
}

// TestC4_Image_MetadataBlock_NeutralizesDisplayName covers the
// no-graphics image fallback path.
func TestC4_Image_MetadataBlock_NeutralizesDisplayName(t *testing.T) {
	r := newImageRenderer(Dependencies{}, &fakeC4Source{name: "evil" + oscPayload + ".png", size: 4096})
	out := r.metadataBlock(RenderContext{}, "")
	if strings.ContainsAny(out, "\x1b\x9b") {
		t.Errorf("image metadata block leaked ESC/CSI:\n  %q", out)
	}
}

// TestC4_Markdown_AssembleRaw_NeutralizesContent covers the goldmark
// pre-render boundary. Glamour does NOT strip ESC bytes from
// non-code-block content, so the renderer must do it before handoff.
func TestC4_Markdown_AssembleRaw_NeutralizesContent(t *testing.T) {
	lines := []source.Line{
		{Number: 1, Raw: "# Header"},
		{Number: 2, Raw: "evil" + oscPayloadShort + " body"},
	}
	out := assembleRaw(lines)
	if strings.ContainsAny(out, "\x1b\x9b") {
		t.Errorf("assembleRaw leaked ESC/CSI bytes:\n  %q", out)
	}
	if !strings.Contains(out, "evil") {
		t.Errorf("assembleRaw dropped surrounding content; got %q", out)
	}
}

// TestC4_Markdown_Render_DoesNotLeakInOutput is the end-to-end
// markdownRenderer assertion: even with Glamour in the pipeline,
// the final rendered string must not carry the hostile bytes.
func TestC4_Markdown_Render_DoesNotLeakInOutput(t *testing.T) {
	hostile := "evil" + oscPayloadShort
	buf := loader.NewLineBuffer(0, 0, &fakeC4Source{name: "doc.md"})
	buf.Append([]source.Line{
		{Number: 1, Raw: "# title"},
		{Number: 2, Raw: hostile},
	})
	buf.MarkComplete(2)

	r := newMarkdownRenderer(Dependencies{
		Theme: Theme{Name: "dark"},
	})
	ctx := RenderContext{
		Buffer:   buf,
		Viewport: viewport.New(80, 24),
	}
	out := r.Render(ctx)
	if strings.ContainsAny(out, "\x1b\x9b") {
		// Glamour will emit SGR escapes (\x1b[...m) for styling — those
		// are LEGITIMATE and welcome. The C4 contract is specifically
		// about NON-CSI escapes (OSC/DCS/etc) leaking through. So we
		// scan for `\x1b]` (OSC start) and `\x1b]` precursors plus
		// `\x9b` (8-bit CSI which the renderer should never emit).
		// A bare `\x1b[…m` is allowed.
		if strings.Contains(out, "\x1b]") || strings.Contains(out, "\x9b") {
			t.Errorf("markdown render leaked OSC / 8-bit CSI from hostile content:\n  %q", out)
		}
	}
}

// fakeC4Source is a minimal source.Source implementation for the C4
// boundary tests — DisplayName / Metadata are the only methods these
// tests exercise.
type fakeC4Source struct {
	name string
	size int64
}

func (f *fakeC4Source) Kind() source.Kind   { return source.KindPDF }
func (f *fakeC4Source) DisplayName() string { return f.name }
func (f *fakeC4Source) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (f *fakeC4Source) Reopen() (io.ReadSeeker, error) { return bytes.NewReader(nil), nil }
func (f *fakeC4Source) Metadata() source.Metadata {
	return source.Metadata{Path: f.name, Size: f.size}
}
