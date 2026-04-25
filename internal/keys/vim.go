// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package keys

import (
	"github.com/charmbracelet/bubbles/key"
)

// WithVim returns a copy of `base` with the additive vim bindings layered
// on top. The default bindings remain active so a user who doesn't know
// (or want) vim keybindings can still use the arrow keys / Home / End /
// PageUp / PageDown.
//
// Bindings added (per contracts/keys.md "Vim addition" column):
//
//	Action               | Vim key
//	---------------------+--------
//	scroll_up            | k
//	scroll_down          | j
//	scroll_left          | h
//	scroll_right         | l
//	page_up              | ctrl+b
//	page_down            | ctrl+f
//	half_page_up         | ctrl+u
//	half_page_down       | ctrl+d
//	go_to_top            | g (interpreted by ui as part of "gg")
//	go_to_bottom         | G
//	beginning_of_line    | 0
//	end_of_line          | $
//
// The "gg" sequence is enforced at the UI layer (`internal/ui` tracks a
// pending-g flag); the keymap simply binds the literal `g` key. A single
// `g` press without a follow-up is swallowed by the UI rather than
// firing ActionGoToTop.
//
// `?` (vim's help toggle) is intentionally NOT added because the
// default keymap already binds it as ActionSearchBackward; remapping
// would shadow backward search. F1 remains the only help toggle in
// both default and vim modes — contracts/keys.md was updated to match
// (Copilot review PR#9 round-2 #5; the contract previously listed `?`
// as a vim help addition, which conflicted with the search-backward
// binding the implementation has always honoured).
//
// `ZZ` and `:q` (vim quit) are not bound here because:
//   - `:q` is a command-line command resolved by [internal/ui]'s
//     command-line dispatcher.
//   - `ZZ` is a sequence binding that would require yet another
//     pending-key state machine; deferred to a future polish task.
func WithVim(base KeyMap) KeyMap {
	out := cloneKeyMap(base)
	add := func(action Action, label, k string) {
		out[action] = appendBinding(out[action], k, label)
	}

	add(ActionScrollUp, "scroll up (vim)", "k")
	add(ActionScrollDown, "scroll down (vim)", "j")
	add(ActionScrollLeft, "scroll left (vim)", "h")
	add(ActionScrollRight, "scroll right (vim)", "l")
	add(ActionPageUp, "page up (vim)", "ctrl+b")
	add(ActionPageDown, "page down (vim)", "ctrl+f")
	add(ActionHalfPageUp, "half page up (vim)", "ctrl+u")
	add(ActionHalfPageDown, "half page down (vim)", "ctrl+d")
	add(ActionGoToTop, "go to top (vim: gg)", "g")
	add(ActionGoToBottom, "go to bottom (vim)", "G")
	add(ActionBeginningOfLine, "beginning of line (vim)", "0")
	add(ActionEndOfLine, "end of line (vim)", "$")

	return out
}

// appendBinding returns the existing binding slice with one additional
// [key.Binding] for `k`. Each vim binding is its own [key.Binding] so
// the F1 help overlay can present "↑ / k" rather than collapsing the
// keys into a single line — readability beats density for keymap docs.
func appendBinding(existing []key.Binding, k, label string) []key.Binding {
	b := key.NewBinding(
		key.WithKeys(k),
		key.WithHelp(displayKeys([]string{k}), label),
	)
	out := make([]key.Binding, len(existing), len(existing)+1)
	copy(out, existing)
	return append(out, b)
}
