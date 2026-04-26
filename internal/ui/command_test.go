// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/search"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// numberedBody returns N lines numbered 1..N.
func numberedBody(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line ")
		b.WriteString(itoa(i))
		b.WriteByte('\n')
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// drainStream pushes every chunk through the model so search has a full
// resident buffer to scan.
func drainStream(t *testing.T, m Model) Model {
	t.Helper()
	for c := range m.stream.Updates {
		updated, _ := m.Update(chunkLoadedMsg{chunk: c, stream: m.stream})
		m = updated.(Model)
	}
	return m
}

func TestCommandLine_OpenColon(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	mm := updated.(Model)
	if !mm.commandLine.Active {
		t.Fatal(": should activate the command-line prompt")
	}
	if mm.commandLine.Prefix != ':' {
		t.Errorf("prompt prefix: got %q want :", mm.commandLine.Prefix)
	}
}

func TestCommandLine_AppendsRunes(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = updated.(Model)
	for _, r := range "42" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	if m.commandLine.Buffer != "42" {
		t.Errorf("buffer: got %q want 42", m.commandLine.Buffer)
	}
}

func TestCommandLine_BackspaceErasesRune(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	for _, r := range ":abc" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if m.commandLine.Buffer != "ab" {
		t.Errorf("after backspace: got %q want ab", m.commandLine.Buffer)
	}
}

func TestCommandLine_EscCancelsPrompt(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.commandLine.Active {
		t.Errorf("Esc should close the prompt")
	}
}

func TestCommand_JumpToLineN(t *testing.T) {
	t.Parallel()
	// Body must be large enough that the target line stays within the
	// viewport's clampable scroll range.
	m := newTestModel(t, numberedBody(300))
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	// `:42` jumps to line 42.
	for _, r := range ":42" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if got := m.viewport.YOffset; got != 41 {
		t.Errorf(":42 should set YOffset=41 (line 42, 0-indexed); got %d", got)
	}
}

func TestCommand_JumpToZeroAlias(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(300))
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	m.viewport.SetYOffset(50)
	for _, r := range ":0" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if got := m.viewport.YOffset; got != 0 {
		t.Errorf(":0 should jump to line 1 (YOffset=0); got %d", got)
	}
}

func TestCommand_JumpToDollarAlias(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(300))
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range ":$" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	// Bubble Tea's viewport clamps the requested YOffset to the
	// content-bound maximum (totalRows - height). For a 300-line body
	// with viewport height ~23, YOffset ends near 277. Assert we
	// scrolled well past the start without pinning the exact value.
	if m.viewport.YOffset < 200 {
		t.Errorf(":$ should jump near the bottom; got YOffset=%d", m.viewport.YOffset)
	}
}

func TestCommand_JumpOutOfRangeClamps(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(300))
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range ":99999" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	// Out-of-range jump clamps to the last line; viewport then clamps
	// to its content-height max. The exact YOffset varies with
	// rendering chrome, so we just assert we scrolled near the end.
	if m.viewport.YOffset < 200 {
		t.Errorf("out-of-range jump should clamp near the end; got YOffset=%d", m.viewport.YOffset)
	}
	if !strings.Contains(m.statusAdvisory, "99999") {
		t.Errorf("expected advisory mentioning 99999; got %q", m.statusAdvisory)
	}
}

func TestCommand_SetVimToggle(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	for _, r := range ":set vim" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !m.vim {
		t.Errorf(":set vim should enable vim mode")
	}
	// j should now scroll down (vim binding present).
	if !bindingMatches(m.keyMap[keys.ActionScrollDown], "j") {
		t.Errorf(":set vim should add j as scroll-down binding")
	}
	// :set novim disables.
	for _, r := range ":set novim" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.vim {
		t.Errorf(":set novim should disable vim mode")
	}
}

func TestCommand_SetVimPreservesUserOverrides(t *testing.T) {
	t.Parallel()
	// User has remapped ActionQuit to "x" (instead of the default
	// q/esc/ctrl+c). Toggling :set vim then :set novim must not
	// silently drop that remap (Copilot review PR#9 round-3 #5).
	src := &fakeSource{body: numberedBody(20), kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	base, _ := keys.ApplyOverrides(keys.Default(), map[string][]string{
		"quit": {"x"},
	})
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       base,
		BaseKeyMap:   base,
	})
	m, _ = applyResize(m, 80, 24)
	// Sanity: x is bound to quit before any toggle.
	if !bindingMatches(m.keyMap[keys.ActionQuit], "x") {
		t.Fatalf("setup: 'x' should bind ActionQuit before vim toggle")
	}
	// :set vim then :set novim — the override must still be in place.
	for _, cmd := range []string{":set vim", ":set novim"} {
		for _, r := range cmd {
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = updated.(Model)
		}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(Model)
	}
	if !bindingMatches(m.keyMap[keys.ActionQuit], "x") {
		t.Errorf(":set vim → :set novim dropped user override 'x' for quit (have %v)",
			bindingKeyList(m.keyMap[keys.ActionQuit]))
	}
}

func TestPromptLine_SuppressesAnsiInMonoTheme(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(5))
	m.theme.Mono = true
	m, _ = applyResize(m, 80, 10)
	for _, r := range ":42" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	out := m.View()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("mono theme + active prompt: View must be ANSI-free; got %q", out)
	}
	if !strings.Contains(out, ":42") {
		t.Errorf("mono prompt should still show buffer; got %q", out)
	}
}

// TestFooter_StdinDisplayName pins T094 — when the source is a
// [source.StdinSource], its DisplayName ("<stdin>") flows verbatim
// through the footer plumbing without being mistaken for a path. The
// foundational footer wraps the name with [filepath.Base], which is a
// no-op on a token containing no separators.
func TestFooter_StdinDisplayName(t *testing.T) {
	t.Parallel()
	stdinSrc := source.NewStdinSource(strings.NewReader("alpha\nbeta\n"), "")
	stream, err := loader.Open(context.Background(), stdinSrc, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open(stdin): %v", err)
	}
	m := NewModel(ModelOptions{
		Source:       stdinSrc,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
	})
	m, _ = applyResize(m, 80, 10)
	out := m.View()
	if !strings.Contains(out, "<stdin>") {
		t.Errorf("stdin footer should contain <stdin>; got %q", out)
	}
}

func TestFooter_SuppressesAnsiInColorMonoCaps(t *testing.T) {
	t.Parallel()
	// ColorMono caps (TERM=dumb / NO_COLOR=1) must suppress chrome
	// ANSI even when Theme.Mono is false (Copilot review PR#9
	// round-3 #3).
	src := &fakeSource{body: numberedBody(5), kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24, ColorDepth: term.ColorMono},
		Config:       config.Defaults(),
		Theme:        render.ThemeDark(), // not Mono
		KeyMap:       keys.Default(),
	})
	m, _ = applyResize(m, 80, 10)
	out := m.View()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("ColorMono caps: View must be ANSI-free even when Theme.Mono is false; got %q", out)
	}
}

func TestCommand_SetTheme(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	for _, r := range ":set theme light" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.theme.Name != "light" {
		t.Errorf(":set theme light: got theme %q want light", m.theme.Name)
	}
}

// TestCommand_SetThemeAutoUsesCaps confirms that `:set theme auto` —
// the runtime equivalent of the cli auto branch — re-resolves through
// [term.Capabilities.BackgroundLuminance] (T067 verifies the wiring is
// in place after T066 lands).
func TestCommand_SetThemeAutoUsesCaps(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	// Pin a "light" luminance directly on the model's caps so the auto
	// branch has something to resolve against. The Capabilities struct
	// is copied by value into the model, so reaching in here only
	// touches the test's copy.
	m.caps.BackgroundLuminance = 0.85
	m.theme = render.ThemeDark() // start in dark so the swap is observable
	m, _ = applyResize(m, 80, 24)
	for _, r := range ":set theme auto" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.theme.Name != "light" {
		t.Errorf(":set theme auto with light caps: got %q want light", m.theme.Name)
	}
}

// TestCommand_SetThemeAutoFallsBackToDark covers the symmetric case:
// NaN luminance — the "we couldn't probe" signal from [term.Detect] —
// must keep the model on the dark theme.
func TestCommand_SetThemeAutoFallsBackToDark(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m.caps.BackgroundLuminance = math.NaN()
	m.theme = render.ThemeLight()
	m, _ = applyResize(m, 80, 24)
	for _, r := range ":set theme auto" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.theme.Name != "dark" {
		t.Errorf(":set theme auto with NaN caps: got %q want dark", m.theme.Name)
	}
}

func TestCommand_QuitAlias(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	for _, r := range ":q" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(":q should produce a quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestCommand_UnknownReportsAdvisory(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	for _, r := range ":noSuchThing" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !strings.Contains(m.statusAdvisory, "unknown") {
		t.Errorf("expected 'unknown' in advisory; got %q", m.statusAdvisory)
	}
}

func TestCommand_OpenSwapsSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := "alpha\nbeta\ngamma\n"
	path := filepath.Join(dir, "fixture.txt")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m := newTestModel(t, "original\n")
	m, _ = applyResize(m, 80, 24)
	for _, r := range ":open " + path {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(":open <path> should return a cmd")
	}
	msg := cmd()
	open, ok := msg.(openResultMsg)
	if !ok {
		t.Fatalf("expected openResultMsg, got %T", msg)
	}
	if open.err != nil {
		t.Fatalf(":open returned error: %v", open.err)
	}
	updated, _ := m.Update(open)
	m = updated.(Model)
	if m.source.DisplayName() == "fake.txt" {
		t.Errorf(":open did not swap the source")
	}
	// Drain the new stream so search etc. has content; verify body line shows.
	m = drainStream(t, m)
	view := m.View()
	if !strings.Contains(view, "alpha") {
		t.Errorf("post-:open view missing fixture content; got %q", view)
	}
}

func TestSearch_ForwardJumpsToFirstMatch(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(300))
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range "/line 42" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if len(m.search.Matches) == 0 {
		t.Fatalf("/line 42 should produce matches")
	}
	if m.viewport.YOffset != 41 {
		t.Errorf("viewport should jump to match at line 42 (YOffset=41); got %d", m.viewport.YOffset)
	}
}

func TestSearch_NoMatchSurfacesAdvisory(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range "/zzznotthere" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !strings.Contains(m.statusAdvisory, "no match") {
		t.Errorf("expected 'no match' advisory; got %q", m.statusAdvisory)
	}
}

func TestSearch_NextMatchAdvancesAndWraps(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo bar\nbaz\nfoo qux\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range "/foo" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if len(m.search.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.search.Matches))
	}
	if m.search.CurrentMatch != 0 {
		t.Errorf("first match: CurrentMatch should be 0; got %d", m.search.CurrentMatch)
	}
	// `n` advances to the next match.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.search.CurrentMatch != 1 {
		t.Errorf("after n: CurrentMatch should be 1; got %d", m.search.CurrentMatch)
	}
	// `n` again wraps to 0 and sets the wrapped advisory.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.search.CurrentMatch != 0 {
		t.Errorf("after second n (wrap): CurrentMatch should be 0; got %d", m.search.CurrentMatch)
	}
	if !strings.Contains(m.statusAdvisory, "wrapped") {
		t.Errorf("expected 'wrapped' advisory after n wrap; got %q", m.statusAdvisory)
	}
}

func TestSearch_PrevMatch(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo bar\nbaz\nfoo qux\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range "/foo" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	// `N` goes back: from index 0 wraps to last index (1).
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = updated.(Model)
	if m.search.CurrentMatch != 1 {
		t.Errorf("N should wrap from 0 to last (1); got %d", m.search.CurrentMatch)
	}
}

func TestSearch_BackwardOpensWithQuestion(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo bar\nbaz\nfoo qux\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)
	if m.commandLine.Prefix != '?' {
		t.Errorf("? should open backward search prompt; got prefix %q", m.commandLine.Prefix)
	}
}

func TestSearch_RegexCompileErrorSurfaces(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults()
	cfg.RegexDefault = true
	src := &fakeSource{body: "foo\nbar\n", kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
	})
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range "/[invalid" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !strings.Contains(m.statusAdvisory, "invalid pattern") {
		t.Errorf("expected 'invalid pattern' advisory; got %q", m.statusAdvisory)
	}
}

func TestVim_GgSequenceJumpsToTop(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(50))
	m.keyMap = keys.WithVim(keys.Default())
	m.vim = true
	m, _ = applyResize(m, 80, 10)
	m = drainStream(t, m)
	m.viewport.SetYOffset(40)
	// First g arms the pending flag without scrolling.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	if m.viewport.YOffset != 40 {
		t.Errorf("single g should not jump (pending state); YOffset=%d", m.viewport.YOffset)
	}
	if !m.vimPendingG {
		t.Errorf("first g should set vimPendingG")
	}
	// Second g fires GoToTop.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	if m.viewport.YOffset != 0 {
		t.Errorf("gg should jump to top (YOffset=0); got %d", m.viewport.YOffset)
	}
	if m.vimPendingG {
		t.Errorf("vimPendingG should be cleared after gg")
	}
}

func TestVim_PendingGCancelledByOtherKey(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(50))
	m.keyMap = keys.WithVim(keys.Default())
	m.vim = true
	m, _ = applyResize(m, 80, 10)
	m = drainStream(t, m)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.vimPendingG {
		t.Errorf("non-g key should clear vimPendingG")
	}
}

func TestVim_GUppercaseGoesToBottom(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(50))
	m.keyMap = keys.WithVim(keys.Default())
	m.vim = true
	m, _ = applyResize(m, 80, 10)
	m = drainStream(t, m)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = updated.(Model)
	if m.viewport.YOffset == 0 {
		t.Errorf("G should jump to bottom; YOffset=%d", m.viewport.YOffset)
	}
}

func TestPromptHistory_RecallPreviousCommand(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(20))
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	// Submit ":5" once.
	for _, r := range ":5" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	// Open another `:` prompt; ↑ should recall "5".
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.commandLine.Buffer != "5" {
		t.Errorf("history recall: got %q want 5", m.commandLine.Buffer)
	}
}

func TestSearch_StateInRenderContext(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "foo\nbar\nfoo\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range "/foo" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	ctx := m.renderContext()
	if ctx.Search.Query != "foo" {
		t.Errorf("renderContext().Search.Query: got %q want foo", ctx.Search.Query)
	}
	if len(ctx.Search.Matches) != 2 {
		t.Errorf("renderContext().Search.Matches count: got %d want 2", len(ctx.Search.Matches))
	}
}

func TestPromptView_ShowsBuffer(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, numberedBody(5))
	m, _ = applyResize(m, 80, 10)
	for _, r := range ":42" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	view := m.View()
	if !strings.Contains(view, ":42") {
		t.Errorf("View should show :42 prompt while active; got %q", view)
	}
}

func TestStripSearchPrefixes_ForceRegex(t *testing.T) {
	t.Parallel()
	cleaned, regex, mode := stripSearchPrefixes(`\vfoo.*bar`, false, search.CaseSmart)
	if cleaned != "foo.*bar" {
		t.Errorf("cleaned: got %q want %q", cleaned, "foo.*bar")
	}
	if !regex {
		t.Errorf("\\v prefix should force regex=true")
	}
	if mode != search.CaseSmart {
		t.Errorf("\\v should not change case mode; got %v want CaseSmart", mode)
	}
}

func TestStripSearchPrefixes_ForceLiteral(t *testing.T) {
	t.Parallel()
	cleaned, regex, _ := stripSearchPrefixes(`\Vfoo.*bar`, true, search.CaseSmart)
	if cleaned != "foo.*bar" {
		t.Errorf("cleaned: got %q want %q", cleaned, "foo.*bar")
	}
	if regex {
		t.Errorf("\\V prefix should force regex=false")
	}
}

func TestStripSearchPrefixes_ForceCaseSensitivity(t *testing.T) {
	t.Parallel()
	cleaned, _, mode := stripSearchPrefixes(`\cFoo`, false, search.CaseSmart)
	if cleaned != "Foo" {
		t.Errorf("cleaned: got %q want Foo", cleaned)
	}
	if mode != search.CaseInsensitive {
		t.Errorf("\\c should force CaseInsensitive; got %v", mode)
	}

	cleaned, _, mode = stripSearchPrefixes(`\CFoo`, false, search.CaseSmart)
	if cleaned != "Foo" {
		t.Errorf("cleaned: got %q want Foo", cleaned)
	}
	if mode != search.CaseSensitive {
		t.Errorf("\\C should force CaseSensitive; got %v", mode)
	}
}

func TestStripSearchPrefixes_StackedPrefixes(t *testing.T) {
	t.Parallel()
	cleaned, regex, mode := stripSearchPrefixes(`\v\Cfoo`, false, search.CaseSmart)
	if cleaned != "foo" {
		t.Errorf("cleaned: got %q want foo", cleaned)
	}
	if !regex {
		t.Errorf("stacked \\v\\C: regex should be true")
	}
	if mode != search.CaseSensitive {
		t.Errorf("stacked \\v\\C: case mode should be CaseSensitive; got %v", mode)
	}
}

func TestStripSearchPrefixes_NoPrefix(t *testing.T) {
	t.Parallel()
	cleaned, regex, mode := stripSearchPrefixes("plain query", true, search.CaseSensitive)
	if cleaned != "plain query" {
		t.Errorf("cleaned: got %q want %q", cleaned, "plain query")
	}
	if !regex {
		t.Errorf("regex should be unchanged when no prefix; got %v", regex)
	}
	if mode != search.CaseSensitive {
		t.Errorf("case mode should be unchanged; got %v", mode)
	}
}

func TestStripSearchPrefixes_UnknownEscapeStops(t *testing.T) {
	t.Parallel()
	// `\x` is not a recognised toggle — the function leaves it intact
	// so the regex engine (or literal matcher) sees the original query.
	cleaned, _, _ := stripSearchPrefixes(`\xfoo`, false, search.CaseSmart)
	if cleaned != `\xfoo` {
		t.Errorf("unknown escape should be left intact; got %q", cleaned)
	}
}

func TestCaseModeFromConfig_RecognisedValues(t *testing.T) {
	t.Parallel()
	cases := map[string]search.CaseMode{
		"smart":       search.CaseSmart,
		"sensitive":   search.CaseSensitive,
		"insensitive": search.CaseInsensitive,
		"SMART":       search.CaseSmart, // case-insensitive parse
		"  smart  ":   search.CaseSmart, // whitespace-tolerant
	}
	for in, want := range cases {
		got := caseModeFromConfig(in, search.CaseInsensitive)
		if got != want {
			t.Errorf("caseModeFromConfig(%q) = %v want %v", in, got, want)
		}
	}
}

func TestCaseModeFromConfig_UnknownFallsBack(t *testing.T) {
	t.Parallel()
	got := caseModeFromConfig("garbage", search.CaseSmart)
	if got != search.CaseSmart {
		t.Errorf("unknown spec should fall back to fallback; got %v", got)
	}
	got = caseModeFromConfig("", search.CaseSensitive)
	if got != search.CaseSensitive {
		t.Errorf("empty spec should fall back; got %v", got)
	}
}

func TestSearch_CaseModeFromConfigOverridesSmartDefault(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults()
	cfg.CaseMode = "sensitive"
	src := &fakeSource{body: "Foo\nfoo\n", kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
	})
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	// Lowercase query "foo" with cfg.CaseMode = "sensitive" should
	// only match the lowercase line, not "Foo".
	for _, r := range "/foo" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if len(m.search.Matches) != 1 {
		t.Errorf("cfg.CaseMode=sensitive should yield 1 match for 'foo'; got %d", len(m.search.Matches))
	}
}

func TestSearch_PrefixOverridesCaseSensitivity(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults() // CaseMode = "smart" by default
	src := &fakeSource{body: "Foo\nfoo\nFOO\n", kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	m := NewModel(ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: term.Capabilities{Cols: 80, Rows: 24},
		Config:       cfg,
		Theme:        render.ThemeDark(),
		KeyMap:       keys.Default(),
	})
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	// `\Cfoo`: literal, case-sensitive. Only "foo" matches.
	for _, r := range `/\Cfoo` {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if len(m.search.Matches) != 1 {
		t.Errorf(`\C prefix should yield 1 match; got %d`, len(m.search.Matches))
	}
	if m.search.CaseMode != search.CaseSensitive {
		t.Errorf("CaseMode after \\C: got %v want CaseSensitive", m.search.CaseMode)
	}
	if m.search.Query != "foo" {
		t.Errorf("Query should be cleaned of prefix; got %q want foo", m.search.Query)
	}
}

func TestSearch_InitialJumpDoesNotSetWrappedAdvisory(t *testing.T) {
	t.Parallel()
	// Forward search from the top finds matches before any wrap, so
	// "search wrapped" must not be the advisory after the first jump
	// (Copilot review PR#9 #4).
	m := newTestModel(t, "alpha\nfoo bar\nbaz\nfoo qux\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	for _, r := range "/foo" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.search.Wrapped {
		t.Errorf("initial forward jump should not set Wrapped; got true")
	}
	if strings.Contains(m.statusAdvisory, "wrapped") {
		t.Errorf("initial forward jump should not surface 'wrapped' advisory; got %q", m.statusAdvisory)
	}
}

func TestSearch_InitialJumpFromMidFileDoesSetWrappedWhenNeeded(t *testing.T) {
	t.Parallel()
	// Forward search from a position past every match — the only hit
	// is found after wrap, so the advisory should fire.
	// Body must be large enough for the viewport to actually scroll
	// past the only match (line 1) so the scan has to wrap.
	var b strings.Builder
	b.WriteString("foo only here\n")
	for i := 2; i <= 200; i++ {
		b.WriteString("filler line ")
		b.WriteString(itoa(i))
		b.WriteByte('\n')
	}
	m := newTestModel(t, b.String())
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	m.viewport.SetYOffset(100)
	for _, r := range "/foo" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !m.search.Wrapped {
		t.Errorf("post-wrap initial jump should set Wrapped=true; got %+v", m.search)
	}
	if !strings.Contains(m.statusAdvisory, "wrapped") {
		t.Errorf("post-wrap initial jump should surface 'wrapped' advisory; got %q", m.statusAdvisory)
	}
}

func TestSearch_BackwardInitialPicksFirstHitInTraversalOrder(t *testing.T) {
	t.Parallel()
	// Backward search emits matches in descending line order; the
	// first emitted hit (index 0) is the one closest to `from` going
	// backwards, which is what the user expects to land on after the
	// initial jump (Copilot review PR#9 #5).
	m := newTestModel(t, "foo\nbar\nfoo\nbaz\nfoo\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)
	m.viewport.SetYOffset(4) // anchor near the end
	for _, r := range "?foo" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if len(m.search.Matches) == 0 {
		t.Fatalf("expected matches for backward search")
	}
	if m.search.CurrentMatch != 0 {
		t.Errorf("backward initial jump should pick index 0; got %d", m.search.CurrentMatch)
	}
}

// bindingMatches reports whether any of the bindings for an action
// includes the supplied key string.
func bindingMatches(bindings []key.Binding, want string) bool {
	for _, b := range bindings {
		for _, k := range b.Keys() {
			if k == want {
				return true
			}
		}
	}
	return false
}

// bindingKeyList flattens the keys across every binding for an action.
// Used to surface the actual keymap state when an assertion fails.
func bindingKeyList(bindings []key.Binding) []string {
	var out []string
	for _, b := range bindings {
		out = append(out, b.Keys()...)
	}
	return out
}

// Ensure search.SentinelWrapped is referenced by at least one test in
// this file so an accidental rename in package search shows up here.
var _ = search.SentinelWrapped
