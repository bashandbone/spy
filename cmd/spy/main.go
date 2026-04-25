// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/graphics"
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
	os.Exit(run(os.Args[1:]))
}

// run is the testable entry point: takes argv (without argv[0]) and
// returns the exit code. Side effects (alt-screen, panic-safe restore,
// graphics cleanup) all live here so tests can drive the parse path
// without spinning up tea.NewProgram.
func run(args []string) int {
	pf, err := ParseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spy: %v\n", err)
		return exitUsageError
	}
	if pf.Help {
		fmt.Fprint(os.Stdout, helpText)
		return exitOK
	}
	if pf.Version {
		fmt.Fprintf(os.Stdout, "spy %s\n", version)
		return exitOK
	}

	// 1. Probe terminal capabilities. We defer Restore + cleanup AFTER
	//    cfg.Graphics has had a chance to override caps.Graphics — the
	//    cleanup closure must fire against the protocol we actually
	//    used to emit, not the auto-detected one.
	caps := term.Detect(context.Background())
	restore := term.Restore()
	defer restore()

	// 2. Load layered config.
	flagVim := boolPtr(pf.Vim)
	flagRegex := boolPtr(pf.Regex)
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
		FlagHighlightCap:   pf.HighlightCap,
	})
	for _, w := range warnings {
		if errors.Is(w, config.ErrConfigNotFound) {
			fmt.Fprintf(os.Stderr, "spy: %v\n", w)
			return exitUsageError
		}
		// Soft warnings (unknown key, type mismatch) go to stderr but
		// don't abort.
		fmt.Fprintf(os.Stderr, "spy: %v\n", w)
	}
	if pf.NoColor {
		cfg.NoColor = true
	}

	// Apply cfg.Graphics over caps.Graphics so flag/env/config overrides
	// drive the runtime cleanup defer chain. "auto" / "" leaves the
	// auto-detected protocol alone (Copilot review PR#7 #1).
	caps.Graphics = applyGraphicsOverride(caps.Graphics, cfg.Graphics)
	cleanupGraphics := graphics.CleanupFunc(caps.Graphics)
	defer cleanupGraphics()

	// 3. Pick source. FromArgs honours file paths; "-" / stdin
	//    construction is deferred to US5 so we surface ErrNoInput here
	//    as exit 2 (matching contracts/cli.md "missing argument when
	//    stdin is a TTY").
	src, err := source.FromArgs(pf.Args, os.Stdin, pf.Lang)
	if err != nil {
		return exitForSourceError(err, pf.Args)
	}

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

	// 5. Build the UI model.
	theme := render.ResolveTheme(cfg.Theme, caps, cfg.NoColor)
	keyMap := keys.Default()
	if cfg.VimMode {
		// US2 (T055) supplies the actual additive bindings; the
		// foundational keymap is identical to default.
		keyMap = keys.Default()
	}
	if len(cfg.Keys) > 0 {
		mergedKM, kerrs := keys.ApplyOverrides(keyMap, cfg.Keys)
		for _, e := range kerrs {
			fmt.Fprintf(os.Stderr, "spy: %v\n", e)
		}
		keyMap = mergedKM
	}

	model := ui.NewModel(ui.ModelOptions{
		Source:       src,
		Stream:       stream,
		Capabilities: caps,
		Config:       cfg,
		Theme:        theme,
		KeyMap:       keyMap,
		Cancel:       cancel,
	})

	// tea.WithMouseCellMotion is deferred to US1 (T046) — Phase 2 has no
	// mouse handlers and enabling tracking sequences without a consumer
	// can disrupt terminal selection / scrollback (Copilot review PR#7
	// #19).
	prog := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		// Cancel the loader so its background goroutine doesn't leak
		// past the program's error path (Copilot review PR#7 #2).
		cancel()
		fmt.Fprintf(os.Stderr, "spy: tea program: %v\n", err)
		return exitGenericError
	}
	return exitOK
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
func exitForSourceError(err error, args []string) int {
	target := "<no input>"
	if len(args) > 0 {
		target = args[0]
	}
	switch {
	case errors.Is(err, source.ErrNoInput):
		// Phase 2 returns ErrNoInput regardless of stdin TTY state
		// because StdinSource lands in US5; the message reflects the
		// real condition (Copilot review PR#7 #18).
		fmt.Fprintf(os.Stderr, "spy: no input: missing FILE; stdin is not supported yet\n")
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
		fmt.Fprintf(os.Stderr, "spy: unsupported format: %s: %v\n", target, err)
		return exitUnsupported
	}
	fmt.Fprintf(os.Stderr, "spy: %v\n", err)
	return exitGenericError
}

func boolPtr(b bool) *bool {
	if !b {
		return nil
	}
	return &b
}

// helpText is the --help output. The stdin examples (`cat ... | spy`,
// `git diff | spy`) from contracts/cli.md are intentionally omitted
// here because Phase 2 does not yet implement StdinSource — those
// examples return cleanly when US5 (T090–T094) wires up stdin and
// helpText is updated then.
//
// `--debug` is parsed and accepted (so the flag never reads as
// "unknown") but the structured-JSON debug log it documents in
// contracts/cli.md "Debug log format" lands as part of the polish
// phase. It is omitted from helpText for now.
const helpText = `Usage: spy [OPTIONS] [FILE]
A focused popup viewer for text, code, PDFs, and images.

Options:
  -h, --help              show this help and exit
  -V, --version           show version and exit
      --theme=<value>     dark|light|auto|<chroma-style>  (default: auto)
      --vim               enable vim keybindings
  -l, --lang=<name>       force language for highlighting
      --regex             treat searches as regex by default
      --no-color          disable color (alias for NO_COLOR=1)
      --graphics=<value>  auto|none|kitty|iterm2|sixel    (default: auto)
      --no-line-numbers   hide line numbers
      --no-wrap           disable soft wrap
      --highlight-cap=N   disable highlighting above N bytes
      --config=<path>     config file path
      --no-config         do not load any config file

Examples:
  spy README.md
  spy --theme=light README.md
  spy -l go ./cmd/spy/main.go
`
