// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Sentinel errors emitted as warnings via the `[]error` returned from
// [Load]. Callers inspect via [errors.Is]; soft failures (unknown keys,
// type mismatches, missing default-path config) are warnings, not
// fatal. Only an explicit `--config <path>` that points at a missing
// file should raise an error the caller surfaces as exit 2.
var (
	ErrConfigNotFound   = errors.New("config file not found")
	ErrConfigUnknownKey = errors.New("unknown config key")
	ErrConfigParse      = errors.New("config parse error")
)

// LoadOptions controls discovery and the per-source overrides that flow
// in *after* the file has been parsed. All fields are optional; the
// zero-value `LoadOptions{}` performs the standard XDG discovery.
type LoadOptions struct {
	// ConfigPath, when set, replaces the default XDG path. Combined
	// with ExplicitConfigPath this also signals that a missing file is
	// a hard error (per contracts/config.md "Discovery rules" #1).
	ConfigPath         string
	ExplicitConfigPath bool

	// NoConfig short-circuits all file lookup; only env+flags+defaults
	// are merged.
	NoConfig bool

	// Flag-level overrides. Empty / zero values mean "no flag was set"
	// — flags don't override env unless the user passed them. Pointer
	// types let `false` / `0` be real values rather than "unset"
	// sentinels (FlagHighlightCap accepts 0 = disable highlighting,
	// per Copilot review PR#7 #28).
	FlagTheme        string
	FlagVim          *bool
	FlagRegex        *bool
	FlagGraphics     string
	FlagWordWrap     *bool
	FlagLineNums     *bool
	FlagHighlightCap *int64
}

// Load discovers the config file (per [LoadOptions]), parses it, then
// applies env overrides and flag overrides in that order. Soft failures
// (missing default-path file, unknown keys, type mismatches) become
// entries in the returned warnings slice; the [*Config] is always
// non-nil so callers can proceed with sensible defaults regardless.
func Load(opts LoadOptions) (*Config, []error) {
	cfg := Defaults()
	var warnings []error

	if !opts.NoConfig {
		path := opts.ConfigPath
		if path == "" {
			path = defaultPath()
		}
		warnings = append(warnings, loadFile(cfg, path, opts.ExplicitConfigPath)...)
	}

	applyEnv(cfg)
	applyFlags(cfg, opts)
	return cfg, warnings
}

// defaultPath returns the standard XDG location, falling back to
// ~/.config/spy/config.toml when XDG_CONFIG_HOME is unset.
func defaultPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "spy", "config.toml")
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "spy", "config.toml")
	}
	return ""
}

// loadFile reads `path` and merges it into `cfg`. A missing file is OK
// when `explicit` is false (config files are optional); when explicit,
// the missing file is reported as an [ErrConfigNotFound] warning so
// callers can choose to surface exit 2.
func loadFile(cfg *Config, path string, explicit bool) []error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if explicit {
				return []error{fmt.Errorf("%w: %s", ErrConfigNotFound, path)}
			}
			return nil
		}
		return []error{fmt.Errorf("%w: %v", ErrConfigParse, err)}
	}
	return mergeTOML(cfg, data, explicit)
}

// mergeTOML decodes `data` over `cfg`. Unknown top-level keys produce
// [ErrConfigUnknownKey] warnings; the file is otherwise applied
// field-by-field over the supplied defaults. A type mismatch anywhere
// in the file is fatal at decode time — [BurntSushi/toml] returns a
// single decode error rather than per-field validation, so we surface
// the whole file as [ErrConfigParse]. Per-field type validation with
// fall-through to defaults would require a custom UnmarshalTOML pass
// and is deferred to a future polish task.
func mergeTOML(cfg *Config, data []byte, _ bool) []error {
	var warnings []error
	// Decode into a fresh Config so we know exactly what was set vs.
	// what came from defaults; toml.MetaData.IsDefined tells us which.
	dst := &Config{}
	meta, err := toml.Decode(string(data), dst)
	if err != nil {
		return []error{fmt.Errorf("%w: %v", ErrConfigParse, err)}
	}
	for _, undecoded := range meta.Undecoded() {
		warnings = append(warnings,
			fmt.Errorf("%w: %s", ErrConfigUnknownKey, undecoded.String()))
	}
	// Copy each defined field over the defaults.
	if meta.IsDefined("theme") {
		cfg.Theme = dst.Theme
	}
	if meta.IsDefined("vim_mode") {
		cfg.VimMode = dst.VimMode
	}
	if meta.IsDefined("regex_default") {
		cfg.RegexDefault = dst.RegexDefault
	}
	if meta.IsDefined("case_mode") {
		cfg.CaseMode = dst.CaseMode
	}
	if meta.IsDefined("word_wrap") {
		cfg.WordWrap = dst.WordWrap
	}
	if meta.IsDefined("line_numbers") {
		cfg.LineNumbers = dst.LineNumbers
	}
	if meta.IsDefined("tab_width") {
		cfg.TabWidth = dst.TabWidth
	}
	if meta.IsDefined("max_resident_bytes") {
		cfg.MaxResidentBytes = dst.MaxResidentBytes
	}
	if meta.IsDefined("window_size") {
		cfg.WindowSize = dst.WindowSize
	}
	if meta.IsDefined("highlight_cap_bytes") {
		cfg.HighlightCapBytes = dst.HighlightCapBytes
	}
	if meta.IsDefined("graphics") {
		cfg.Graphics = dst.Graphics
	}
	if meta.IsDefined("min_cols") {
		cfg.MinCols = dst.MinCols
	}
	if meta.IsDefined("min_rows") {
		cfg.MinRows = dst.MinRows
	}
	if meta.IsDefined("keys") {
		cfg.Keys = dst.Keys
	}
	if meta.IsDefined("lang") {
		cfg.Lang = dst.Lang
	}
	return warnings
}

// applyEnv layers env vars on top of file-loaded values. Only env vars
// the contract names are honored.
func applyEnv(cfg *Config) {
	if v := os.Getenv("SPY_THEME"); v != "" {
		cfg.Theme = v
	}
	if v := os.Getenv("SPY_VIM"); v != "" {
		cfg.VimMode = parseBool(v)
	}
	if v := os.Getenv("SPY_GRAPHICS"); v != "" {
		cfg.Graphics = strings.ToLower(v)
	}
	if v := os.Getenv("NO_COLOR"); v != "" {
		cfg.NoColor = true
	}
}

// applyFlags layers CLI flags on top of file+env. Each FlagX field is
// considered "set" when its zero value is meaningful (non-empty string,
// non-nil pointer); flags that weren't passed are skipped.
func applyFlags(cfg *Config, opts LoadOptions) {
	if opts.FlagTheme != "" {
		cfg.Theme = opts.FlagTheme
	}
	if opts.FlagVim != nil {
		cfg.VimMode = *opts.FlagVim
	}
	if opts.FlagRegex != nil {
		cfg.RegexDefault = *opts.FlagRegex
	}
	if opts.FlagGraphics != "" {
		cfg.Graphics = strings.ToLower(opts.FlagGraphics)
	}
	if opts.FlagWordWrap != nil {
		cfg.WordWrap = *opts.FlagWordWrap
	}
	if opts.FlagLineNums != nil {
		cfg.LineNumbers = *opts.FlagLineNums
	}
	if opts.FlagHighlightCap != nil {
		cfg.HighlightCapBytes = *opts.FlagHighlightCap
	}
}

// parseBool accepts the typical truthy values seen in env vars. Any
// other value reads as false (consistent with `NO_COLOR`-style env vars
// where any non-empty value is "true" for some, "1" for others — this
// matches the spec's loose interpretation in contracts/config.md).
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
