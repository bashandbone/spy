// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"bytes"
	"errors"
	"image"
	"io"
	"strings"
	"testing"

	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// TestImageDecoder_RecoversFromPanic exercises the deferred recover
// guard around image.Decode — a hostile / corrupt blob that triggers
// a panic in any registered image decoder must NOT terminate spy;
// the renderer must return ErrUnsupportedDecoder so the calling
// renderer falls back to the metadata block.
//
// We can't easily force a panic in a stdlib decoder from outside, so
// we use a panicReader: an io.Reader that panics on Read. PNG/JPEG
// decoders end up reading from this and the panic propagates through
// image.Decode — the recover in imageRenderer.decode catches it.
func TestImageDecoder_RecoversFromPanic(t *testing.T) {
	src := &panicSource{name: "evil.png", panicMsg: "synthetic decoder panic"}
	r := &imageRenderer{src: src}

	_, err := r.decode()
	if err == nil {
		t.Fatalf("decode of panicking source returned nil error — recover did not fire")
	}
	if !errors.Is(err, ErrUnsupportedDecoder) {
		t.Errorf("decode error is not ErrUnsupportedDecoder: got %v", err)
	}
	if !strings.Contains(err.Error(), "synthetic decoder panic") {
		t.Errorf("decode error does not surface panic message: got %v", err)
	}
}

// TestImageDecoder_PanicDoesNotCorruptCallSite verifies that even
// when decode panics, the metadata-block fallback path still
// produces a deterministic frame so the user sees something sensible
// instead of a torn-down alt-screen. We pass a graphics-capable
// Capabilities value so renderFresh enters the decode path; the
// recover-fallback then routes back through metadataBlock.
func TestImageDecoder_PanicDoesNotCorruptCallSite(t *testing.T) {
	src := &panicSource{name: "evil.png", panicMsg: "boom"}
	r := newImageRenderer(Dependencies{}, src)
	caps := term.Capabilities{Graphics: term.GraphicsKitty, Cols: 80, Rows: 24}
	frame := r.Render(RenderContext{Capabilities: caps})
	if !strings.Contains(frame, "evil.png") {
		t.Errorf("metadata-block fallback after decoder panic missing filename; got %q", frame)
	}
	if frame == "" {
		t.Errorf("metadata-block fallback after decoder panic returned empty string")
	}
}

// panicSource is a source.Source whose Open() returns an io.Reader
// that panics on the first Read. Used to drive a panic through
// image.Decode without depending on a specific decoder bug.
type panicSource struct {
	name     string
	panicMsg string
}

func (p *panicSource) DisplayName() string { return p.name }

func (p *panicSource) Kind() source.Kind { return source.KindImage }

func (p *panicSource) Metadata() source.Metadata {
	return source.Metadata{Path: p.name, Size: 0}
}

func (p *panicSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(&panicReader{msg: p.panicMsg}), nil
}

func (p *panicSource) Reopen() (io.ReadSeeker, error) {
	// Image decode path uses Open; Reopen is only consulted by PDF.
	return bytes.NewReader(nil), nil
}

// panicReader panics from Read with a controlled message.
type panicReader struct{ msg string }

func (p *panicReader) Read(_ []byte) (int, error) { panic(p.msg) }

// _ keeps the image import live; imageRenderer.decode is the trust
// boundary for these tests and a future panic-test addition may
// need image.Image directly.
var _ image.Image = (image.Image)(nil)
