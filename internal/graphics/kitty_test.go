// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildDeterministicPNG produces a 16×16 RGBA gradient encoded as PNG
// using the standard library's default filter strategy. The output
// byte stream is stable across Go versions that don't change the
// default png.Encoder configuration (which has not happened since
// Go 1.0). Used by every encoder golden test so the three protocols
// share the same input.
func buildDeterministicPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 16),
				G: uint8(y * 16),
				B: uint8((x + y) * 8),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

// loadDeterministicImage decodes the deterministic PNG back into an
// image.Image so we can exercise the encoder against a real image
// (the encoders all call png.Encode internally; passing back the
// decoded image rather than the bytes keeps the test independent of
// the test helper's encoder choice).
func loadDeterministicImage(t *testing.T) image.Image {
	t.Helper()
	pngBytes := buildDeterministicPNG(t)
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	return img
}

func TestEncodeKitty_HasGraphicsPrefixAndTerminator(t *testing.T) {
	out, err := encodeKitty(loadDeterministicImage(t))
	if err != nil {
		t.Fatalf("encodeKitty: %v", err)
	}
	if !strings.HasPrefix(out, "\x1b_G") {
		t.Errorf("kitty output missing \\x1b_G prefix: %q", out[:min(20, len(out))])
	}
	if !strings.HasSuffix(out, "\x1b\\") {
		t.Errorf("kitty output missing \\x1b\\\\ terminator")
	}
	if !strings.Contains(out, "a=T,f=100") {
		t.Errorf("kitty output missing transmit + PNG marker (a=T,f=100)")
	}
}

func TestEncodeKitty_NilImageReturnsError(t *testing.T) {
	_, err := encodeKitty(nil)
	if err == nil {
		t.Errorf("encodeKitty(nil) should return an error")
	}
}

func TestEncodeKitty_ChunkSizeRespects4096B(t *testing.T) {
	// Build a noisy image that PNG can't compress small enough to fit in
	// a single 4096 B base64 chunk. Each chunk's base64 payload must be
	// ≤ 4096 bytes.
	img := noisyImage(t, 512, 512)
	out, err := encodeKitty(img)
	if err != nil {
		t.Fatalf("encodeKitty: %v", err)
	}
	// Walk the chunks: each frame is `\x1b_G…;<base64>\x1b\\`.
	// Split on the terminator and check every frame's base64 portion.
	chunks := strings.Split(out, "\x1b\\")
	chunkCount := 0
	for _, c := range chunks {
		if c == "" {
			continue
		}
		// Chunk shape: `\x1b_G<params>;<base64>`.
		semi := strings.Index(c, ";")
		if semi < 0 {
			t.Errorf("malformed kitty chunk (no semicolon): %q", c)
			continue
		}
		payload := c[semi+1:]
		if len(payload) > kittyChunkSize {
			t.Errorf("chunk payload exceeded %d B: got %d", kittyChunkSize, len(payload))
		}
		if !isBase64(payload) {
			t.Errorf("chunk payload is not valid base64: %q", payload[:min(40, len(payload))])
		}
		chunkCount++
	}
	if chunkCount < 2 {
		t.Errorf("256×256 image should produce ≥ 2 chunks, got %d", chunkCount)
	}
}

func TestEncodeKitty_LastChunkUsesMZero(t *testing.T) {
	// The final chunk in a multi-chunk transmission must carry `m=0`.
	img := noisyImage(t, 512, 512)
	out, err := encodeKitty(img)
	if err != nil {
		t.Fatalf("encodeKitty: %v", err)
	}
	// The last chunk's parameters precede the final `\x1b\\`.
	// We split-then-find-the-last-non-empty chunk.
	chunks := strings.Split(out, "\x1b\\")
	var last string
	for _, c := range chunks {
		if c != "" {
			last = c
		}
	}
	if !strings.Contains(last, "m=0") {
		t.Errorf("last chunk should carry `m=0`, got %q", last)
	}
}

func TestEncodeKitty_SingleChunkOmitsMore(t *testing.T) {
	// A 1×1 image fits in a single chunk; the encoder should omit `m`
	// entirely (Kitty allows that for one-shot transmits).
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	out, err := encodeKitty(img)
	if err != nil {
		t.Fatalf("encodeKitty: %v", err)
	}
	if strings.Contains(out, "m=") {
		t.Errorf("single-chunk image should not emit m=…, got %q", out)
	}
}

// TestEncodeKitty_GoldenPayload encodes the deterministic 16×16 PNG and
// asserts the **complete** escape stream byte-for-byte against the
// pinned goldens. Catches malformed payloads between the `\x1b_G…`
// prefix and `\x1b\\` terminator that look correct under prefix-only
// checks but render as broken images. Per T068b.
func TestEncodeKitty_GoldenPayload(t *testing.T) {
	img := loadDeterministicImage(t)
	got, err := encodeKitty(img)
	if err != nil {
		t.Fatalf("encodeKitty: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "kitty_expected.bin"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("kitty golden mismatch — first divergence at byte %d", firstDiff(got, string(want)))
	}
}

func TestKittyDeleteAllShape(t *testing.T) {
	if kittyDeleteAll != "\x1b_Ga=d,d=A;\x1b\\" {
		t.Errorf("kitty delete-all sequence drift: %q", kittyDeleteAll)
	}
}

func isBase64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

func firstDiff(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// noisyImage produces a `w`×`h` RGBA image filled with a pseudo-random
// byte stream PNG can't meaningfully compress. The seed is fixed so
// output is deterministic across test runs but the result still defeats
// the standard library's filter heuristics, so the encoded PNG is
// big enough to force multi-chunk transmission in the Kitty test.
func noisyImage(t *testing.T, w, h int) image.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// xorshift32 — fast, deterministic, no allocation.
	var state uint32 = 0x9E3779B9
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			state ^= state << 13
			state ^= state >> 17
			state ^= state << 5
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(state),
				G: uint8(state >> 8),
				B: uint8(state >> 16),
				A: 255,
			})
		}
	}
	return img
}
