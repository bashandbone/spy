// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package highlight

import "testing"

// Phase 2 highlight is a no-op skeleton; the full Highlighter (Chroma
// lexer + formatter, streaming, HighlightCap downgrade) lands in US1
// (T042). These smoke tests pin the constructor and Disabled() semantics
// so the foundational viewer can rely on a non-nil zero-value Highlighter.

func TestNew_ReturnsNonNil(t *testing.T) {
	h := New()
	if h == nil {
		t.Fatal("New returned nil")
	}
}

func TestDisabled_ZeroValueIsFalse(t *testing.T) {
	h := New()
	if h.Disabled() {
		t.Errorf("zero-value Highlighter must report Disabled=false")
	}
}

func TestDisabled_NilSafe(t *testing.T) {
	var h *Highlighter
	if h.Disabled() {
		t.Errorf("nil Highlighter must report Disabled=false (defensive)")
	}
}
