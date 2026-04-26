// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"strings"
)

// containsRawEscByte reports whether s carries a raw ESC (0x1b) or
// CSI (0x9b) byte. Distinct from strings.ContainsAny because that
// helper decodes the chars argument as runes — and U+FFFD (the
// replacement Neutralize substitutes in) is also the fallback rune
// for the invalid byte 0x9b on its own, which gives false positives
// once neutralisation has already run.
func containsRawEscByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == 0x1b || c == 0x9b {
			return true
		}
	}
	return false
}

// Neutralize replaces every ESC (0x1b) and CSI (0x9b) byte in `s`
// with the Unicode replacement character U+FFFD (`�`). Each
// replacement byte expands to the 3-byte UTF-8 encoding (EF BF BD),
// so the returned string is longer than the input when escapes are
// present. Terminal-cell width math and search-match offsets that
// use the original Raw bytes stay valid because we never reach this
// path for offsets — only for human-visible emit boundaries.
//
// Why: a file whose content includes `\x1b]2;malicious\x07` — the OSC 2
// "set window title" sequence — would, if rendered verbatim, change the
// user's terminal title (or worse, with longer DCS / OSC payloads).
// Neutralising at every emit boundary gives the user a faithful
// representation of the file's bytes (the offsets, line numbers, and
// search positions are unchanged) while removing the active payload.
//
// The trade-off: files that intentionally contain ANSI colour escapes
// (e.g., output from `git diff --color=always` redirected to disk) lose
// their colour. That is an acceptable v0.1.0 default; a future
// `--allow-ansi-passthrough` flag can opt back in once the
// SGR-but-not-OSC discriminator is wired in.
//
// Exported because cmd/spy uses it to sanitise file paths before
// stderr error writes — every byte that reaches the user's terminal
// is funnelled through here when it could carry attacker-controlled
// content (filename, file body, PDF text-extraction output, status-bar
// advisories).
//
// Cross-references:
//
//   - Spec T109b.c "Terminal escape injection from file content"
//   - Acceptance review C4 "neutralizeEscapes bypassed in markdown / PDF /
//     statusbar / image / stderr paths"
//   - Acceptance review LOW-1 "use U+FFFD instead of '?'"
//   - tests/integration/escape_injection_test.go
func Neutralize(s string) string {
	if !containsRawEscByte(s) {
		return s
	}
	var b strings.Builder
	// Pre-size: each escape byte becomes 3 bytes; common case has only
	// a handful of escapes so the rough upper bound (len(s)+12) is
	// cheaper than counting escapes first.
	b.Grow(len(s) + 12)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case 0x1b, 0x9b:
			b.WriteRune('�')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// neutralizeEscapes is the unexported alias kept for the existing
// in-package call sites that prefer the historical name. New code
// should call [Neutralize] directly.
func neutralizeEscapes(s string) string { return Neutralize(s) }
