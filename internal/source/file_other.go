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
// creation on Windows already requires elevated privileges so the
// attack surface is materially smaller, but consumers should treat this
// as a documented limitation rather than a guarantee (Copilot review
// acceptance M2).
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
