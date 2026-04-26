// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

// containsRawEscByte reports whether s carries a raw ESC (0x1b) or
// CSI (0x9b) byte. Distinct from strings.ContainsAny because that
// helper decodes the chars argument as runes — and 0x9b on its own
// is invalid UTF-8 that decodes as U+FFFD, so any source file
// containing a literal U+FFFD would false-positive ContainsAny.
// Walking bytes directly avoids that decode entirely.
func containsRawEscByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == 0x1b || c == 0x9b {
			return true
		}
	}
	return false
}

// Neutralize replaces every ESC (0x1b) and CSI (0x9b) byte in `s`
// with the ASCII byte `'?'`. The substitution is byte-for-byte by
// design: len(Neutralize(s)) == len(s) and every byte position other
// than the replaced one is unchanged. Several callers (notably
// [applyMatchHighlights] in match.go) compute byte offsets against
// the pre-neutralized string and slice the post-neutralized string
// using those offsets — any change in length would misalign the
// slices and could split a multi-byte UTF-8 sequence across a
// highlight boundary.
//
// Why: a file whose content includes `\x1b]2;malicious\x07` — the OSC 2
// "set window title" sequence — would, if rendered verbatim, change the
// user's terminal title (or worse, with longer DCS / OSC payloads).
// Neutralizing at every emit boundary gives the user a faithful
// representation of the file's bytes (the offsets, line numbers, and
// search positions are unchanged) while removing the active payload.
//
// The trade-off: files that intentionally contain ANSI color escapes
// (e.g., output from `git diff --color=always` redirected to disk) lose
// their color. That is an acceptable v0.1.0 default; a future
// `--allow-ansi-passthrough` flag can opt back in once the
// SGR-but-not-OSC discriminator is wired in.
//
// On the choice of `'?'` over U+FFFD (the "conventional" Unicode
// replacement): U+FFFD encodes to 3 UTF-8 bytes, which violates the
// length-preservation invariant that callers depend on (PR#26 review).
// `'?'` is a valid trade-off — slight ambiguity with literal question
// marks in source files vs. correct match-highlight offsets across
// every call site.
//
// Exported because cmd/spy uses it to sanitize file paths before
// stderr error writes — every byte that reaches the user's terminal
// is funneled through here when it could carry attacker-controlled
// content (filename, file body, PDF text-extraction output, status-bar
// advisories).
//
// Cross-references:
//
//   - Spec T109b.c "Terminal escape injection from file content"
//   - Acceptance review C4 "neutralizeEscapes bypassed in markdown / PDF /
//     statusbar / image / stderr paths"
//   - Acceptance review LOW-1 (closed WONTFIX in PR#26 review: U+FFFD
//     proposal violates the byte-preservation invariant on which
//     match.go applyMatchHighlights depends)
//   - internal/render/match.go applyMatchHighlights (depends on
//     byte-for-byte invariant)
//   - tests/integration/escape_injection_test.go
func Neutralize(s string) string {
	if !containsRawEscByte(s) {
		return s
	}
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case 0x1b, 0x9b:
			b[i] = '?'
		}
	}
	return string(b)
}

// neutralizeEscapes is the unexported alias kept for the existing
// in-package call sites that prefer the historical name. New code
// should call [Neutralize] directly.
func neutralizeEscapes(s string) string { return Neutralize(s) }
