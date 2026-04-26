// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncodeITerm2_HasInlineMarkers(t *testing.T) {
	out, err := encodeITerm2(loadDeterministicImage(t))
	if err != nil {
		t.Fatalf("encodeITerm2: %v", err)
	}
	if !strings.HasPrefix(out, "\x1b]1337;File=inline=1;preserveAspectRatio=1:") {
		t.Errorf("iterm2 output missing OSC 1337 inline prefix")
	}
	if !strings.HasSuffix(out, "\x07") {
		t.Errorf("iterm2 output missing BEL terminator")
	}
}

func TestEncodeITerm2_NilImageReturnsError(t *testing.T) {
	_, err := encodeITerm2(nil)
	if err == nil {
		t.Errorf("encodeITerm2(nil) should return an error")
	}
}

func TestEncodeITerm2_PayloadIsBase64(t *testing.T) {
	out, err := encodeITerm2(loadDeterministicImage(t))
	if err != nil {
		t.Fatalf("encodeITerm2: %v", err)
	}
	colon := strings.Index(out, ":")
	if colon < 0 {
		t.Fatalf("iterm2 output missing payload separator")
	}
	body := strings.TrimSuffix(out[colon+1:], "\x07")
	if _, err := base64.StdEncoding.DecodeString(body); err != nil {
		t.Errorf("iterm2 payload is not valid base64: %v", err)
	}
}

// TestEncodeITerm2_GoldenPayload covers T069's full-payload golden
// requirement against the same deterministic input PNG used by the
// Kitty golden (T068b).
func TestEncodeITerm2_GoldenPayload(t *testing.T) {
	got, err := encodeITerm2(loadDeterministicImage(t))
	if err != nil {
		t.Fatalf("encodeITerm2: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "iterm2_expected.bin"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("iterm2 golden mismatch — first divergence at byte %d", firstDiff(got, string(want)))
	}
}
