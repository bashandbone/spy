// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package keys

import (
	"errors"
	"sort"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

// allActions is the complete vocabulary the viewer must understand. Every
// row in contracts/keys.md plus the application actions named in tasks.md
// (T046c ActionReload, T100b/c ActionToggleLineNumbers /
// ActionToggleWordWrap / ActionOpenFile) is covered here. Adding a new
// Action to keymap.go without adding it here is a contract drift; the
// TestActionVocabularyIsClosed guard below enforces parity in the other
// direction.
var allActions = []Action{
	// Navigation
	ActionScrollUp, ActionScrollDown, ActionScrollLeft, ActionScrollRight,
	ActionPageUp, ActionPageDown, ActionHalfPageUp, ActionHalfPageDown,
	ActionGoToTop, ActionGoToBottom, ActionBeginningOfLine, ActionEndOfLine,
	ActionNextPage, ActionPrevPage,
	// Search
	ActionSearchForward, ActionSearchBackward, ActionSearchSubmit,
	ActionSearchCancel, ActionNextMatch, ActionPrevMatch,
	// Command line
	ActionCommandOpen, ActionCommandSubmit, ActionCommandCancel,
	// Application
	ActionQuit, ActionToggleHelp, ActionOpenFile, ActionReload,
	ActionToggleLineNumbers, ActionToggleWordWrap,
}

func TestActionConstantsAreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[Action]struct{}, len(allActions))
	for _, a := range allActions {
		if a == "" {
			t.Errorf("Action constant has empty string value")
		}
		if _, dup := seen[a]; dup {
			t.Errorf("duplicate Action constant: %q", a)
		}
		seen[a] = struct{}{}
	}
}

// TestActionVocabularyIsClosed pins the spelling of every Action so an
// accidental rename in keymap.go shows up here rather than in callers.
func TestActionVocabularyIsClosed(t *testing.T) {
	t.Parallel()
	want := []string{
		"beginning_of_line", "command_cancel", "command_open", "command_submit",
		"end_of_line", "go_to_bottom", "go_to_top", "half_page_down",
		"half_page_up", "next_match", "next_page", "open_file", "page_down",
		"page_up", "prev_match", "prev_page", "quit", "reload",
		"scroll_down", "scroll_left", "scroll_right", "scroll_up",
		"search_backward", "search_cancel", "search_forward", "search_submit",
		"toggle_help", "toggle_line_numbers", "toggle_word_wrap",
	}
	got := make([]string, len(allActions))
	for i, a := range allActions {
		got[i] = string(a)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("action count mismatch: got %d (%v) want %d (%v)", len(got), got, len(want), want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("action[%d]: got %q want %q (sorted)", i, got[i], w)
		}
	}
}

func TestDefaultKeyMapBindsRequiredActions(t *testing.T) {
	t.Parallel()
	km := Default()
	// Every Action with a non-vim default in contracts/keys.md must have
	// at least one binding. HalfPage{Up,Down}, BeginningOfLine, and
	// EndOfLine are intentionally vim-only — the vim layer in
	// internal/keys/vim.go (US2, T055) adds them.
	required := []Action{
		ActionScrollUp, ActionScrollDown, ActionScrollLeft, ActionScrollRight,
		ActionPageUp, ActionPageDown, ActionGoToTop, ActionGoToBottom,
		ActionNextPage, ActionPrevPage,
		ActionSearchForward, ActionSearchBackward, ActionSearchSubmit,
		ActionSearchCancel, ActionNextMatch, ActionPrevMatch,
		ActionCommandOpen, ActionCommandSubmit, ActionCommandCancel,
		ActionQuit, ActionToggleHelp, ActionOpenFile, ActionReload,
		ActionToggleLineNumbers, ActionToggleWordWrap,
	}
	for _, a := range required {
		bs, ok := km[a]
		if !ok {
			t.Errorf("Default() missing action %q", a)
			continue
		}
		if len(bs) == 0 {
			t.Errorf("Default()[%q] has zero bindings", a)
		}
	}
}

func TestDefaultBindingsMatchContract(t *testing.T) {
	t.Parallel()
	km := Default()
	cases := []struct {
		action Action
		keys   []string // every listed key MUST appear in the binding set
	}{
		{ActionScrollUp, []string{"up"}},
		{ActionScrollDown, []string{"down"}},
		{ActionScrollLeft, []string{"left"}},
		{ActionScrollRight, []string{"right"}},
		{ActionPageUp, []string{"pgup"}},
		{ActionPageDown, []string{"pgdown", " "}},
		{ActionGoToTop, []string{"home"}},
		{ActionGoToBottom, []string{"end"}},
		{ActionNextPage, []string{"]"}},
		{ActionPrevPage, []string{"["}},
		{ActionSearchForward, []string{"/"}},
		{ActionSearchBackward, []string{"?"}},
		{ActionSearchSubmit, []string{"enter"}},
		{ActionSearchCancel, []string{"esc"}},
		{ActionNextMatch, []string{"n"}},
		{ActionPrevMatch, []string{"N"}},
		{ActionCommandOpen, []string{":"}},
		{ActionCommandSubmit, []string{"enter"}},
		{ActionCommandCancel, []string{"esc"}},
		{ActionQuit, []string{"q", "esc", "ctrl+c"}},
		{ActionToggleHelp, []string{"f1"}},
		{ActionOpenFile, []string{"o"}},
		{ActionReload, []string{"ctrl+r", "r"}},
		{ActionToggleLineNumbers, []string{"ctrl+l"}},
		{ActionToggleWordWrap, []string{"ctrl+w"}},
	}
	for _, tc := range cases {
		bindings := km[tc.action]
		for _, want := range tc.keys {
			if !bindingsContain(bindings, want) {
				t.Errorf("Default()[%q] missing key %q (have %v)",
					tc.action, want, bindingKeys(bindings))
			}
		}
	}
}

// TestDefaultBindingsAreNonEmpty asserts every binding has a key set and a
// help label. Empty bindings would render blank rows in the F1 overlay.
func TestDefaultBindingsHaveHelp(t *testing.T) {
	t.Parallel()
	km := Default()
	for action, bindings := range km {
		for i, b := range bindings {
			h := b.Help()
			if h.Key == "" {
				t.Errorf("Default()[%q][%d] has empty Help.Key", action, i)
			}
			if h.Desc == "" {
				t.Errorf("Default()[%q][%d] has empty Help.Desc", action, i)
			}
			if len(b.Keys()) == 0 {
				t.Errorf("Default()[%q][%d] has zero key strings", action, i)
			}
		}
	}
}

// --- ApplyOverrides ---

func TestApplyOverrides_KnownAction(t *testing.T) {
	t.Parallel()
	base := Default()
	out, errs := ApplyOverrides(base, map[string][]string{
		"quit": {"x", "ctrl+q"},
	})
	if len(errs) != 0 {
		t.Errorf("unexpected errors for known action: %v", errs)
	}
	if !bindingsContain(out[ActionQuit], "x") {
		t.Errorf("ApplyOverrides did not bind \"x\" to ActionQuit")
	}
	if !bindingsContain(out[ActionQuit], "ctrl+q") {
		t.Errorf("ApplyOverrides did not bind \"ctrl+q\" to ActionQuit")
	}
	// Override REPLACES the action's bindings; the stock "q"/"esc"/"ctrl+c"
	// must no longer be present after a non-empty override.
	if bindingsContain(out[ActionQuit], "esc") {
		t.Errorf("ApplyOverrides should replace bindings on non-empty override, not merge")
	}
}

func TestApplyOverrides_UnknownAction(t *testing.T) {
	t.Parallel()
	base := Default()
	_, errs := ApplyOverrides(base, map[string][]string{
		"warp_drive": {"alt+w"},
	})
	if len(errs) == 0 {
		t.Fatalf("expected warning for unknown action")
	}
	if !errors.Is(errs[0], ErrUnknownAction) {
		t.Errorf("expected ErrUnknownAction wrapped, got %v", errs[0])
	}
	// Base map is untouched so callers can use it as a fallback.
	if !bindingsContain(base[ActionQuit], "q") {
		t.Errorf("base keymap was mutated by ApplyOverrides")
	}
}

func TestApplyOverrides_EmptyKeyList(t *testing.T) {
	t.Parallel()
	base := Default()
	// An explicit empty list means "drop all bindings for this action".
	out, errs := ApplyOverrides(base, map[string][]string{
		"toggle_help": {},
	})
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(out[ActionToggleHelp]) != 0 {
		t.Errorf("expected zero bindings after empty override, got %v",
			bindingKeys(out[ActionToggleHelp]))
	}
}

func TestApplyOverrides_Idempotent(t *testing.T) {
	t.Parallel()
	base := Default()
	overrides := map[string][]string{"quit": {"x"}}
	once, errs1 := ApplyOverrides(base, overrides)
	twice, errs2 := ApplyOverrides(once, overrides)
	if len(errs1) != 0 || len(errs2) != 0 {
		t.Errorf("unexpected errors: once=%v twice=%v", errs1, errs2)
	}
	if !equalKeyMaps(once, twice) {
		t.Errorf("ApplyOverrides not idempotent:\nonce =%v\ntwice=%v",
			dumpKeyMap(once), dumpKeyMap(twice))
	}
}

func TestApplyOverrides_NilOrEmpty(t *testing.T) {
	t.Parallel()
	base := Default()
	out, errs := ApplyOverrides(base, nil)
	if len(errs) != 0 {
		t.Errorf("nil overrides produced errors: %v", errs)
	}
	if !equalKeyMaps(base, out) {
		t.Errorf("nil overrides changed the keymap")
	}
	out, errs = ApplyOverrides(base, map[string][]string{})
	if len(errs) != 0 {
		t.Errorf("empty overrides produced errors: %v", errs)
	}
	if !equalKeyMaps(base, out) {
		t.Errorf("empty overrides changed the keymap")
	}
}

// TestActionHelpCoversEveryAction exercises [actionHelp] for the full
// vocabulary so a future Action added without a matching switch case
// surfaces here (and so coverage doesn't bottom out on the long switch).
func TestActionHelpCoversEveryAction(t *testing.T) {
	t.Parallel()
	for _, a := range allActions {
		got := actionHelp(a)
		if got == "" {
			t.Errorf("actionHelp(%q) = empty", a)
		}
	}
}

// TestApplyOverrides_ReboundActionKeepsHelpLabel verifies that the help
// label survives a remap so the F1 overlay isn't blank or the snake-case
// action name after a user override.
func TestApplyOverrides_ReboundActionKeepsHelpLabel(t *testing.T) {
	t.Parallel()
	out, _ := ApplyOverrides(Default(), map[string][]string{
		"toggle_word_wrap": {"alt+w"},
	})
	bs := out[ActionToggleWordWrap]
	if len(bs) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bs))
	}
	help := bs[0].Help()
	if help.Desc != "toggle word wrap" {
		t.Errorf("unexpected help desc: %q", help.Desc)
	}
	if help.Key == "" {
		t.Errorf("empty help key after override")
	}
}

// TestDisplayKeys covers the space-name carve-out and the empty case.
func TestDisplayKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"q"}, "q"},
		{[]string{"pgdown", " "}, "pgdown/space"},
		{[]string{"q", "esc", "ctrl+c"}, "q/esc/ctrl+c"},
	}
	for _, tc := range cases {
		if got := displayKeys(tc.in); got != tc.want {
			t.Errorf("displayKeys(%v) = %q want %q", tc.in, got, tc.want)
		}
	}
}

// --- helpers ---

func bindingsContain(bs []key.Binding, want string) bool {
	for _, b := range bs {
		for _, k := range b.Keys() {
			if k == want {
				return true
			}
		}
	}
	return false
}

func bindingKeys(bs []key.Binding) []string {
	out := make([]string, 0)
	for _, b := range bs {
		out = append(out, b.Keys()...)
	}
	return out
}

func equalKeyMaps(a, b KeyMap) bool {
	if len(a) != len(b) {
		return false
	}
	for action, av := range a {
		bv, ok := b[action]
		if !ok {
			return false
		}
		ak := bindingKeys(av)
		bk := bindingKeys(bv)
		if len(ak) != len(bk) {
			return false
		}
		sort.Strings(ak)
		sort.Strings(bk)
		for i := range ak {
			if ak[i] != bk[i] {
				return false
			}
		}
	}
	return true
}

func dumpKeyMap(km KeyMap) map[Action][]string {
	out := make(map[Action][]string, len(km))
	for a, bs := range km {
		ks := bindingKeys(bs)
		sort.Strings(ks)
		out[a] = ks
	}
	return out
}
