// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !unix

package source

import "os"

// openNoFollow on non-Unix platforms (notably Windows) cannot use
// O_NOFOLLOW because the syscall constant isn't defined. We fall back
// to a plain [os.Open]. The TOCTOU window between [filepath.EvalSymlinks]
// and this call is therefore not closed on Windows; symbolic-link
// creation on Windows MAY require elevated privileges depending on
// system configuration (for example, Developer Mode or policy
// settings), so the attack surface may be materially smaller — but
// consumers should treat this as a documented limitation rather than
// a guarantee (Copilot review acceptance M2; PR#24 review).
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
