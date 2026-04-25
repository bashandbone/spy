// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// ParsedFlags is the post-parse view of every flag and positional from
// contracts/cli.md. Pointer-typed fields (Vim, NoColor, etc.) would
// surface "flag was set vs left default" but the contract is content
// with bools — config / env precedence is layered after parsing in
// internal/config.Load.
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
	HighlightCap  int64
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
	fs := flag.NewFlagSet("spy", flag.ContinueOnError)
	// Suppress flag's automatic "flag provided but not defined" message
	// — we surface our own error wrapping for the test surface.
	fs.SetOutput(io.Discard)

	fs.BoolVar(&pf.Help, "help", false, "show help and exit")
	fs.BoolVar(&pf.Help, "h", false, "show help and exit (short)")
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
	fs.Int64Var(&pf.HighlightCap, "highlight-cap", 0, "disable syntax highlighting above this many bytes (0 = use config / default)")
	fs.StringVar(&pf.ConfigPath, "config", "", "config file path (overrides XDG default)")
	fs.BoolVar(&pf.NoConfig, "no-config", false, "skip loading any config file")
	fs.StringVar(&pf.DebugPath, "debug", "", "write a debug log to this path")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("flag parse: %w", err)
	}

	if pf.ConfigPath != "" && pf.NoConfig {
		return nil, errors.New("--config and --no-config are mutually exclusive")
	}

	pf.Args = fs.Args()
	return pf, nil
}
