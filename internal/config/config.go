// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package config defines the layered configuration the viewer reads at
// startup. Precedence (highest first): CLI flags > environment >
// $XDG_CONFIG_HOME/spy/config.toml > compiled defaults.
package config

// Config is the merged view callers consume. Field tags use TOML's
// snake_case convention so [Load] can decode the on-disk schema in one
// pass; the Go field names follow Go conventions.
type Config struct {
	Theme        string `toml:"theme"`
	VimMode      bool   `toml:"vim_mode"`
	RegexDefault bool   `toml:"regex_default"`
	CaseMode     string `toml:"case_mode"`
	WordWrap     bool   `toml:"word_wrap"`
	LineNumbers  bool   `toml:"line_numbers"`
	TabWidth     int    `toml:"tab_width"`

	MaxResidentBytes  int64 `toml:"max_resident_bytes"`
	WindowSize        int   `toml:"window_size"`
	HighlightCapBytes int64 `toml:"highlight_cap_bytes"`

	Graphics string `toml:"graphics"`

	MinCols int `toml:"min_cols"`
	MinRows int `toml:"min_rows"`

	// NoColor mirrors NO_COLOR=1; loaded from env at startup, not the
	// config file. Kept on Config so renderers branch on a single source
	// of truth.
	NoColor bool `toml:"-"`

	// Keys allows users to override individual key-action bindings. The
	// merge happens in [internal/keys.ApplyOverrides] at viewer init.
	Keys map[string][]string `toml:"keys"`

	// Lang holds per-language overrides keyed by Chroma lexer name.
	Lang map[string]LangOverride `toml:"lang"`
}

// LangOverride is the subset of Config keys honoured inside a
// `[lang.<name>]` table. Excludes Keys (a global concern) and Min{Cols,
// Rows} (terminal-bound).
type LangOverride struct {
	Theme             string `toml:"theme"`
	WordWrap          bool   `toml:"word_wrap"`
	LineNumbers       bool   `toml:"line_numbers"`
	TabWidth          int    `toml:"tab_width"`
	HighlightCapBytes int64  `toml:"highlight_cap_bytes"`
}

// Defaults returns the compiled-in baseline values from contracts/config.md.
// All fields not set by the config file / env / flags fall back to this.
func Defaults() *Config {
	return &Config{
		Theme:             "auto",
		VimMode:           false,
		RegexDefault:      false,
		CaseMode:          "smart",
		WordWrap:          true,
		LineNumbers:       true,
		TabWidth:          4,
		MaxResidentBytes:  268435456, // 256 MiB
		WindowSize:        8192,
		HighlightCapBytes: 5242880, // 5 MiB
		Graphics:          "auto",
		MinCols:           80,
		MinRows:           24,
		Keys:              map[string][]string{},
		Lang:              map[string]LangOverride{},
	}
}
