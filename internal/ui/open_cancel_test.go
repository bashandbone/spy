// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The tests below pin acceptance review M6: a `:open <path>` command
// that's still in flight when the user quits must have its CancelFunc
// invoked instead of being silently dropped along with the pending
// openResultMsg. Without the openCancel field on Model, Bubble Tea's
// abandoned-message path leaked both the new stream's reader
// goroutine and its cancel func.

// TestM6_RunOpenCommandStashesCancel verifies the structural
// invariant: invoking runOpenCommand against a real path stashes a
// non-nil openCancel on the model BEFORE the returned tea.Cmd runs.
// That stash is what the quit paths consume.
func TestM6_RunOpenCommandStashesCancel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m := newTestModel(t, "original\n")
	m, _ = applyResize(m, 80, 24)

	updated, cmd := m.runOpenCommand(path)
	m = updated.(Model)
	if m.openCancel == nil {
		t.Fatal("runOpenCommand should stash openCancel on the model BEFORE returning the tea.Cmd")
	}
	if cmd == nil {
		t.Fatal("runOpenCommand should return a tea.Cmd")
	}
	// Drive the cmd to completion so the openResultMsg arrives and
	// clears openCancel.
	msg := cmd()
	if _, ok := msg.(openResultMsg); !ok {
		t.Fatalf("expected openResultMsg, got %T", msg)
	}
	updated2, _ := m.Update(msg)
	m = updated2.(Model)
	if m.openCancel != nil {
		t.Errorf("onOpenResult should clear openCancel; got non-nil")
	}
}

// TestM6_QuitCancelsInFlightOpen verifies the quit path: starting an
// `:open <path>` stashes openCancel; an immediate q keypress invokes
// it before the tea.Quit fires. We instrument cancellation via a
// cancel-tracking helper that wraps the real CancelFunc.
func TestM6_QuitCancelsInFlightOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.txt")
	if err := os.WriteFile(path, []byte("body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m := newTestModel(t, "original\n")
	m, _ = applyResize(m, 80, 24)

	updated, _ := m.runOpenCommand(path)
	m = updated.(Model)
	if m.openCancel == nil {
		t.Fatal("runOpenCommand did not stash openCancel")
	}

	// Wrap m.openCancel so we can observe invocation. Atomic so the
	// observation is race-detector clean even though we never spawn
	// another goroutine here.
	var cancelled atomic.Bool
	orig := m.openCancel
	m.openCancel = func() {
		cancelled.Store(true)
		orig()
	}

	// Send 'q' — ActionQuit handler must invoke openCancel.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should produce a tea.Cmd (tea.Quit)")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
	if !cancelled.Load() {
		t.Error("ActionQuit must invoke openCancel before returning tea.Quit")
	}
}

// TestM6_QuitCommandCancelsInFlightOpen mirrors the above for the
// `:q` / `:quit` command path.
func TestM6_QuitCommandCancelsInFlightOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.txt")
	if err := os.WriteFile(path, []byte("body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m := newTestModel(t, "original\n")
	m, _ = applyResize(m, 80, 24)

	updated, _ := m.runOpenCommand(path)
	m = updated.(Model)

	var cancelled atomic.Bool
	orig := m.openCancel
	m.openCancel = func() {
		cancelled.Store(true)
		orig()
	}

	updated2, cmd := m.runCommand("q")
	_ = updated2
	if cmd == nil {
		t.Fatal(":q should return a tea.Quit cmd")
	}
	if !cancelled.Load() {
		t.Error(":q should invoke openCancel before tea.Quit")
	}
}

// TestM6_SecondOpenCancelsFirst verifies that issuing a second
// `:open <path>` while the first is still in flight cancels the
// prior in-flight loader's CancelFunc — otherwise the prior open's
// goroutine survives until its tea.Cmd produces a message that the
// new model will then drop on the floor (gen-mismatch).
func TestM6_SecondOpenCancelsFirst(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.txt")
	pathB := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(pathA, []byte("a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile a: %v", err)
	}
	if err := os.WriteFile(pathB, []byte("b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile b: %v", err)
	}

	m := newTestModel(t, "original\n")
	m, _ = applyResize(m, 80, 24)

	updated, _ := m.runOpenCommand(pathA)
	m = updated.(Model)
	if m.openCancel == nil {
		t.Fatal("first runOpenCommand did not stash openCancel")
	}

	var firstCancelled atomic.Bool
	origA := m.openCancel
	m.openCancel = func() {
		firstCancelled.Store(true)
		origA()
	}

	updated2, _ := m.runOpenCommand(pathB)
	m = updated2.(Model)
	if !firstCancelled.Load() {
		t.Error("second runOpenCommand should cancel the in-flight first open")
	}
	if m.openCancel == nil {
		t.Error("second runOpenCommand should stash a fresh openCancel")
	}
}
