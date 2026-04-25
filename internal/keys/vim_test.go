// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package keys

import (
	"testing"
)

func TestWithVim_PreservesDefaultArrowBindings(t *testing.T) {
	t.Parallel()
	base := Default()
	vim := WithVim(base)
	// Arrow keys must still resolve to the default scroll actions.
	cases := []struct {
		action Action
		key    string
	}{
		{ActionScrollUp, "up"},
		{ActionScrollDown, "down"},
		{ActionScrollLeft, "left"},
		{ActionScrollRight, "right"},
		{ActionPageUp, "pgup"},
		{ActionPageDown, "pgdown"},
		{ActionGoToTop, "home"},
		{ActionGoToBottom, "end"},
	}
	for _, tc := range cases {
		if !bindingsContain(vim[tc.action], tc.key) {
			t.Errorf("WithVim()[%q] dropped default key %q (have %v)",
				tc.action, tc.key, bindingKeys(vim[tc.action]))
		}
	}
}

func TestWithVim_AddsScrollKeys(t *testing.T) {
	t.Parallel()
	vim := WithVim(Default())
	cases := []struct {
		action Action
		key    string
	}{
		{ActionScrollUp, "k"},
		{ActionScrollDown, "j"},
		{ActionScrollLeft, "h"},
		{ActionScrollRight, "l"},
	}
	for _, tc := range cases {
		if !bindingsContain(vim[tc.action], tc.key) {
			t.Errorf("WithVim()[%q] missing vim key %q (have %v)",
				tc.action, tc.key, bindingKeys(vim[tc.action]))
		}
	}
}

func TestWithVim_AddsPageAndHalfPage(t *testing.T) {
	t.Parallel()
	vim := WithVim(Default())
	cases := []struct {
		action Action
		key    string
	}{
		{ActionPageUp, "ctrl+b"},
		{ActionPageDown, "ctrl+f"},
		{ActionHalfPageUp, "ctrl+u"},
		{ActionHalfPageDown, "ctrl+d"},
	}
	for _, tc := range cases {
		if !bindingsContain(vim[tc.action], tc.key) {
			t.Errorf("WithVim()[%q] missing key %q (have %v)",
				tc.action, tc.key, bindingKeys(vim[tc.action]))
		}
	}
}

func TestWithVim_AddsGoToAndLineNav(t *testing.T) {
	t.Parallel()
	vim := WithVim(Default())
	cases := []struct {
		action Action
		key    string
	}{
		// g is the trigger key for "gg"; the UI sequences the second press.
		{ActionGoToTop, "g"},
		{ActionGoToBottom, "G"},
		{ActionBeginningOfLine, "0"},
		{ActionEndOfLine, "$"},
	}
	for _, tc := range cases {
		if !bindingsContain(vim[tc.action], tc.key) {
			t.Errorf("WithVim()[%q] missing key %q (have %v)",
				tc.action, tc.key, bindingKeys(vim[tc.action]))
		}
	}
}

func TestWithVim_DoesNotMutateBase(t *testing.T) {
	t.Parallel()
	base := Default()
	original := dumpKeyMap(base)
	_ = WithVim(base)
	after := dumpKeyMap(base)
	if len(original) != len(after) {
		t.Fatalf("base length changed: %d -> %d", len(original), len(after))
	}
	for action, beforeKeys := range original {
		afterKeys := after[action]
		if len(beforeKeys) != len(afterKeys) {
			t.Errorf("base[%q] mutated: %v -> %v", action, beforeKeys, afterKeys)
			continue
		}
		for i := range beforeKeys {
			if beforeKeys[i] != afterKeys[i] {
				t.Errorf("base[%q][%d] mutated: %q -> %q",
					action, i, beforeKeys[i], afterKeys[i])
			}
		}
	}
}

func TestWithVim_BindingsHaveHelp(t *testing.T) {
	t.Parallel()
	vim := WithVim(Default())
	for action, bindings := range vim {
		for i, b := range bindings {
			h := b.Help()
			if h.Key == "" {
				t.Errorf("WithVim()[%q][%d] empty Help.Key", action, i)
			}
			if h.Desc == "" {
				t.Errorf("WithVim()[%q][%d] empty Help.Desc", action, i)
			}
		}
	}
}

// TestWithVim_DoesNotShadowSearchBackward verifies that `?` continues to
// resolve to ActionSearchBackward in vim mode (per contracts/keys.md
// conflict-resolution rules). Vim's `?` (help) shortcut would conflict
// so we explicitly do not add it.
func TestWithVim_DoesNotShadowSearchBackward(t *testing.T) {
	t.Parallel()
	vim := WithVim(Default())
	if !bindingsContain(vim[ActionSearchBackward], "?") {
		t.Errorf("vim mode dropped ? from ActionSearchBackward (have %v)",
			bindingKeys(vim[ActionSearchBackward]))
	}
	if bindingsContain(vim[ActionToggleHelp], "?") {
		t.Errorf("vim mode mistakenly bound ? to ActionToggleHelp (would shadow search)")
	}
}
