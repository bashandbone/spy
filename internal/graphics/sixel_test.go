// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncodeSixel_StartsWithDCS(t *testing.T) {
	// All sixel transmissions start with the Device Control String
	// (DCS) introducer `\x1bP` and end with the String Terminator `\x1b\\`.
	out, err := encodeSixel(loadDeterministicImage(t))
	if err != nil {
		t.Fatalf("encodeSixel: %v", err)
	}
	if !strings.HasPrefix(out, "\x1bP") {
		t.Errorf("sixel output missing DCS introducer")
	}
	if !strings.HasSuffix(out, "\x1b\\") {
		t.Errorf("sixel output missing String Terminator")
	}
}

func TestEncodeSixel_NilImageReturnsError(t *testing.T) {
	_, err := encodeSixel(nil)
	if err == nil {
		t.Errorf("encodeSixel(nil) should return an error")
	}
}

func TestEncodeSixel_NonEmpty(t *testing.T) {
	out, err := encodeSixel(loadDeterministicImage(t))
	if err != nil {
		t.Fatalf("encodeSixel: %v", err)
	}
	// Sixel encoder output for a 16×16 image is non-trivial — the
	// envelope alone is 4 bytes, but a real raster takes hundreds.
	if len(out) < 50 {
		t.Errorf("sixel output suspiciously short: %d bytes", len(out))
	}
}

// TestEncodeSixel_GoldenPayload pins the byte-for-byte sixel output for
// the deterministic 16×16 input. Sixel output may vary across go-sixel
// versions; the regen procedure is documented in
// internal/graphics/testdata/README.md.
func TestEncodeSixel_GoldenPayload(t *testing.T) {
	got, err := encodeSixel(loadDeterministicImage(t))
	if err != nil {
		t.Fatalf("encodeSixel: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "sixel_expected.bin"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("sixel golden mismatch — first divergence at byte %d (%d != %d bytes)",
			firstDiff(got, string(want)), len(got), len(want))
	}
}
