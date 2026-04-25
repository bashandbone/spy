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
