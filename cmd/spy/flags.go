// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
)

// ParsedFlags is the post-parse view of every flag and positional from
// contracts/cli.md. Pointer-typed fields (HighlightCap) surface "flag
// was set vs left default" so a literal `--highlight-cap=0` can disable
// highlighting (Copilot review PR#7 #28); plain bools / strings rely
// on the layered config merge in [internal/config.Load] for "is set"
// semantics.
type ParsedFlags struct {
	Help    bool
	Version bool

	Theme         string
	Vim           bool
	Lang          string
	Regex         bool
	NoColor       bool
	Graphics      string
	NoLineNumbers bool
	NoWrap        bool
	HighlightCap  *int64 // nil when --highlight-cap was not passed
	ConfigPath    string
	NoConfig      bool
	DebugPath     string

	Args []string
}

// ParseFlags parses the contracts/cli.md flag surface into a
// [ParsedFlags]. Pure: no environment access, no exits — the caller
// (main) is responsible for surfacing --help / --version and exiting.
func ParseFlags(args []string) (*ParsedFlags, error) {
	pf := &ParsedFlags{}
	fs := buildFlagSet(pf)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("flag parse: %w", err)
	}
	if pf.ConfigPath != "" && pf.NoConfig {
		return nil, errors.New("--config and --no-config are mutually exclusive")
	}
	pf.Args = fs.Args()
	return pf, nil
}

// WriteHelp prints the same FlagSet that [ParseFlags] uses, so the
// flag surface and `--help` output cannot drift (Copilot review PR#7
// #30). Includes the usage header and examples; the flag list itself
// comes from [flag.FlagSet.PrintDefaults].
//
// Stdin examples (`cat ... | spy`, `git diff | spy`) are intentionally
// omitted because Phase 2 has not yet implemented StdinSource — those
// land in US5.
func WriteHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: spy [OPTIONS] [FILE]")
	fmt.Fprintln(w, "A focused popup viewer for text, code, PDFs, and images.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	pf := &ParsedFlags{}
	fs := buildFlagSet(pf)
	fs.SetOutput(w)
	fs.PrintDefaults()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  spy README.md")
	fmt.Fprintln(w, "  spy --theme=light README.md")
	fmt.Fprintln(w, "  spy -l go ./cmd/spy/main.go")
}

// buildFlagSet wires every flag the contracts/cli.md surface defines
// into the supplied [ParsedFlags]. Shared by [ParseFlags] (which uses
// it to actually parse argv) and [WriteHelp] (which prints the same
// flags via [flag.FlagSet.PrintDefaults]).
func buildFlagSet(pf *ParsedFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("spy", flag.ContinueOnError)

	fs.BoolVar(&pf.Help, "help", false, "show this help and exit")
	fs.BoolVar(&pf.Help, "h", false, "show this help and exit (short)")
	fs.BoolVar(&pf.Version, "version", false, "show version and exit")
	fs.BoolVar(&pf.Version, "V", false, "show version and exit (short)")

	fs.StringVar(&pf.Theme, "theme", "", `theme: "auto"|"dark"|"light"|<chroma-style>`)
	fs.BoolVar(&pf.Vim, "vim", false, "enable vim keybindings")
	fs.StringVar(&pf.Lang, "lang", "", "force a Chroma lexer name")
	fs.StringVar(&pf.Lang, "l", "", "force a Chroma lexer name (short)")
	fs.BoolVar(&pf.Regex, "regex", false, "treat searches as regex by default")
	fs.BoolVar(&pf.NoColor, "no-color", false, "disable color (alias for NO_COLOR=1)")
	fs.StringVar(&pf.Graphics, "graphics", "", `graphics: "auto"|"none"|"kitty"|"iterm2"|"sixel"`)
	fs.BoolVar(&pf.NoLineNumbers, "no-line-numbers", false, "hide line numbers")
	fs.BoolVar(&pf.NoWrap, "no-wrap", false, "disable soft-wrap")
	fs.Func("highlight-cap", "disable syntax highlighting above this many bytes (set to 0 to disable entirely)", func(s string) error {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("--highlight-cap: %w", err)
		}
		if n < 0 {
			return fmt.Errorf("--highlight-cap: must be >= 0, got %d", n)
		}
		pf.HighlightCap = &n
		return nil
	})
	fs.StringVar(&pf.ConfigPath, "config", "", "config file path (overrides XDG default)")
	fs.BoolVar(&pf.NoConfig, "no-config", false, "skip loading any config file")
	fs.StringVar(&pf.DebugPath, "debug", "", "write a debug log to this path")

	return fs
}
