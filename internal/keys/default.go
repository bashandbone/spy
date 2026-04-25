// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package keys

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

// Default returns the stock keymap from contracts/keys.md (Default column).
// Vim additions land in [WithVim] (US2). The returned map is freshly
// allocated — callers may mutate it without affecting subsequent calls.
func Default() KeyMap {
	bind := func(help string, keys ...string) key.Binding {
		return key.NewBinding(
			key.WithKeys(keys...),
			key.WithHelp(displayKeys(keys), help),
		)
	}
	return KeyMap{
		// --- Navigation ---
		ActionScrollUp:    {bind("scroll up", "up")},
		ActionScrollDown:  {bind("scroll down", "down")},
		ActionScrollLeft:  {bind("scroll left", "left")},
		ActionScrollRight: {bind("scroll right", "right")},
		ActionPageUp:      {bind("page up", "pgup")},
		ActionPageDown:    {bind("page down", "pgdown", " ")},
		// HalfPage{Up,Down} have no default key; vim adds Ctrl-U / Ctrl-D.
		ActionHalfPageUp:   {},
		ActionHalfPageDown: {},
		ActionGoToTop:      {bind("go to top", "home")},
		ActionGoToBottom:   {bind("go to bottom", "end")},
		// BeginningOfLine / EndOfLine are vim-only by default; the contract
		// documents them as "Home (when on line)" but the foundational
		// viewer has no in-line cursor so the binding is left to vim.
		ActionBeginningOfLine: {},
		ActionEndOfLine:       {},
		ActionNextPage:        {bind("next PDF page", "]")},
		ActionPrevPage:        {bind("prev PDF page", "[")},

		// --- Search ---
		ActionSearchForward:  {bind("search forward", "/")},
		ActionSearchBackward: {bind("search backward", "?")},
		// Submit / cancel are bound here so the help overlay shows the
		// active prompt keys; the prompt-state machine in internal/ui
		// owns the runtime context check.
		ActionSearchSubmit: {bind("submit search", "enter")},
		ActionSearchCancel: {bind("cancel search", "esc")},
		ActionNextMatch:    {bind("next match", "n")},
		ActionPrevMatch:    {bind("prev match", "N")},

		// --- Command line ---
		ActionCommandOpen:   {bind("command", ":")},
		ActionCommandSubmit: {bind("submit command", "enter")},
		ActionCommandCancel: {bind("cancel command", "esc")},

		// --- Application ---
		ActionQuit:              {bind("quit", "q", "esc", "ctrl+c")},
		ActionToggleHelp:        {bind("help", "f1")},
		ActionOpenFile:          {bind("open file", "o")},
		ActionReload:            {bind("reload source", "ctrl+r", "r")},
		ActionToggleLineNumbers: {bind("toggle line numbers", "ctrl+l")},
		ActionToggleWordWrap:    {bind("toggle word wrap", "ctrl+w")},
	}
}

// actionHelp returns the human-readable description shown in the F1
// overlay for the given Action. Used by [ApplyOverrides] so a remapped
// action keeps its descriptive label.
func actionHelp(a Action) string {
	switch a {
	case ActionScrollUp:
		return "scroll up"
	case ActionScrollDown:
		return "scroll down"
	case ActionScrollLeft:
		return "scroll left"
	case ActionScrollRight:
		return "scroll right"
	case ActionPageUp:
		return "page up"
	case ActionPageDown:
		return "page down"
	case ActionHalfPageUp:
		return "half page up"
	case ActionHalfPageDown:
		return "half page down"
	case ActionGoToTop:
		return "go to top"
	case ActionGoToBottom:
		return "go to bottom"
	case ActionBeginningOfLine:
		return "beginning of line"
	case ActionEndOfLine:
		return "end of line"
	case ActionNextPage:
		return "next PDF page"
	case ActionPrevPage:
		return "prev PDF page"
	case ActionSearchForward:
		return "search forward"
	case ActionSearchBackward:
		return "search backward"
	case ActionSearchSubmit:
		return "submit search"
	case ActionSearchCancel:
		return "cancel search"
	case ActionNextMatch:
		return "next match"
	case ActionPrevMatch:
		return "prev match"
	case ActionCommandOpen:
		return "command"
	case ActionCommandSubmit:
		return "submit command"
	case ActionCommandCancel:
		return "cancel command"
	case ActionQuit:
		return "quit"
	case ActionToggleHelp:
		return "help"
	case ActionOpenFile:
		return "open file"
	case ActionReload:
		return "reload source"
	case ActionToggleLineNumbers:
		return "toggle line numbers"
	case ActionToggleWordWrap:
		return "toggle word wrap"
	}
	return string(a)
}

// displayKeys joins a key list into a single string suitable for the F1
// help overlay (e.g. "q/esc"). The space key prints as "space" rather
// than the raw character so the overlay isn't ambiguous.
func displayKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		if k == " " {
			parts[i] = "space"
		} else {
			parts[i] = k
		}
	}
	return strings.Join(parts, "/")
}
