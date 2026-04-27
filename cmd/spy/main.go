// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	xterm "golang.org/x/term"

	"github.com/alecthomas/chroma/v2/styles"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/graphics"
	"github.com/knitli/spy/internal/highlight"
	"github.com/knitli/spy/internal/keys"
	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/render"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
	"github.com/knitli/spy/internal/ui"
)

// Build-time injectable version string. Linked via -ldflags at release.
var version = "0.1.0"

// Exit codes from contracts/cli.md.
const (
	exitOK           = 0
	exitGenericError = 1
	exitUsageError   = 2
	exitIOError      = 3
	exitUnsupported  = 4
	exitTTYError     = 5
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin))
}

// run is the testable entry point: takes argv (without argv[0]) and a
// stdin handle, returning the exit code. Side effects (alt-screen,
// panic-safe restore, graphics cleanup) all live here so tests can
// drive the parse path without spinning up tea.NewProgram.
//
// Tests pass nil for `stdin` to exercise the "no input available"
// branch deterministically; production wiring threads `os.Stdin`
// through so the contract from contracts/cli.md "Stdin behavior"
// applies.
func run(args []string, stdin *os.File) int {
	pf, err := ParseFlags(args)
	if err != nil {
		// Flag parse errors can include the user's argv verbatim
		// (e.g. "unknown flag: --\x1b]2;evil\x07"). Sanitize before
		// stderr — acceptance review C4.
		fmt.Fprintf(os.Stderr, "spy: %s\n", render.Neutralize(err.Error()))
		return exitUsageError
	}
	if pf.Help {
		WriteHelp(os.Stdout)
		return exitOK
	}
	if pf.Version {
		fmt.Fprintf(os.Stdout, "spy %s\n", version)
		return exitOK
	}

	// Tmux popup dispatch: if we're inside tmux, stdin is present and is
	// a TTY (not piped), and the user hasn't opted out, re-exec via tmux
	// display-popup so spy occupies the full terminal window rather than
	// the current pane. The PopupSentinelEnv variable is injected into
	// the shell command so the inner process skips this block.
	if os.Getenv(term.PopupSentinelEnv) == "" &&
		!pf.NoPopup &&
		os.Getenv("TMUX") != "" &&
		stdin != nil && xterm.IsTerminal(int(stdin.Fd())) {
		if exitCode, err := term.LaunchTmuxPopup(args); err == nil {
			return exitCode
		}
		// tmux unavailable or display-popup failed to start → fall
		// through to normal pager mode rather than exiting with an error.
	}

	// 1. Probe terminal capabilities. We defer Restore + cleanup AFTER
	//    cfg.Graphics has had a chance to override caps.Graphics — the
	//    cleanup closure must fire against the protocol we actually
	//    used to emit, not the auto-detected one.
	caps := term.Detect(context.Background())
	restore := term.Restore()
	defer restore()

	// 2. Load layered config.
	//
	// For --vim and --regex we differentiate "flag actually passed"
	// from "left at default" via ParsedFlags.FlagWasSet — a user
	// passing --vim=false to override their TOML must propagate
	// &false (not nil). nil means "not set" in the config layer
	// and falls back to TOML / built-in default (acceptance review
	// LOW-3).
	flagVim := flagBoolPtr(pf, "vim", pf.Vim)
	flagRegex := flagBoolPtr(pf, "regex", pf.Regex)
	var flagWordWrap, flagLineNums *bool
	if pf.NoWrap {
		f := false
		flagWordWrap = &f
	}
	if pf.NoLineNumbers {
		f := false
		flagLineNums = &f
	}
	cfg, warnings := config.Load(config.LoadOptions{
		ConfigPath:         pf.ConfigPath,
		ExplicitConfigPath: pf.ConfigPath != "",
		NoConfig:           pf.NoConfig,
		FlagTheme:          pf.Theme,
		FlagVim:            flagVim,
		FlagRegex:          flagRegex,
		FlagGraphics:       pf.Graphics,
		FlagWordWrap:       flagWordWrap,
		FlagLineNums:       flagLineNums,
		FlagHighlightCap:   pf.HighlightCap, // *int64; nil when unset, &0 disables
	})
	for _, w := range warnings {
		// Config warnings can quote user-supplied TOML key/value text
		// (e.g. "unknown key 'foo\x1b]2;evil\x07' in [keys]").
		// Sanitize before stderr — acceptance review C4.
		safeW := render.Neutralize(w.Error())
		if errors.Is(w, config.ErrConfigNotFound) {
			fmt.Fprintf(os.Stderr, "spy: %s\n", safeW)
			return exitUsageError
		}
		// Soft warnings (unknown key, type mismatch) go to stderr but
		// don't abort.
		fmt.Fprintf(os.Stderr, "spy: %s\n", safeW)
	}
	if pf.NoColor {
		cfg.NoColor = true
	}

	// Apply cfg.Graphics over caps.Graphics so flag/env/config overrides
	// drive the runtime cleanup defer chain. "auto" / "" leaves the
	// auto-detected protocol alone (Copilot review PR#7 #1).
	caps.Graphics = applyGraphicsOverride(caps.Graphics, cfg.Graphics)

	// 3. Pick source. FromArgs handles the full resolution table from
	//    contracts/cli.md: file paths, "-" forced stdin, auto-stdin
	//    when stdin is non-TTY and no FILE was given. ErrNoInput here
	//    means "missing FILE and stdin is a TTY" — exit 2.
	src, err := source.FromArgs(pf.Args, stdin, pf.Lang)
	if err != nil {
		return exitForSourceError(err, pf.Args)
	}

	// Non-TTY stdout falls back to degenerate-cat (verbatim copy, exit
	// 0) per contracts/cli.md "Stdout / stderr / exit codes". exit 5
	// is reserved for explicitly TTY-required paths (Copilot review
	// PR#7 #29). The graphics cleanup defer is deliberately registered
	// AFTER this branch — on the non-TTY path no graphics escapes were
	// emitted (the runDegenerate copier just streams bytes), so firing
	// the cleanup escape would pollute stdout (Phase 6 US4: the Kitty
	// "delete all images" sequence is a real protocol message that
	// shows up as garbage when piped).
	// Skip degenerate mode when running inside a tmux popup: the popup
	// PTY may not pass IsTerminal checks on some platforms (e.g. WSL2)
	// even though a real terminal is present. The sentinel guarantees we
	// were launched by LaunchTmuxPopup and have a usable PTY.
	if !caps.IsTTY && os.Getenv(term.PopupSentinelEnv) == "" {
		return runDegenerate(src)
	}

	// 3a. Now that we know the TUI will actually start, register the
	//     graphics cleanup defer so it fires on tea.Quit, on SIGINT,
	//     AND on panic (research R10).
	cleanupGraphics := graphics.CleanupFunc(caps.Graphics)
	defer cleanupGraphics()

	// 4. Open the loader stream.
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := loader.Open(ctx, src, loader.Config{
		MaxResidentBytes: cfg.MaxResidentBytes,
		WindowSize:       cfg.WindowSize,
	})
	if err != nil {
		cancel()
		return exitForSourceError(err, pf.Args)
	}

	// 5. Build the UI model. The OSC 11 luminance probe only fires
	//    when the resolved theme spec is "auto" — explicit dark/light
	//    or a named Chroma style throws the result away, so we'd be
	//    paying the 50 ms budget for nothing (Copilot review PR#10
	//    round-3 #2). NoColor also short-circuits the probe because
	//    Mono mode wins regardless of the underlying theme.
	if !cfg.NoColor && wantsAutoTheme(cfg.Theme) {
		caps.BackgroundLuminance = term.DetectBackgroundLuminance(context.Background())
	}
	theme := render.ResolveTheme(cfg.Theme, caps, cfg.NoColor)
	// Build the *base* keymap first: defaults + any user [keys]
	// overrides. Vim is layered on top of that base so a runtime
	// `:set novim` can restore the base without losing the user's
	// overrides (Copilot review PR#9 round-3 #5).
	baseKeyMap := keys.Default()
	if len(cfg.Keys) > 0 {
		mergedKM, kerrs := keys.ApplyOverrides(baseKeyMap, cfg.Keys)
		for _, e := range kerrs {
			// Keymap-override errors quote user-supplied action /
			// key strings from cfg.Keys. Sanitize before stderr —
			// acceptance review C4.
			fmt.Fprintf(os.Stderr, "spy: %s\n", render.Neutralize(e.Error()))
		}
		baseKeyMap = mergedKM
	}
	keyMap := baseKeyMap
	if cfg.VimMode {
		keyMap = keys.WithVim(baseKeyMap)
	}

	// Construct the per-session highlighter. nil for KindText / KindBinary
	// is fine — the renderers branch on a nil highlighter and emit raw
	// content.
	highlighter := newHighlighter(theme, caps, cfg.HighlightCapBytes)

	model := ui.NewModel(ui.ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: caps,
		Config:       cfg,
		Theme:        theme,
		KeyMap:       keyMap,
		BaseKeyMap:   baseKeyMap,
		Highlighter:  highlighter,
		Cancel:       cancel,
	})

	// Install our own SIGINT/SIGTERM handler BEFORE tea.NewProgram so
	// we can return the documented exit codes (130 for SIGINT, 143
	// for SIGTERM per contracts/cli.md). Without
	// tea.WithoutSignalHandler, Bubble Tea's internal handler catches
	// SIGINT and converts to tea.Quit — prog.Run then returns nil
	// (or a generic error) and we'd exit 0/1 instead of 130. SIGTERM
	// receives no special handling from Bubble Tea v1, but the
	// process would still exit 0 because tea.Quit short-circuits
	// before the signal can terminate the process.
	//
	// signal.Notify also suppresses Go's default signal behavior, so
	// the program won't be torn down before our deferred
	// term.Restore + graphics.CleanupFunc chain fires.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// US1 enables MouseCellMotion so future search-prompt UI can react to
	// click-to-scroll. Bubble Tea handles SIGWINCH internally via its
	// terminal-renderer goroutine, so no extra option is required.
	prog := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithoutSignalHandler(),
	)

	// Goroutine: when a signal arrives, translate it to tea.Quit so
	// the alt-screen exit and graphics cleanup escapes still fire via
	// Bubble Tea's normal teardown. The signal value is stashed in
	// `caught` so the post-Run path can decide between exit-on-signal
	// (128+signum) and exit-on-clean-quit (exitOK).
	//
	// `caught` is buffered (cap 1) so the send never blocks. A
	// separate `done` channel lets the forwarding goroutine exit
	// cleanly when run() returns without receiving a signal —
	// `signal.Stop(sigCh)` unregisters delivery but does not close
	// sigCh, so a bare `<-sigCh` would leak the goroutine across
	// every test invocation that exercises run() (Copilot review
	// PR#15 #8).
	caught := make(chan os.Signal, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case sig, ok := <-sigCh:
			if !ok {
				return
			}
			caught <- sig
			// Send the QuitMsg directly. `tea.Quit` is a Cmd
			// constructor (a func returning a Msg) — passing it
			// as `prog.Send(tea.Quit())` would deliver the Cmd
			// itself as a Msg, which the update loop ignores.
			// The QuitMsg value is what the renderer's teardown
			// path watches for.
			prog.Send(tea.QuitMsg{})
		case <-done:
			return
		}
	}()

	if _, err := prog.Run(); err != nil {
		// Cancel the loader so its background goroutine doesn't leak
		// past the program's error path (Copilot review PR#7 #2).
		cancel()
		// A signal that fired DURING Run still wins — return
		// 128+signum so the caller sees the documented exit code
		// even if Bubble Tea's teardown surfaced an error.
		if sig := drainCaughtSignal(caught); sig != 0 {
			return 128 + int(sig)
		}
		// Bubble Tea errors aren't ordinarily user-controlled, but a
		// rendering error chain can wrap upstream string content
		// (e.g. file path in a renderer-init failure). Sanitize
		// defensively — acceptance review C4.
		fmt.Fprintf(os.Stderr, "spy: tea program: %s\n", render.Neutralize(err.Error()))
		return exitGenericError
	}

	// Clean tea.Run exit. If a signal triggered it, return the
	// matching exit code; otherwise the user pressed q/esc and we
	// return exitOK.
	if sig := drainCaughtSignal(caught); sig != 0 {
		return 128 + int(sig)
	}
	return exitOK
}

// drainCaughtSignal returns the signal number received on `caught`
// without blocking, or 0 if the channel is empty. Exit codes per
// `contracts/cli.md`: SIGINT (signal 2) → 130; SIGTERM (signal 15)
// → 143.
func drainCaughtSignal(caught <-chan os.Signal) syscall.Signal {
	select {
	case sig := <-caught:
		if s, ok := sig.(syscall.Signal); ok {
			return s
		}
	default:
	}
	return 0
}

// runDegenerate is the contracts/cli.md "stdout (non-TTY)" path:
// open the source and stream its bytes verbatim to stdout, exit 0.
// No alt-screen, no rendering, no graphics — `spy file.go > out.txt`
// behaves like `cat file.go > out.txt`.
//
// The DisplayName passed to exitForSourceError is already sanitized
// inside that helper. The error is sanitized before reaching stderr.
// Acceptance review C4.
func runDegenerate(src source.Source) int {
	rc, err := src.Open()
	if err != nil {
		return exitForSourceError(err, []string{src.DisplayName()})
	}
	defer rc.Close()
	if _, err := io.Copy(os.Stdout, rc); err != nil {
		fmt.Fprintf(os.Stderr, "spy: write stdout: %s\n", render.Neutralize(err.Error()))
		return exitIOError
	}
	return exitOK
}

// newHighlighter resolves the Chroma style named on the active Theme
// and constructs a [highlight.Highlighter] with the active color depth
// and HighlightCap. An unknown / missing style name automatically
// resolves to Chroma's bundled Fallback style (chroma's [styles.Get]
// guarantees a non-nil result) so a user-mistyped --theme=foobar still
// produces a working session — Theme.Mono separately disables
// rendering when --no-color was passed.
func newHighlighter(theme render.Theme, caps term.Capabilities, capBytes int64) *highlight.Highlighter {
	return highlight.New(styles.Get(theme.ChromaStyle), caps.ColorDepth, capBytes)
}

// wantsAutoTheme reports whether `cfg.Theme` resolves through the
// auto-detect branch (and therefore needs a background luminance
// reading). An empty value or the literal "auto" qualifies; explicit
// dark/light and named Chroma styles bypass the probe.
func wantsAutoTheme(spec string) bool {
	switch strings.ToLower(strings.TrimSpace(spec)) {
	case "", "auto":
		return true
	}
	return false
}

// applyGraphicsOverride layers cfg.Graphics on top of the auto-detected
// caps.Graphics. "" / "auto" leaves the auto value; anything else
// matching the contracts/cli.md vocabulary replaces it. The match is
// case-insensitive so config/env/flag inputs all behave the same way
// regardless of caller capitalization (Copilot review PR#7 #17).
// Unknown values are treated as "auto" (caller already surfaced any
// warnings).
func applyGraphicsOverride(detected term.Graphics, override string) term.Graphics {
	switch strings.ToLower(strings.TrimSpace(override)) {
	case "none":
		return term.GraphicsNone
	case "kitty":
		return term.GraphicsKitty
	case "iterm", "iterm2":
		return term.GraphicsITerm2
	case "sixel":
		return term.GraphicsSixel
	}
	return detected
}

// exitForSourceError maps a source-layer error to the documented exit
// code from contracts/cli.md. The stderr line uses the
// "spy: <reason>: <detail>" format the contract requires.
//
// `target` is sanitized through [render.Neutralize] before reaching
// stderr because it's user-controlled input (the filename arg) and a
// hostile path containing `\x1b]2;evil\x07` would otherwise drive the
// terminal title before alt-screen even starts. Same for the wrapped
// error string. Acceptance review C4.
func exitForSourceError(err error, args []string) int {
	target := "<no input>"
	if len(args) > 0 {
		target = render.Neutralize(args[0])
	}
	safeErr := render.Neutralize(err.Error())
	switch {
	case errors.Is(err, source.ErrNoInput):
		// US5 turned StdinSource on: ErrNoInput now reflects the
		// real "no FILE and stdin is a TTY (or absent)" condition.
		// contracts/cli.md row "absent no yes yes" expects exit 2
		// with usage printed (Copilot review PR#12 round-3 #8) — the
		// short error line precedes the full --help output so the
		// user sees both the cause and the available flags.
		fmt.Fprintf(os.Stderr, "spy: no input: missing FILE; pipe content via stdin or pass a path\n\n")
		WriteHelp(os.Stderr)
		return exitUsageError
	case errors.Is(err, source.ErrAmbiguousArgs):
		// `-` alongside FILE, or multiple FILEs — the contract row
		// "present yes — yes" is a usage error mapped to exit 2.
		fmt.Fprintf(os.Stderr, "spy: usage: %s\n", safeErr)
		return exitUsageError
	case errors.Is(err, source.ErrNotFound):
		fmt.Fprintf(os.Stderr, "spy: cannot open: %s: not found\n", target)
		return exitIOError
	case errors.Is(err, source.ErrPermission):
		fmt.Fprintf(os.Stderr, "spy: cannot open: %s: permission denied\n", target)
		return exitIOError
	case errors.Is(err, source.ErrBinary):
		fmt.Fprintf(os.Stderr, "spy: binary file: %s: refusing to render binary content\n", target)
		return exitUnsupported
	case errors.Is(err, source.ErrUnsupported):
		fmt.Fprintf(os.Stderr, "spy: unsupported format: %s: %s\n", target, safeErr)
		return exitUnsupported
	}
	fmt.Fprintf(os.Stderr, "spy: %s\n", safeErr)
	return exitGenericError
}

// flagBoolPtr returns a pointer to `val` when the user explicitly
// passed `name` on the command line, otherwise nil. The downstream
// config layer treats nil as "not set" so it falls back to the
// TOML value or the built-in default; a non-nil pointer (even
// `&false`) wins over both.
//
// Replaces the historical boolPtr(false) → nil shortcut, which
// conflated "user passed --vim=false" with "user didn't pass --vim"
// and silently dropped explicit-disable overrides (acceptance
// review LOW-3).
func flagBoolPtr(pf *ParsedFlags, name string, val bool) *bool {
	if !pf.FlagWasSet(name) {
		return nil
	}
	return &val
}
