// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package keys

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
)

// Action names a viewer behaviour. The full vocabulary is fixed at
// compile time; bindings are configurable via [ApplyOverrides].
type Action string

// Navigation actions.
const (
	ActionScrollUp        Action = "scroll_up"
	ActionScrollDown      Action = "scroll_down"
	ActionScrollLeft      Action = "scroll_left"
	ActionScrollRight     Action = "scroll_right"
	ActionPageUp          Action = "page_up"
	ActionPageDown        Action = "page_down"
	ActionHalfPageUp      Action = "half_page_up"
	ActionHalfPageDown    Action = "half_page_down"
	ActionGoToTop         Action = "go_to_top"
	ActionGoToBottom      Action = "go_to_bottom"
	ActionBeginningOfLine Action = "beginning_of_line"
	ActionEndOfLine       Action = "end_of_line"
	ActionNextPage        Action = "next_page"
	ActionPrevPage        Action = "prev_page"
)

// Search actions.
const (
	ActionSearchForward  Action = "search_forward"
	ActionSearchBackward Action = "search_backward"
	ActionSearchSubmit   Action = "search_submit"
	ActionSearchCancel   Action = "search_cancel"
	ActionNextMatch      Action = "next_match"
	ActionPrevMatch      Action = "prev_match"
)

// Command-line actions.
const (
	ActionCommandOpen   Action = "command_open"
	ActionCommandSubmit Action = "command_submit"
	ActionCommandCancel Action = "command_cancel"
)

// Application-level actions.
const (
	ActionQuit              Action = "quit"
	ActionToggleHelp        Action = "toggle_help"
	ActionOpenFile          Action = "open_file"
	ActionReload            Action = "reload"
	ActionToggleLineNumbers Action = "toggle_line_numbers"
	ActionToggleWordWrap    Action = "toggle_word_wrap"
)

// KeyMap binds each known [Action] to one or more [key.Binding] entries.
// A [Action] with an empty slice means "no key triggers this action" — the
// help overlay omits it but no error is produced.
type KeyMap map[Action][]key.Binding

// ErrUnknownAction wraps any override that names an Action the viewer
// does not understand. Callers can detect it with [errors.Is].
var ErrUnknownAction = errors.New("unknown action")

// ErrUnknownKey wraps an override entry whose key string cannot be parsed
// by the underlying key package. Reserved for future use; today
// [ApplyOverrides] accepts any non-empty string at the binding layer
// because [key.NewBinding] is permissive.
var ErrUnknownKey = errors.New("unknown key")

// knownActions is populated from [allActionsList]; its zero-cost lookup
// keeps [ApplyOverrides] linear in the number of overrides. See
// [allActionsList] for the canonical order.
var knownActions = func() map[Action]struct{} {
	out := make(map[Action]struct{}, len(allActionsList))
	for _, a := range allActionsList {
		out[a] = struct{}{}
	}
	return out
}()

// allActionsList is the canonical iteration order of every [Action] the
// viewer understands. Tests in this package assert that this list is
// closed (no Action present in code that is missing here, none missing
// from code that appears here).
var allActionsList = []Action{
	ActionScrollUp, ActionScrollDown, ActionScrollLeft, ActionScrollRight,
	ActionPageUp, ActionPageDown, ActionHalfPageUp, ActionHalfPageDown,
	ActionGoToTop, ActionGoToBottom, ActionBeginningOfLine, ActionEndOfLine,
	ActionNextPage, ActionPrevPage,
	ActionSearchForward, ActionSearchBackward, ActionSearchSubmit,
	ActionSearchCancel, ActionNextMatch, ActionPrevMatch,
	ActionCommandOpen, ActionCommandSubmit, ActionCommandCancel,
	ActionQuit, ActionToggleHelp, ActionOpenFile, ActionReload,
	ActionToggleLineNumbers, ActionToggleWordWrap,
}

// ApplyOverrides returns a new [KeyMap] in which any action present in
// `overrides` has its bindings replaced by the supplied keys. Unknown
// actions are skipped and reported as wrapped [ErrUnknownAction] entries
// in the returned slice; callers are expected to surface them as warnings
// (per contracts/keys.md "Unrecognized keys log a warning to --debug
// and are silently dropped").
//
// The base [KeyMap] is never mutated. An empty key list (`{}`) means "no
// keys for this action" and disables it. A nil or empty `overrides` map
// returns a deep copy of the base unchanged.
func ApplyOverrides(base KeyMap, overrides map[string][]string) (KeyMap, []error) {
	out := cloneKeyMap(base)
	if len(overrides) == 0 {
		return out, nil
	}
	var warnings []error
	for name, keys := range overrides {
		action := Action(name)
		if _, ok := knownActions[action]; !ok {
			warnings = append(warnings, fmt.Errorf("%w: %q", ErrUnknownAction, name))
			continue
		}
		out[action] = bindingsFromKeys(action, keys)
	}
	return out, warnings
}

// cloneKeyMap returns a deep copy whose binding slices are independent of
// the source (so overrides cannot leak into the caller's base map).
func cloneKeyMap(km KeyMap) KeyMap {
	out := make(KeyMap, len(km))
	for action, bs := range km {
		clone := make([]key.Binding, len(bs))
		copy(clone, bs)
		out[action] = clone
	}
	return out
}

// bindingsFromKeys produces a single binding whose key set is the union of
// `keys`, or an empty slice if `keys` is empty. The help label is the
// stock label for that action so the F1 overlay still has a readable
// description after a remap.
func bindingsFromKeys(action Action, keys []string) []key.Binding {
	if len(keys) == 0 {
		return []key.Binding{}
	}
	help := actionHelp(action)
	return []key.Binding{
		key.NewBinding(
			key.WithKeys(keys...),
			key.WithHelp(displayKeys(keys), help),
		),
	}
}
