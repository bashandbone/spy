// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// fakeImageSource is a [source.Source] that yields the provided PNG
// bytes without touching the filesystem. Used by the image-renderer
// tests so they don't rely on a real file fixture.
type fakeImageSource struct {
	name  string
	bytes []byte
	path  string
	size  int64
}

func (f *fakeImageSource) Kind() source.Kind   { return source.KindImage }
func (f *fakeImageSource) DisplayName() string { return f.name }
func (f *fakeImageSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.bytes)), nil
}
func (f *fakeImageSource) Reopen() (io.ReadSeeker, error) { return bytes.NewReader(f.bytes), nil }
func (f *fakeImageSource) Metadata() source.Metadata {
	return source.Metadata{
		Path:     f.path,
		Size:     f.size,
		Modified: time.Time{},
	}
}

func deterministicPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 32), G: uint8(y * 32), B: 64, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func newFakeImageSource(t *testing.T, name string) *fakeImageSource {
	t.Helper()
	body := deterministicPNG(t)
	return &fakeImageSource{name: name, bytes: body, path: name, size: int64(len(body))}
}

func TestImageRenderer_GraphicsCapableEmitsProtocolBytes(t *testing.T) {
	src := newFakeImageSource(t, "demo.png")
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindImage, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty},
	})
	if !strings.HasPrefix(out, "\x1b_G") {
		t.Errorf("Kitty path should emit graphics protocol bytes, got %q", out[:min(40, len(out))])
	}
	if !strings.HasSuffix(out, "\x1b\\") {
		t.Errorf("Kitty path missing terminator")
	}
}

func TestImageRenderer_NonCapableShowsMetadata(t *testing.T) {
	src := newFakeImageSource(t, "demo.png")
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindImage, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsNone},
	})
	for _, want := range []string{"[image: demo.png]", "dimensions: 8×8", "size:", "fallback:"} {
		if !strings.Contains(out, want) {
			t.Errorf("metadata block missing %q in %q", want, out)
		}
	}
}

func TestImageRenderer_DecodeFailureFallsBack(t *testing.T) {
	src := &fakeImageSource{
		name: "bogus.png", bytes: []byte("not a real image"), path: "bogus.png", size: 16,
	}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindImage, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty},
	})
	if !strings.Contains(out, "[image: bogus.png]") {
		t.Errorf("expected metadata fallback header, got %q", out)
	}
	if !strings.Contains(out, "decode failed") {
		t.Errorf("expected decode-failed note, got %q", out)
	}
}

func TestImageRenderer_OpenFailureFallsBack(t *testing.T) {
	src := &errImageSource{name: "lockedout.png"}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := ForKind(source.KindImage, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty},
	})
	if !strings.Contains(out, "[image: lockedout.png]") {
		t.Errorf("expected metadata header, got %q", out)
	}
}

func TestImageRenderer_NilSourceProducesPlaceholder(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark()}
	r := ForKind(source.KindImage, deps)
	out := r.Render(RenderContext{
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty},
	})
	if !strings.Contains(out, "no source attached") {
		t.Errorf("expected nil-source placeholder, got %q", out)
	}
}

func TestImageRenderer_RowToLineAlwaysZero(t *testing.T) {
	deps := Dependencies{Theme: ThemeDark(), Source: newFakeImageSource(t, "x.png")}
	r := ForKind(source.KindImage, deps)
	if r.RowToLine(RenderContext{}, 0) != 0 {
		t.Errorf("RowToLine should return 0 for image kind")
	}
	if r.RowToLine(RenderContext{}, 9999) != 0 {
		t.Errorf("RowToLine should return 0 for any row")
	}
}

func TestImageRenderer_CacheReusesEncodedFrame(t *testing.T) {
	src := newFakeImageSource(t, "cached.png")
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := newImageRenderer(deps, src)
	caps := term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty}
	out1 := r.Render(RenderContext{Capabilities: caps})
	out2 := r.Render(RenderContext{Capabilities: caps})
	if out1 != out2 {
		t.Errorf("re-rendered frame should be identical when caps unchanged")
	}
}

// countingImageSource wraps a source body but records how many times
// Open / Reopen has been invoked so the negative-cache test can assert
// that a failing decode path doesn't keep re-opening the file on every
// render (Copilot review PR#11 round-3).
type countingImageSource struct {
	name      string
	body      []byte
	openCalls int
}

func (c *countingImageSource) Kind() source.Kind { return source.KindImage }
func (c *countingImageSource) DisplayName() string {
	return c.name
}
func (c *countingImageSource) Open() (io.ReadCloser, error) {
	c.openCalls++
	return io.NopCloser(bytes.NewReader(c.body)), nil
}
func (c *countingImageSource) Reopen() (io.ReadSeeker, error) {
	c.openCalls++
	return bytes.NewReader(c.body), nil
}
func (c *countingImageSource) Metadata() source.Metadata {
	return source.Metadata{Path: c.name, Size: int64(len(c.body))}
}

func TestImageRenderer_FailedDecodeIsCached(t *testing.T) {
	// First render fails decode and produces the metadata fallback;
	// subsequent renders must hit the cache instead of re-opening +
	// re-decoding the broken bytes (Copilot review PR#11 round-3).
	src := &countingImageSource{name: "bogus.png", body: []byte("not a real image")}
	deps := Dependencies{Theme: ThemeDark(), Source: src}
	r := newImageRenderer(deps, src)
	caps := term.Capabilities{Cols: 80, Rows: 24, Graphics: term.GraphicsKitty}
	out1 := r.Render(RenderContext{Capabilities: caps})
	openAfterFirst := src.openCalls
	out2 := r.Render(RenderContext{Capabilities: caps})
	out3 := r.Render(RenderContext{Capabilities: caps})
	if out1 != out2 || out2 != out3 {
		t.Errorf("failed-decode fallback should be stable across renders")
	}
	if src.openCalls != openAfterFirst {
		t.Errorf("subsequent renders should not re-open the source: got %d additional opens",
			src.openCalls-openAfterFirst)
	}
}

func TestImageRenderer_DimensionsHelperHandlesError(t *testing.T) {
	r := newImageRenderer(Dependencies{Theme: ThemeDark()}, &errImageSource{name: "x.png"})
	if got := r.dimensions(); got != "" {
		t.Errorf("dimensions() on failing source: got %q want empty", got)
	}
}

// errImageSource always errors on Open / Reopen so we can exercise the
// fallback paths.
type errImageSource struct{ name string }

func (e *errImageSource) Kind() source.Kind              { return source.KindImage }
func (e *errImageSource) DisplayName() string            { return e.name }
func (e *errImageSource) Open() (io.ReadCloser, error)   { return nil, os.ErrNotExist }
func (e *errImageSource) Reopen() (io.ReadSeeker, error) { return nil, os.ErrNotExist }
func (e *errImageSource) Metadata() source.Metadata {
	return source.Metadata{Path: filepath.Join(".", e.name)}
}
