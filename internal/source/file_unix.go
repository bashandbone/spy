// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build unix

package source

import (
	"os"
	"syscall"
)

// openNoFollow opens path read-only with [syscall.O_NOFOLLOW] so a
// symlink that was swapped in between [filepath.EvalSymlinks] and this
// call cannot redirect us to a different inode. Without O_NOFOLLOW the
// kernel would silently dereference the new target, defeating the
// stat()-then-read validation in FileSource (Copilot review
// acceptance M2).
//
// We also pass [syscall.O_NONBLOCK] so opening a FIFO inode does not
// block waiting for a writer — the caller (rejectSpecialMode + Stat
// on the fd) immediately rejects FIFOs/sockets/devices, but only if
// open() returns. Without O_NONBLOCK the open of a read-only FIFO
// blocks until a writer is connected, which would deadlock the test
// suite and any user who pointed `spy` at a stray FIFO. The flag
// affects only the open(2) blocking behaviour, not reads — and we
// never read from rejected fds anyway (Copilot review acceptance M1).
//
// The Unix kernels return ELOOP when a path component is a symlink and
// O_NOFOLLOW is set; we let that surface as an open() error rather
// than try to translate it to a custom sentinel — the existing
// [classifyFSError] path produces a useful enough message.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
