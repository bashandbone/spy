// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

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

	// 1. Probe terminal capabilities; capture restore + graphics cleanup.
	caps := term.Detect(context.Background())
	restore := term.Restore()
	defer restore()
	cleanupGraphics := graphics.CleanupFunc(caps.Graphics)
	defer cleanupGraphics()

	// 2. Load layered config.
	flagVim := boolPtr(pf.Vim)
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
		FlagGraphics:       pf.Graphics,
		FlagWordWrap:       flagWordWrap,
		FlagLineNums:       flagLineNums,
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

	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "spy: tea program: %v\n", err)
		return exitGenericError
	}
	return exitOK
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
		fmt.Fprintf(os.Stderr, "spy: no input: stdin is a TTY and no FILE was given\n")
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

const helpText = `Usage: spy [OPTIONS] [FILE]
A focused popup viewer for text, code, PDFs, and images.

Options:
  -h, --help              show this help and exit
  -V, --version           show version and exit
      --theme=<value>     dark|light|auto|<chroma-style>  (default: auto)
      --vim               enable vim keybindings
  -l, --lang=<name>       force language for highlighting
      --regex             treat searches as regex
      --no-color          disable color (alias for NO_COLOR=1)
      --graphics=<value>  auto|none|kitty|iterm2|sixel    (default: auto)
      --no-line-numbers   hide line numbers
      --no-wrap           disable soft wrap
      --highlight-cap=N   disable highlighting above N bytes
      --config=<path>     config file path
      --no-config         do not load any config file
      --debug=<path>      write debug log to path

Examples:
  spy README.md
  cat main.go | spy -l go
  git diff HEAD~ | spy
`

// _ = ensure we keep the term package imported even if no live caller
// references it directly during refactors. Removed once all wiring is
// in place; harmless to keep until US3 lands.
var _ = term.Capabilities{}
