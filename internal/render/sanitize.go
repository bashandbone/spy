// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import "strings"

// neutralizeEscapes replaces every ESC (0x1b) and CSI (0x9b) byte in
// `s` with the printable replacement character '?'. The substitution
// is byte-for-byte so terminal-cell width math and search-match offsets
// computed against the original Raw bytes stay valid.
//
// Why: a file whose content includes `\x1b]2;malicious\x07` — the OSC 2
// "set window title" sequence — would, if rendered verbatim, change the
// user's terminal title (or worse, with longer DCS / OSC payloads).
// Neutralising at the renderer boundary gives the user a faithful
// representation of the file's bytes (the offsets, line numbers, and
// search positions are unchanged) while removing the active payload.
//
// The trade-off: files that intentionally contain ANSI colour escapes
// (e.g., output from `git diff --color=always` redirected to disk) lose
// their colour. That is an acceptable v0.1.0 default; a future
// `--allow-ansi-passthrough` flag can opt back in once the
// SGR-but-not-OSC discriminator is wired in.
//
// Cross-references:
//
//   - Spec T109b.c "Terminal escape injection from file content"
//   - tests/integration/escape_injection_test.go
func neutralizeEscapes(s string) string {
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
