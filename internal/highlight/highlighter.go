// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package highlight

// Highlighter is the per-session syntax-highlighting engine. The full
// implementation (Chroma lexer + formatter, streaming, HighlightCap
// downgrade) lands in US1 (T042). This Phase 2 skeleton exists so
// internal/render and internal/ui can wire dependencies through their
// constructors today; the zero-value Highlighter is safe to pass and
// produces no styling.
type Highlighter struct {
	// disabled records whether HighlightCap was exceeded so the status
	// bar can surface the WarnHighlightDisabled advisory once US1
	// implements the side-channel.
	disabled bool
}

// New is the placeholder constructor. The real signature, parameters,
// and behaviour are wired in T042.
func New() *Highlighter {
	return &Highlighter{}
}

// Disabled reports whether highlighting was disabled (HighlightCap hit).
// US1 populates this; for now the foundational viewer never exceeds the
// cap because the cap is checked at a layer that doesn't exist yet.
func (h *Highlighter) Disabled() bool {
	if h == nil {
		return false
	}
	return h.disabled
}
