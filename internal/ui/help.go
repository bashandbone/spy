// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

// Help is the F1 / `?` overlay. Phase 2 ships an empty stub; the full
// implementation (centred lipgloss block populated from the active
// KeyMap) lands later in the polish phase.
//
// The function exists so other story phases that wire `?` to a help
// toggle have a stable target call site. Returning the empty string
// is intentional — the foundational viewer simply ignores the toggle.
func (m Model) Help() string {
	return ""
}
