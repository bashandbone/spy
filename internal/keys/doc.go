// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package keys defines the viewer's keyboard action vocabulary.
//
// Status: skeleton (Phase 1). The exported API lands in Phase 2.
//
// Planned: an Action vocabulary the viewer understands; a KeyMap binding
// key sequences to Actions; a Default constructor returning the base arrow
// / named-key bindings; a WithVim layer that adds additive vim bindings
// on top; an ApplyOverrides merger that takes user config-file overrides
// and reports unknown actions or unparsable key strings as warnings rather
// than fatal errors.
package keys
