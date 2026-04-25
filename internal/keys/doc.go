// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package keys defines the Action vocabulary the viewer understands and
// the KeyMap that binds key sequences to Actions. Default returns the base
// arrow / named-key bindings; WithVim layers additive vim bindings on top;
// ApplyOverrides merges user config-file overrides and reports unknown
// actions or unparsable key strings as warnings rather than fatal errors.
package keys
