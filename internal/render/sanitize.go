// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import "strings"

// Neutralize replaces every ESC (0x1b) and CSI (0x9b) byte in `s`
// with the printable replacement character '?'. The substitution is
// byte-for-byte so terminal-cell width math and search-match offsets
// computed against the original Raw bytes stay valid.
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
//   - tests/integration/escape_injection_test.go
func Neutralize(s string) string {
	if !strings.ContainsAny(s, "\x1b\x9b") {
		return s
	}
	b := []byte(s)
	for i, c := range b {
		switch c {
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
