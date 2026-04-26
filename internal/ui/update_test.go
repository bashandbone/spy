// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// fakeCodeSource is a code-kind variant of fakeSource for highlight tests.
type fakeCodeSource struct {
	body string
	lang string
}

func (f *fakeCodeSource) Kind() source.Kind { return source.KindCode }
func (f *fakeCodeSource) DisplayName() string {
	if f.lang != "" {
		return "fake." + f.lang
	}
	return "fake.txt"
}
func (f *fakeCodeSource) Metadata() source.Metadata {
	return source.Metadata{LineCount: -1, Language: f.lang}
}
func (f *fakeCodeSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.body)), nil
}
func (f *fakeCodeSource) Reopen() (io.ReadSeeker, error) {
	return strings.NewReader(f.body), nil
}

func TestNewModel_HighlightsCodeFirstChunk(t *testing.T) {
	src := &fakeCodeSource{body: "package main\nfunc x() {}\n", lang: "go"}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	h := highlight.New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024)
	cfg := config.Defaults()
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  h,
	})
	if h.Lang() != "go" {
		t.Errorf("NewModel did not propagate language to highlighter: got %q want go", h.Lang())
	}
	// First chunk's lines should have Tokens populated after NewModel.
	if len(m.stream.First.Lines) == 0 {
		t.Fatal("First chunk has no lines")
	}
	for i, l := range m.stream.First.Lines {
		if l.Tokens == nil {
			t.Errorf("line %d (%q) Tokens is nil; NewModel did not pre-highlight first chunk", i, l.Raw)
		}
	}
}

func TestUpdate_OnChunkRunsHighlighter(t *testing.T) {
	src := &fakeCodeSource{
		body: strings.Repeat("func main() {}\n", 200),
		lang: "go",
	}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	h := highlight.New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024)
	cfg := config.Defaults()
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  h,
	})
	m, _ = applyResize(m, 80, 24)
	c, ok := <-stream.Updates
	if !ok {
		t.Fatal("expected at least one Update chunk for a 200-line body")
	}
	updated, _ := m.Update(chunkLoadedMsg{chunk: c})
	mm := updated.(Model)
	view := mm.View()
	if !strings.Contains(view, "\x1b[") {
		t.Errorf("expected ANSI escapes in highlighted code view; got: %q", view)
	}
}

func TestUpdate_OnChunkSkipsHighlightForTextKind(t *testing.T) {
	// Plain text shouldn't pay the lex cost; the highlighter should not
	// be invoked even when one is provided.
	src := &fakeSource{body: "alpha\nbeta\n", kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	h := highlight.New(styles.Get("monokai"), term.ColorANSI256, 0) // 0 disables
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  h,
	})
	m, _ = applyResize(m, 80, 24)
	// Drive a fake chunk through.
	updated, _ := m.Update(chunkLoadedMsg{chunk: stream.First})
	mm := updated.(Model)
	if mm.statusAdvisory != "" {
		t.Errorf("text source should not surface highlighting advisories; got %q", mm.statusAdvisory)
	}
}

func TestUpdate_HighlightAdvisorySurfacedOnDisable(t *testing.T) {
	src := &fakeCodeSource{body: "package main\nfunc x() {}\n", lang: "go"}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	h := highlight.New(styles.Get("monokai"), term.ColorANSI256, 0)
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  h,
	})
	m, _ = applyResize(m, 80, 24)
	// First chunk highlight call already disabled the highlighter (cap=0).
	if !h.Disabled() {
		t.Fatalf("expected highlighter disabled after first call with cap=0")
	}
	// Drive a chunkLoadedMsg so onChunk drains the warning.
	updated, _ := m.Update(chunkLoadedMsg{chunk: stream.First})
	mm := updated.(Model)
	if mm.statusAdvisory == "" {
		t.Errorf("expected statusAdvisory after highlighter disable warning")
	}
}

// TestUpdate_ScrollHighlightsNewlyVisibleLines is a regression test for
// the bug where syntax highlighting only applied to the initially-visible
// viewport window. After scrolling, lines that were outside the initial
// viewport must receive ANSI color once they scroll into view.
func TestUpdate_ScrollHighlightsNewlyVisibleLines(t *testing.T) {
	const viewHeight = 10
	// Build a 40-line Go source; the viewport is 10 rows, so lines
	// 11+ are outside the initial window and were previously rendered
	// as raw text.
	var lineSlice []string
	for i := 0; i < 40; i++ {
		lineSlice = append(lineSlice, "x := 1")
	}
	body := strings.Join(lineSlice, "\n") + "\n"

	src := &fakeCodeSource{body: body, lang: "go"}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	h := highlight.New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024)
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: viewHeight + 1, ColorDepth: term.ColorANSI256},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  h,
	})
	m, _ = applyResize(m, 80, viewHeight+1) // +1 for footer row

	// Drain all chunks so the full 40-line buffer is resident.
	for c := range stream.Updates {
		updated, _ := m.Update(chunkLoadedMsg{chunk: c})
		m = updated.(Model)
	}

	// Scroll down far enough that the initially-visible lines are off
	// screen and only lines that were originally outside the viewport
	// are visible.
	for i := 0; i < viewHeight+5; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	if m.viewport.YOffset == 0 {
		t.Skip("viewport did not scroll; can't exercise this path")
	}

	// The visible content must contain ANSI escapes — if the bug
	// is present, lines that were below the initial viewport are
	// rendered as raw text and the view has no color codes.
	view := m.viewport.View()
	if !strings.Contains(view, "\x1b[") {
		t.Errorf("lines scrolled into view should be syntax-highlighted; view=%q", view)
	}
}

// --- ActionReload ---

func TestUpdate_ReloadSuccessfullyReplacesStream(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "reload.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := source.NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := loader.Open(ctx, src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	cancelCalled := atomic.Bool{}
	wrapCancel := context.CancelFunc(func() {
		cancelCalled.Store(true)
		cancel()
	})
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Cancel:       wrapCancel,
	})
	m, _ = applyResize(m, 80, 24)
	original := m.stream
	// Trigger reload.
	updated, cmd := m.Update(reloadMsg{})
	mm := updated.(Model)
	if !cancelCalled.Load() {
		t.Errorf("expected previous loader Cancel to fire on reload")
	}
	if cmd == nil {
		t.Fatal("reload should issue a command to perform Open")
	}
	// Run the cmd — it should produce a reloadResultMsg.
	resultMsg := cmd()
	rrm, ok := resultMsg.(reloadResultMsg)
	if !ok {
		t.Fatalf("reload cmd produced unexpected msg type %T", resultMsg)
	}
	if rrm.err != nil {
		t.Fatalf("reload cmd returned error: %v", rrm.err)
	}
	updated2, _ := mm.Update(rrm)
	mm2 := updated2.(Model)
	if mm2.stream == original {
		t.Errorf("reload should swap the stream pointer")
	}
	if mm2.lastError != nil {
		t.Errorf("reload should clear lastError on success, got %v", mm2.lastError)
	}
	if mm2.status == render.StatusError {
		t.Errorf("status should not be Error after successful reload")
	}
}

func TestUpdate_ReloadAfterDeleteRetainsBuffer(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "vanish.txt")
	if err := os.WriteFile(path, []byte("present\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := source.NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
	})
	m, _ = applyResize(m, 80, 24)
	priorBuffer := m.stream.Buffer
	// Delete the underlying file before reload.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	updated, cmd := m.Update(reloadMsg{})
	mm := updated.(Model)
	rrm, ok := cmd().(reloadResultMsg)
	if !ok {
		t.Fatalf("reload cmd: unexpected msg type")
	}
	if rrm.err == nil {
		t.Fatalf("reload should error when file is deleted; got nil")
	}
	updated2, _ := mm.Update(rrm)
	mm2 := updated2.(Model)
	if mm2.lastError == nil {
		t.Errorf("expected lastError to capture reload failure")
	}
	if mm2.status != render.StatusError {
		t.Errorf("expected StatusError after reload failure, got %v", mm2.status)
	}
	if mm2.stream == nil || mm2.stream.Buffer != priorBuffer {
		t.Errorf("reload failure must retain prior stream/buffer; got %v", mm2.stream)
	}
}

func TestUpdate_ReloadCancelsPreviousCancel(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cancel.txt")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := source.NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	called := atomic.Bool{}
	cancelFn := context.CancelFunc(func() { called.Store(true) })
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Cancel:       cancelFn,
	})
	m, _ = applyResize(m, 80, 24)
	_, _ = m.Update(reloadMsg{})
	if !called.Load() {
		t.Errorf("expected reload to invoke previous Cancel")
	}
}

func TestUpdate_ReloadOnNoSourceIsNoOp(t *testing.T) {
	m := newTestModel(t, "")
	m, _ = applyResize(m, 80, 24)
	m.source = nil // simulate degenerate state
	updated, cmd := m.Update(reloadMsg{})
	if cmd != nil {
		t.Errorf("reload with no source should be a no-op")
	}
	if updated.(Model).status == render.StatusError {
		t.Errorf("reload no-op should not surface an error")
	}
}

func TestUpdate_ReloadOnReloadKey(t *testing.T) {
	// 'r' is bound to ActionReload. Pressing it should produce a Cmd
	// that ultimately yields a reloadMsg.
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("'r' should produce a reload command")
	}
	msg := cmd()
	if _, ok := msg.(reloadMsg); !ok {
		t.Errorf("'r' should produce reloadMsg, got %T", msg)
	}
}

func TestUpdate_StreamDoneKeepsStatusError(t *testing.T) {
	// streamDoneMsg should not stomp on a StatusError set by reload.
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	m.status = render.StatusError
	m.lastError = errors.New("boom")
	updated, _ := m.Update(streamDoneMsg{})
	mm := updated.(Model)
	if mm.status != render.StatusError {
		t.Errorf("streamDoneMsg should preserve StatusError, got %v", mm.status)
	}
}

func TestNewModel_PreHighlightPushesTokensIntoBuffer(t *testing.T) {
	// Copilot review PR#8 #1: NewModel must propagate the first-chunk
	// tokens into the buffer (via SetTokens) so the renderer's
	// Buffer.Slice() returns lines with Tokens populated, NOT nil.
	src := &fakeCodeSource{body: "package main\nfunc x() {}\n", lang: "go"}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	h := highlight.New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024)
	NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  h,
	})
	resident := stream.Buffer.Slice(0, stream.Buffer.Total())
	if len(resident) == 0 {
		t.Fatal("buffer is empty")
	}
	for i, l := range resident {
		if l.Tokens == nil {
			t.Errorf("buffer line %d (%q) Tokens is nil after NewModel; SetTokens did not propagate",
				i, l.Raw)
		}
	}
}

func TestUpdate_OnChunkPushesTokensIntoBuffer(t *testing.T) {
	// Copilot review PR#8 #3: onChunk must propagate freshly-highlighted
	// tokens into the buffer so subsequent renders / scrolls don't
	// re-lex (which would burn the highlighter byte cap).
	src := &fakeCodeSource{
		body: strings.Repeat("func main() {}\n", 200),
		lang: "go",
	}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	h := highlight.New(styles.Get("monokai"), term.ColorANSI256, 5*1024*1024)
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorANSI256},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
		Highlighter:  h,
	})
	m, _ = applyResize(m, 80, 24)
	c, ok := <-stream.Updates
	if !ok {
		t.Fatal("expected a continuation chunk")
	}
	updated, _ := m.Update(chunkLoadedMsg{chunk: c, stream: stream})
	mm := updated.(Model)
	resident := mm.stream.Buffer.Slice(c.StartLine-1, c.StartLine-1+int64(len(c.Lines)))
	if len(resident) == 0 {
		t.Fatal("expected continuation lines in resident range")
	}
	hadTokens := false
	for _, l := range resident {
		if l.Tokens != nil {
			hadTokens = true
			break
		}
	}
	if !hadTokens {
		t.Errorf("onChunk did not propagate highlighter tokens into the buffer")
	}
}

func TestUpdate_StaleChunkLoadedMsgIsDropped(t *testing.T) {
	// Copilot review PR#8 #2: a chunkLoadedMsg whose stream pointer
	// doesn't match m.stream (e.g. delivered after ActionReload swapped
	// streams) must be ignored, not routed through onChunk.
	src := &fakeSource{body: "x\n", kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	other := &loader.Stream{} // pretend this came from a previous loader
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
	})
	m, _ = applyResize(m, 80, 24)
	prevStreaming := m.streaming
	prevStatus := m.status
	updated, cmd := m.Update(chunkLoadedMsg{
		chunk:  loader.Chunk{EOF: true},
		stream: other,
	})
	mm := updated.(Model)
	if cmd != nil {
		t.Errorf("stale chunk should not produce a command, got %v", cmd)
	}
	if mm.streaming != prevStreaming || mm.status != prevStatus {
		t.Errorf("stale chunk mutated model state: streaming %v→%v status %v→%v",
			prevStreaming, mm.streaming, prevStatus, mm.status)
	}
}

func TestUpdate_StaleStreamDoneMsgIsDropped(t *testing.T) {
	// Copilot review PR#8 #2: streamDoneMsg from an old stream must
	// not flip the new stream's status.
	src := &fakeSource{body: strings.Repeat("a\n", 100), kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	other := &loader.Stream{}
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
	})
	m, _ = applyResize(m, 80, 24)
	m.streaming = true
	m.status = render.StatusStreaming
	updated, _ := m.Update(streamDoneMsg{stream: other})
	mm := updated.(Model)
	if !mm.streaming {
		t.Errorf("stale streamDoneMsg flipped streaming=false on the new stream")
	}
	if mm.status != render.StatusStreaming {
		t.Errorf("stale streamDoneMsg changed status: got %v want StatusStreaming", mm.status)
	}
}

// fakePDFSource satisfies [source.Source] for the page-navigation tests.
type fakePDFSource struct {
	body  string
	pages int
}

func (f *fakePDFSource) Kind() source.Kind   { return source.KindPDF }
func (f *fakePDFSource) DisplayName() string { return "fake.pdf" }
func (f *fakePDFSource) Metadata() source.Metadata {
	return source.Metadata{LineCount: -1, PageCount: f.pages}
}
func (f *fakePDFSource) Open() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.body)), nil
}
func (f *fakePDFSource) Reopen() (io.ReadSeeker, error) {
	return strings.NewReader(f.body), nil
}

// newPDFTestModel builds a Model backed by a fake PDF with a known
// page count. The body is just a placeholder — the renderer fails text
// extraction, but the page-navigation handlers don't care because they
// branch only on Kind / PageCount.
func newPDFTestModel(t *testing.T, pages int) Model {
	t.Helper()
	src := &fakePDFSource{body: "%PDF-1.4\n%%EOF\n", pages: pages}
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

func TestUpdate_NextPageAdvancesAndResetsViewport(t *testing.T) {
	// Copilot review PR#11 round-2 #5: `]` must reset the viewport
	// scroll offsets when the page actually changes so the new page
	// doesn't render halfway down.
	m := newPDFTestModel(t, 3)
	m, _ = applyResize(m, 80, 24)
	m.viewport.SetYOffset(7)
	updated, _ := m.onNextPage()
	mm := updated.(Model)
	if mm.page != 2 {
		t.Errorf("page after `]`: got %d want 2", mm.page)
	}
	if mm.viewport.YOffset != 0 {
		t.Errorf("viewport YOffset not reset on page change: got %d", mm.viewport.YOffset)
	}
}

func TestUpdate_NextPageOnLastPageClampsAndAdvises(t *testing.T) {
	// `]` on the last page must be a no-op page-wise but surface the
	// "already on last page" advisory.
	m := newPDFTestModel(t, 2)
	m, _ = applyResize(m, 80, 24)
	m.page = 2
	updated, _ := m.onNextPage()
	mm := updated.(Model)
	if mm.page != 2 {
		t.Errorf("page should clamp at total: got %d want 2", mm.page)
	}
	if !strings.Contains(mm.statusAdvisory, "last page") {
		t.Errorf("expected last-page advisory, got %q", mm.statusAdvisory)
	}
}

func TestUpdate_PrevPageDecrementsAndResetsViewport(t *testing.T) {
	// Copilot review PR#11 round-2 #6: `[` must reset the viewport
	// scroll offsets when the page actually changes.
	m := newPDFTestModel(t, 3)
	m, _ = applyResize(m, 80, 24)
	m.page = 2
	m.viewport.SetYOffset(9)
	updated, _ := m.onPrevPage()
	mm := updated.(Model)
	if mm.page != 1 {
		t.Errorf("page after `[`: got %d want 1", mm.page)
	}
	if mm.viewport.YOffset != 0 {
		t.Errorf("viewport YOffset not reset on page change: got %d", mm.viewport.YOffset)
	}
}

func TestUpdate_PrevPageOnFirstPageClampsAndAdvises(t *testing.T) {
	m := newPDFTestModel(t, 2)
	m, _ = applyResize(m, 80, 24)
	updated, _ := m.onPrevPage()
	mm := updated.(Model)
	if mm.page != 1 {
		t.Errorf("page should clamp at 1: got %d", mm.page)
	}
	if !strings.Contains(mm.statusAdvisory, "first page") {
		t.Errorf("expected first-page advisory, got %q", mm.statusAdvisory)
	}
}

func TestUpdate_JumpToPageResetsViewport(t *testing.T) {
	// Copilot review PR#11 round-2 #1: `:N` page jump should reset
	// scroll offsets when the page actually changes.
	m := newPDFTestModel(t, 5)
	m, _ = applyResize(m, 80, 24)
	m.viewport.SetYOffset(11)
	updated, _ := m.jumpToPage(3)
	mm := updated.(Model)
	if mm.page != 3 {
		t.Errorf("jumpToPage(3): got page %d want 3", mm.page)
	}
	if mm.viewport.YOffset != 0 {
		t.Errorf("viewport YOffset not reset on jump: got %d", mm.viewport.YOffset)
	}
}

func TestUpdate_JumpToOutOfRangePageClampsToTotal(t *testing.T) {
	m := newPDFTestModel(t, 5)
	m, _ = applyResize(m, 80, 24)
	updated, _ := m.jumpToPage(99)
	mm := updated.(Model)
	if mm.page != 5 {
		t.Errorf("jumpToPage(99) on 5-page PDF: got page %d want 5", mm.page)
	}
	if !strings.Contains(mm.statusAdvisory, ">") {
		t.Errorf("expected out-of-range advisory, got %q", mm.statusAdvisory)
	}
}

// --- metaUpdatedMsg payload consumption ---

func TestUpdate_MetaUpdatedMsgCachesTotalLines(t *testing.T) {
	// Copilot review PR#13 round-2 #4 + #5: TotalLines must have an
	// observable consumer. The handler stores the payload on
	// m.totalLines; the footer then reads the cache instead of
	// hitting the buffer mutex.
	m := newTestModel(t, "alpha\nbeta\ngamma\n")
	m, _ = applyResize(m, 80, 24)
	if m.totalLines != 0 {
		t.Errorf("totalLines should start at 0, got %d", m.totalLines)
	}
	updated, _ := m.Update(metaUpdatedMsg{TotalLines: 42})
	mm := updated.(Model)
	if mm.totalLines != 42 {
		t.Errorf("metaUpdatedMsg payload not cached: got %d want 42", mm.totalLines)
	}
}

func TestUpdate_MetaUpdatedMsgZeroPayloadIsIgnored(t *testing.T) {
	// Defensive: a zero TotalLines (e.g. fired from a stream that
	// closed before any lines were read) must not blow away an
	// already-set cache.
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	m.totalLines = 5
	updated, _ := m.Update(metaUpdatedMsg{TotalLines: 0})
	if updated.(Model).totalLines != 5 {
		t.Errorf("zero payload should not stomp an existing cache; got %d want 5", updated.(Model).totalLines)
	}
}

// --- T100b: toggle handlers (ActionToggleLineNumbers,
// ActionToggleWordWrap, ActionOpenFile). ---

func TestUpdate_ToggleLineNumbersFlipsConfigAndRerenders(t *testing.T) {
	m := newTestModel(t, "alpha\nbeta\ngamma\n")
	m, _ = applyResize(m, 80, 24)
	if m.cfg == nil {
		t.Fatal("cfg should be populated by newTestModel")
	}
	before := m.cfg.LineNumbers
	beforeView := m.viewport.View()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	mm := updated.(Model)
	if mm.cfg.LineNumbers == before {
		t.Errorf("ActionToggleLineNumbers did not flip cfg.LineNumbers (still %v)", before)
	}
	// The renderer is rebuilt with the new flag — the rendered frame
	// must change so the gutter appears or disappears on the next paint.
	if mm.viewport.View() == beforeView {
		t.Errorf("ActionToggleLineNumbers did not trigger a re-render (view unchanged)")
	}
}

func TestUpdate_ToggleWordWrapFlipsConfigAndRerenders(t *testing.T) {
	// The wrap-cache invalidation contract itself (ClearWrapCaches
	// resets Line.Wrapped on every resident line) is exercised in
	// internal/loader/window_test.go where the test can poke private
	// buffer fields directly. Here we just verify that the toggle
	// path flips the config flag and triggers a fresh render frame
	// (Copilot review PR#13 #2 — no test-only affordance leaked into
	// the production LineBuffer API).
	body := strings.Repeat("alpha beta gamma delta epsilon zeta eta theta iota\n", 20)
	m := newTestModel(t, body)
	m, _ = applyResize(m, 40, 24)
	before := m.cfg.WordWrap
	beforeView := m.viewport.View()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	mm := updated.(Model)
	if mm.cfg.WordWrap == before {
		t.Errorf("ActionToggleWordWrap did not flip cfg.WordWrap (still %v)", before)
	}
	if mm.viewport.View() == beforeView {
		t.Errorf("ActionToggleWordWrap did not trigger a re-render (view unchanged)")
	}
}

func TestUpdate_ActionOpenFileOpensCommandPromptPreFilled(t *testing.T) {
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm := updated.(Model)
	if !mm.commandLine.Active {
		t.Fatal("ActionOpenFile should activate the command-line prompt")
	}
	if mm.commandLine.Prefix != ':' {
		t.Errorf("ActionOpenFile prompt prefix: got %q want ':'", mm.commandLine.Prefix)
	}
	if mm.commandLine.Buffer != "open " {
		t.Errorf("ActionOpenFile prompt buffer: got %q want \"open \"", mm.commandLine.Buffer)
	}
}

func TestUpdate_ActionOpenFileEscClosesPromptWithoutLoad(t *testing.T) {
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm := updated.(Model)
	updated2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm2 := updated2.(Model)
	if mm2.commandLine.Active {
		t.Errorf("Esc should close the open-file prompt")
	}
	if mm2.commandLine.Buffer != "" {
		t.Errorf("Esc should clear the open-file prompt buffer, got %q", mm2.commandLine.Buffer)
	}
}

func TestLoaderConfigFromConfig(t *testing.T) {
	cfg := &config.Config{MaxResidentBytes: 1024, WindowSize: 8}
	got := loaderConfigFromConfig(cfg)
	if got.MaxResidentBytes != 1024 || got.WindowSize != 8 {
		t.Errorf("loaderConfigFromConfig: got %+v want {1024, 8}", got)
	}
	nilGot := loaderConfigFromConfig(nil)
	if nilGot.MaxResidentBytes != 0 || nilGot.WindowSize != 0 {
		t.Errorf("loaderConfigFromConfig(nil): expected zero, got %+v", nilGot)
	}
}
