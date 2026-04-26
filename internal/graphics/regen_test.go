// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build graphics_regen

package graphics

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRegenerateGoldens rewrites every `*_expected.bin` golden in
// testdata/ from the current encoder output. Build-tag-gated so it
// never runs in CI; the regen procedure is documented in
// testdata/README.md.
//
// Run with: go test -tags graphics_regen ./internal/graphics/...
func TestRegenerateGoldens(t *testing.T) {
	dir := "testdata"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}

	pngBytes := buildDeterministicPNG(t)
	if err := os.WriteFile(filepath.Join(dir, "kitty_input.png"), pngBytes, 0o644); err != nil {
		t.Fatalf("write kitty_input.png: %v", err)
	}

	img := loadDeterministicImage(t)

	kitty, err := encodeKitty(img)
	if err != nil {
		t.Fatalf("encodeKitty: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kitty_expected.bin"), []byte(kitty), 0o644); err != nil {
		t.Fatalf("write kitty_expected.bin: %v", err)
	}

	iterm, err := encodeITerm2(img)
	if err != nil {
		t.Fatalf("encodeITerm2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iterm2_expected.bin"), []byte(iterm), 0o644); err != nil {
		t.Fatalf("write iterm2_expected.bin: %v", err)
	}

	six, err := encodeSixel(img)
	if err != nil {
		t.Fatalf("encodeSixel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sixel_expected.bin"), []byte(six), 0o644); err != nil {
		t.Fatalf("write sixel_expected.bin: %v", err)
	}
	t.Logf("regenerated 4 golden files in %s", dir)
}
