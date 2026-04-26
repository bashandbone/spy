// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"strings"
	"testing"
)

// containsRawEscape is a thin alias over the production
// containsRawEscByte helper so the boundary tests in this package
// can share the byte-level check without re-implementing it. We
// deliberately bypass strings.ContainsAny because that helper
// decodes the chars argument as runes — and U+FFFD (the
// replacement Neutralize substitutes in) is also the fallback rune
// for the invalid byte 0x9b on its own, so ContainsAny(out,
// "\x1b\x9b") gives a false positive once neutralisation has run.
func containsRawEscape(s string) bool { return containsRawEscByte(s) }

// TestNeutralize_PassthroughBenign verifies the fast path: strings
// without ESC/CSI bytes are returned unchanged (same byte-for-byte
// content, no allocation forced).
func TestNeutralize_PassthroughBenign(t *testing.T) {
	t.Parallel()

	for _, tc := range []string{
		"",
		"plain ascii",
		"unicode: αβγ — 漢字 🎉",
		"\t\n\r",
	} {
		got := Neutralize(tc)
		if got != tc {
			t.Errorf("Neutralize(%q) = %q, want passthrough", tc, got)
		}
	}
}

// TestNeutralize_ReplacesEscWithFFFD pins LOW-1: ESC (0x1b) and CSI
// (0x9b) bytes are replaced with the Unicode replacement character
// U+FFFD (encoded as 3 UTF-8 bytes EF BF BD), not a single '?'.
func TestNeutralize_ReplacesEscWithFFFD(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single ESC",
			in:   "\x1b]2;evil\x07",
			want: "�]2;evil\x07",
		},
		{
			name: "single CSI 8-bit",
			in:   "\x9b31m",
			want: "�31m",
		},
		{
			name: "mixed ESC and CSI",
			in:   "before\x1bmid\x9bafter",
			want: "before�mid�after",
		},
		{
			name: "only escapes",
			in:   "\x1b\x9b\x1b",
			want: "���",
		},
		{
			name: "ESC adjacent to multibyte UTF-8",
			in:   "漢\x1b字",
			want: "漢�字",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Neutralize(tc.in)
			if got != tc.want {
				t.Errorf("Neutralize(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Defense in depth: result must contain neither raw byte.
			if containsRawEscape(got) {
				t.Errorf("Neutralize(%q) leaked ESC/CSI: %q", tc.in, got)
			}
			// The replacement bytes are exactly the 3-byte UTF-8 of U+FFFD.
			if !strings.Contains(got, "�") {
				t.Errorf("Neutralize(%q) = %q; missing U+FFFD replacement", tc.in, got)
			}
		})
	}
}

// TestNeutralize_ReplacementByteEncoding verifies the replacement
// character is exactly the 3-byte UTF-8 sequence EF BF BD (U+FFFD)
// — guards against an accidental switch to a single-byte placeholder.
func TestNeutralize_ReplacementByteEncoding(t *testing.T) {
	t.Parallel()
	got := Neutralize("\x1b")
	want := []byte{0xEF, 0xBF, 0xBD}
	if got != string(want) {
		t.Errorf("Neutralize(ESC) = % x, want % x (U+FFFD)", []byte(got), want)
	}
}
