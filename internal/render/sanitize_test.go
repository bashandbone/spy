// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"testing"
)

// containsRawEscape is a thin alias over the production
// containsRawEscByte helper so the boundary tests in this package
// can share the byte-level check without re-implementing it. We
// deliberately bypass strings.ContainsAny because that helper
// decodes the chars argument as runes — and 0x9b on its own is
// invalid UTF-8 that decodes as U+FFFD, so a source file containing
// a literal U+FFFD would false-positive ContainsAny.
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

// TestNeutralize_ReplacesEscWithQuestionMark pins the post-PR#26
// behavior: ESC (0x1b) and CSI (0x9b) bytes are replaced with the
// single ASCII byte `'?'`. The single-byte choice is load-bearing —
// see TestNeutralize_PreservesByteLength for the invariant tests
// downstream callers depend on.
func TestNeutralize_ReplacesEscWithQuestionMark(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single ESC",
			in:   "\x1b]2;evil\x07",
			want: "?]2;evil\x07",
		},
		{
			name: "single CSI 8-bit",
			in:   "\x9b31m",
			want: "?31m",
		},
		{
			name: "mixed ESC and CSI",
			in:   "before\x1bmid\x9bafter",
			want: "before?mid?after",
		},
		{
			name: "only escapes",
			in:   "\x1b\x9b\x1b",
			want: "???",
		},
		{
			name: "ESC adjacent to multibyte UTF-8",
			in:   "漢\x1b字",
			want: "漢?字",
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
		})
	}
}

// TestNeutralize_PreservesByteLength is the load-bearing invariant
// guard for PR#26: applyMatchHighlights (match.go) computes byte
// offsets against the pre-Neutralize string and slices the
// post-Neutralize string with those offsets. Any change in length
// would misalign the slices and could split a multi-byte UTF-8
// sequence across a highlight boundary.
//
// If you are tempted to switch the replacement to U+FFFD or any
// other multi-byte rune, this test will fail — and so will the
// downstream highlight rendering. Pick a different defense.
func TestNeutralize_PreservesByteLength(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"plain ascii",
		"\x1b",
		"\x9b",
		"\x1b\x9b\x1b\x9b",
		"\x1b]2;evil\x07",
		"漢\x1b字\x9b!",
		"start\x1bmiddle\x9bend",
	}
	for _, in := range cases {
		got := Neutralize(in)
		if len(got) != len(in) {
			t.Errorf("Neutralize(%q): len(out)=%d, len(in)=%d — byte-preservation invariant violated", in, len(got), len(in))
		}
	}
}

// TestNeutralize_ReplacementByteIsAscii pins the substitution byte
// value at exactly `'?'` (0x3f). Guards against an accidental switch
// back to a multi-byte replacement.
func TestNeutralize_ReplacementByteIsAscii(t *testing.T) {
	t.Parallel()
	got := Neutralize("\x1b")
	if got != "?" {
		t.Errorf("Neutralize(ESC) = %q (% x), want %q", got, []byte(got), "?")
	}
	if len(got) != 1 {
		t.Errorf("Neutralize(ESC) length = %d, want 1", len(got))
	}
}
