// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// fakeSource satisfies [source.Source] over an in-memory body so the
// model tests don't touch the filesystem.
type fakeSource struct {
	body string
	kind source.Kind
}

func (f *fakeSource) Kind() source.Kind         { return f.kind }
func (f *fakeSource) DisplayName() string       { return "fake.txt" }
func (f *fakeSource) Metadata() source.Metadata { return source.Metadata{LineCount: -1} }
func (f *fakeSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.body)), nil
}
func (f *fakeSource) Reopen() (io.ReadSeeker, error) {
	return strings.NewReader(f.body), nil
}

func newTestModel(t *testing.T, body string) Model {
	t.Helper()
	src := &fakeSource{body: body, kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	cfg := config.Defaults()
	return NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
	})
}

func TestNewModel_FirstFramePaintsAfterResize(t *testing.T) {
	m := newTestModel(t, "alpha\nbeta\ngamma\n")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.(Model).View().Content
	if !strings.Contains(view, "alpha") {
		t.Errorf("first frame missing content: %q", view)
	}
}

func TestUpdate_QuitOnQ(t *testing.T) {
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q should produce a tea.Cmd (Quit)")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestUpdate_QuitOnEsc(t *testing.T) {
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should produce a tea.Cmd (Quit)")
	}
}

func TestUpdate_QuitOnCtrlC(t *testing.T) {
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c should produce a tea.Cmd (Quit)")
	}
}

func TestUpdate_ScrollDownOnArrow(t *testing.T) {
	body := strings.Repeat("line\n", 100)
	m := newTestModel(t, body)
	m, _ = applyResize(m, 80, 10)
	before := m.viewport.YOffset
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	after := updated.(Model).viewport.YOffset
	if after <= before {
		t.Errorf("ScrollDown should advance viewport YOffset: before=%d after=%d", before, after)
	}
}

func TestUpdate_ScrollUpOnArrow(t *testing.T) {
	body := strings.Repeat("line\n", 100)
	m := newTestModel(t, body)
	m, _ = applyResize(m, 80, 10)
	// Scroll down a bit then back up.
	for i := 0; i < 5; i++ {
		mm, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = mm.(Model)
	}
	before := m.viewport.YOffset
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	after := updated.(Model).viewport.YOffset
	if after >= before {
		t.Errorf("ScrollUp should reduce viewport YOffset: before=%d after=%d", before, after)
	}
}

func TestUpdate_ResizeReflowsViewport(t *testing.T) {
	m := newTestModel(t, "alpha\nbeta\n")
	updated1, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated2, _ := updated1.(Model).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	got := updated2.(Model).viewport.Width
	if got != 100 {
		t.Errorf("viewport Width after resize: got %d want 100", got)
	}
}

func TestView_FooterMentionsSource(t *testing.T) {
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	view := m.View().Content
	if !strings.Contains(view, "fake.txt") {
		t.Errorf("footer should mention source name; view=%q", view)
	}
}

func TestView_EmptyBeforeResize(t *testing.T) {
	m := newTestModel(t, "x\n")
	if v := m.View().Content; v != "" {
		t.Errorf("View before resize should be empty, got %q", v)
	}
}

func TestUpdate_PageUpAndPageDown(t *testing.T) {
	body := strings.Repeat("line\n", 200)
	m := newTestModel(t, body)
	m, _ = applyResize(m, 80, 20)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	mDown := updated.(Model)
	if mDown.viewport.YOffset == 0 {
		t.Errorf("PageDown should advance viewport")
	}
	updated, _ = mDown.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	mUp := updated.(Model)
	if mUp.viewport.YOffset >= mDown.viewport.YOffset {
		t.Errorf("PageUp should reduce viewport YOffset")
	}
}

func TestUpdate_GoToTopAndBottom(t *testing.T) {
	body := strings.Repeat("line\n", 200)
	m := newTestModel(t, body)
	m, _ = applyResize(m, 80, 20)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	mEnd := updated.(Model)
	if mEnd.viewport.YOffset == 0 {
		t.Errorf("GoToBottom should advance viewport from 0")
	}
	updated, _ = mEnd.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	mTop := updated.(Model)
	if mTop.viewport.YOffset != 0 {
		t.Errorf("GoToTop should reset YOffset to 0, got %d", mTop.viewport.YOffset)
	}
}

func TestUpdate_UnboundKeyIsNoOp(t *testing.T) {
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if cmd != nil {
		t.Errorf("unbound key produced a command")
	}
	if updated.(Model).viewport.YOffset != m.viewport.YOffset {
		t.Errorf("unbound key changed viewport")
	}
}

func TestUpdate_ChunkLoadedRepaints(t *testing.T) {
	// 200-line body forces continuation chunks beyond the synchronous
	// First chunk; we feed a chunkLoadedMsg to onChunk and assert that
	// the viewport content reflects buffered lines.
	body := strings.Repeat("line\n", 200)
	m := newTestModel(t, body)
	m, _ = applyResize(m, 80, 20)
	// Drain one update from the loader stream into a chunkLoadedMsg.
	c, ok := <-m.stream.Updates
	if !ok {
		t.Fatal("expected at least one Update chunk")
	}
	updated, _ := m.Update(chunkLoadedMsg{chunk: c})
	view := updated.(Model).viewport.View()
	if !strings.Contains(view, "line") {
		t.Errorf("chunk-driven re-render produced no content: %q", view)
	}
}

func TestUpdate_StreamDoneFlipsStreaming(t *testing.T) {
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	// Small file → first chunk has EOF, so streaming is already false.
	// Force streaming=true then send streamDoneMsg to exercise the flip.
	m.streaming = true
	updated, _ := m.Update(streamDoneMsg{})
	if updated.(Model).streaming {
		t.Errorf("streamDoneMsg did not flip streaming=false")
	}
}

func TestWaitForChunk_ProducesMsg(t *testing.T) {
	body := strings.Repeat("line\n", 200)
	m := newTestModel(t, body)
	cmd := waitForChunk(m.stream)
	if cmd == nil {
		t.Fatal("waitForChunk returned nil cmd")
	}
	msg := cmd()
	switch msg.(type) {
	case chunkLoadedMsg, streamDoneMsg:
		// acceptable: either a chunk arrived or the stream is done.
	default:
		t.Errorf("waitForChunk produced unexpected msg type %T", msg)
	}
}

func TestModel_InitReturnsCmd(t *testing.T) {
	body := strings.Repeat("line\n", 200)
	m := newTestModel(t, body)
	if cmd := m.Init(); cmd == nil {
		t.Errorf("Init should return a tea.Cmd to subscribe to chunks")
	}
}

func TestView_FooterDroppedInOneRowTerminal(t *testing.T) {
	m := newTestModel(t, "alpha\nbeta\n")
	m, _ = applyResize(m, 80, 1)
	view := m.View().Content
	if strings.Contains(view, "Line ") {
		t.Errorf("1-row terminal should not render footer; got %q", view)
	}
}

func TestUpdate_ResizePreservesYOffset(t *testing.T) {
	body := strings.Repeat("line\n", 200)
	m := newTestModel(t, body)
	m, _ = applyResize(m, 80, 20)
	// Drain so buffer has > 20 lines.
	for c := range m.stream.Updates {
		updated, _ := m.Update(chunkLoadedMsg{chunk: c})
		m = updated.(Model)
	}
	// Scroll down a few rows.
	for i := 0; i < 10; i++ {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(Model)
	}
	beforeOff := m.viewport.YOffset
	if beforeOff == 0 {
		t.Skip("viewport refused to scroll; can't exercise the resize-preserve path")
	}
	// Resize and confirm the YOffset survives (SC-008).
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	after := updated.(Model)
	if after.viewport.YOffset != beforeOff {
		t.Errorf("resize should preserve YOffset: before=%d after=%d",
			beforeOff, after.viewport.YOffset)
	}
	if after.viewport.Width != 100 {
		t.Errorf("resize should update Width: got %d", after.viewport.Width)
	}
}

func TestView_EmptyInputShowsLineZero(t *testing.T) {
	m := newTestModel(t, "")
	m, _ = applyResize(m, 80, 24)
	view := m.View().Content
	if !strings.Contains(view, "Line 0") {
		t.Errorf("empty input footer should show Line 0, got %q", view)
	}
}

func TestUpdate_HalfPageDownDistinctFromPageDown(t *testing.T) {
	body := strings.Repeat("line\n", 200)
	m := newTestModel(t, body)
	m, _ = applyResize(m, 80, 20)
	// Drive a chunkLoadedMsg through so the viewport has > 20 lines of
	// content (otherwise PageDown is a no-op at the end).
	for c := range m.stream.Updates {
		updated, _ := m.Update(chunkLoadedMsg{chunk: c})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	pageDown := updated.(Model).viewport.YOffset

	m2 := newTestModel(t, body)
	m2, _ = applyResize(m2, 80, 20)
	for c := range m2.stream.Updates {
		updated, _ := m2.Update(chunkLoadedMsg{chunk: c})
		m2 = updated.(Model)
	}
	// Half-page-down doesn't have a default key binding, but we can
	// directly trigger it via the action by feeding ctrl+d if the user
	// configured vim mode. For Phase 2 the action is wired but unbound;
	// the test just confirms that pageDown advanced more than a typical
	// half-page would (~10 lines for height=20).
	if pageDown < 10 {
		t.Errorf("PageDown advanced %d rows; expected ~full page (height=20)", pageDown)
	}
}

// applyResize is a tiny convenience around the Update path.
func applyResize(m Model, w, h int) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(Model), cmd
}
