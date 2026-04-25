// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	c := Defaults()
	if c.Theme != "auto" {
		t.Errorf("Theme: got %q want %q", c.Theme, "auto")
	}
	if c.VimMode {
		t.Errorf("VimMode default: got %v want false", c.VimMode)
	}
	if c.RegexDefault {
		t.Errorf("RegexDefault default: got %v want false", c.RegexDefault)
	}
	if c.CaseMode != "smart" {
		t.Errorf("CaseMode default: got %q want %q", c.CaseMode, "smart")
	}
	if !c.WordWrap {
		t.Errorf("WordWrap default: got %v want true", c.WordWrap)
	}
	if !c.LineNumbers {
		t.Errorf("LineNumbers default: got %v want true", c.LineNumbers)
	}
	if c.TabWidth != 4 {
		t.Errorf("TabWidth default: got %d want 4", c.TabWidth)
	}
	if c.MaxResidentBytes != 268435456 {
		t.Errorf("MaxResidentBytes default: got %d want 268435456", c.MaxResidentBytes)
	}
	if c.HighlightCapBytes != 5242880 {
		t.Errorf("HighlightCapBytes default: got %d want 5242880", c.HighlightCapBytes)
	}
	if c.Graphics != "auto" {
		t.Errorf("Graphics default: got %q want %q", c.Graphics, "auto")
	}
	if c.MinCols != 80 || c.MinRows != 24 {
		t.Errorf("Min*: got %dx%d want 80x24", c.MinCols, c.MinRows)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestLoad_TOMLOverridesDefaults(t *testing.T) {
	p := writeConfig(t, `
theme = "dark"
vim_mode = true
tab_width = 8
max_resident_bytes = 1024
`)
	cfg, warnings := Load(LoadOptions{ConfigPath: p})
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if cfg.Theme != "dark" {
		t.Errorf("Theme: got %q want %q", cfg.Theme, "dark")
	}
	if !cfg.VimMode {
		t.Errorf("VimMode: got false want true")
	}
	if cfg.TabWidth != 8 {
		t.Errorf("TabWidth: got %d want 8", cfg.TabWidth)
	}
	if cfg.MaxResidentBytes != 1024 {
		t.Errorf("MaxResidentBytes: got %d want 1024", cfg.MaxResidentBytes)
	}
	// Untouched fields keep defaults.
	if cfg.CaseMode != "smart" {
		t.Errorf("CaseMode untouched default: got %q want smart", cfg.CaseMode)
	}
}

func TestLoad_MissingFileSilentOK(t *testing.T) {
	cfg, warnings := Load(LoadOptions{ConfigPath: filepath.Join(t.TempDir(), "nope.toml")})
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings for missing default-path file: %v", warnings)
	}
	if cfg.Theme != "auto" {
		t.Errorf("Theme on missing config: got %q want %q", cfg.Theme, "auto")
	}
}

func TestLoad_ExplicitMissingFileWarns(t *testing.T) {
	p := filepath.Join(t.TempDir(), "explicit.toml")
	cfg, warnings := Load(LoadOptions{ConfigPath: p, ExplicitConfigPath: true})
	if len(warnings) == 0 {
		t.Fatalf("expected a warning for missing --config path")
	}
	if !errors.Is(warnings[0], ErrConfigNotFound) {
		t.Errorf("expected ErrConfigNotFound, got %v", warnings[0])
	}
	// On hard --config failure, we still return defaults so the caller
	// can choose to exit 2 or continue with compiled defaults.
	if cfg.Theme != "auto" {
		t.Errorf("Theme fallback: got %q want %q", cfg.Theme, "auto")
	}
}

func TestLoad_BadTOMLWarns(t *testing.T) {
	p := writeConfig(t, "not a [valid toml file")
	_, warnings := Load(LoadOptions{ConfigPath: p, ExplicitConfigPath: true})
	if len(warnings) == 0 {
		t.Errorf("expected a warning for bad TOML")
	}
}

func TestLoad_UnknownKeyWarns(t *testing.T) {
	p := writeConfig(t, `
made_up_key = "x"
theme = "dark"
`)
	cfg, warnings := Load(LoadOptions{ConfigPath: p})
	if len(warnings) == 0 {
		t.Errorf("expected a warning for unknown key")
	}
	if cfg.Theme != "dark" {
		t.Errorf("known keys still applied: got Theme=%q want %q", cfg.Theme, "dark")
	}
}

func TestLoad_BadTypeWarnsAndDefaults(t *testing.T) {
	p := writeConfig(t, `
tab_width = "eight"
`)
	cfg, warnings := Load(LoadOptions{ConfigPath: p, ExplicitConfigPath: true})
	if len(warnings) == 0 {
		t.Errorf("expected a warning for type mismatch")
	}
	if cfg.TabWidth != 4 {
		t.Errorf("TabWidth fallback: got %d want 4", cfg.TabWidth)
	}
}

func TestLoad_PerLanguageOverride(t *testing.T) {
	p := writeConfig(t, `
[lang.go]
theme = "github"

[lang.markdown]
word_wrap = true
`)
	cfg, warnings := Load(LoadOptions{ConfigPath: p})
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if cfg.Lang["go"].Theme != "github" {
		t.Errorf("lang.go theme: got %q want %q", cfg.Lang["go"].Theme, "github")
	}
	if !cfg.Lang["markdown"].WordWrap {
		t.Errorf("lang.markdown word_wrap should be true")
	}
}

func TestLoad_KeysTableOverride(t *testing.T) {
	p := writeConfig(t, `
[keys]
quit = ["x", "ctrl+q"]
`)
	cfg, warnings := Load(LoadOptions{ConfigPath: p})
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if got := cfg.Keys["quit"]; len(got) != 2 || got[0] != "x" || got[1] != "ctrl+q" {
		t.Errorf("Keys[quit]: got %v want [x ctrl+q]", got)
	}
}

func TestLoad_EnvOverridesConfig(t *testing.T) {
	p := writeConfig(t, `
theme = "dark"
vim_mode = false
`)
	t.Setenv("SPY_THEME", "light")
	t.Setenv("SPY_VIM", "1")
	cfg, _ := Load(LoadOptions{ConfigPath: p})
	if cfg.Theme != "light" {
		t.Errorf("env override Theme: got %q want light", cfg.Theme)
	}
	if !cfg.VimMode {
		t.Errorf("env override VimMode: got false want true")
	}
}

func TestLoad_NoColorForcesNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cfg, _ := Load(LoadOptions{NoConfig: true})
	if !cfg.NoColor {
		t.Errorf("NO_COLOR=1 did not set cfg.NoColor")
	}
}

func TestLoad_FlagsOverrideEnv(t *testing.T) {
	t.Setenv("SPY_THEME", "light")
	cfg, _ := Load(LoadOptions{
		NoConfig:  true,
		FlagTheme: "dark",
	})
	if cfg.Theme != "dark" {
		t.Errorf("flag should win over env: got %q want %q", cfg.Theme, "dark")
	}
}

func TestLoad_NoConfigBypassesDiscovery(t *testing.T) {
	p := writeConfig(t, `theme = "github"`)
	cfg, _ := Load(LoadOptions{NoConfig: true, ConfigPath: p})
	if cfg.Theme != "auto" {
		t.Errorf("NoConfig should ignore ConfigPath: got %q want auto", cfg.Theme)
	}
}

func TestLoad_DefaultXDGPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "spy", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`theme="github"`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, _ := Load(LoadOptions{})
	if cfg.Theme != "github" {
		t.Errorf("XDG default path: got Theme=%q want %q", cfg.Theme, "github")
	}
}

func TestLoad_FlagsForAllPointerFields(t *testing.T) {
	yes := true
	no := false
	cfg, _ := Load(LoadOptions{
		NoConfig:     true,
		FlagWordWrap: &no,
		FlagLineNums: &no,
		FlagVim:      &yes,
		FlagGraphics: "Kitty",
	})
	if cfg.WordWrap {
		t.Errorf("FlagWordWrap=false: got true")
	}
	if cfg.LineNumbers {
		t.Errorf("FlagLineNums=false: got true")
	}
	if !cfg.VimMode {
		t.Errorf("FlagVim=true: got false")
	}
	if cfg.Graphics != "kitty" {
		t.Errorf("FlagGraphics lowercases: got %q want kitty", cfg.Graphics)
	}
}

func TestLoad_FlagRegexAndHighlightCap(t *testing.T) {
	yes := true
	cfg, _ := Load(LoadOptions{
		NoConfig:         true,
		FlagRegex:        &yes,
		FlagHighlightCap: 1024,
	})
	if !cfg.RegexDefault {
		t.Errorf("FlagRegex should set cfg.RegexDefault")
	}
	if cfg.HighlightCapBytes != 1024 {
		t.Errorf("FlagHighlightCap: got %d want 1024", cfg.HighlightCapBytes)
	}
}

func TestLoad_FlagHighlightCapZeroIsUnset(t *testing.T) {
	// The flag is "unset" when the value is 0; we don't want to wipe the
	// config file's value just because the user didn't pass --highlight-cap.
	p := writeConfig(t, `highlight_cap_bytes = 9999`)
	cfg, _ := Load(LoadOptions{ConfigPath: p, FlagHighlightCap: 0})
	if cfg.HighlightCapBytes != 9999 {
		t.Errorf("zero FlagHighlightCap should not override file: got %d want 9999",
			cfg.HighlightCapBytes)
	}
}

func TestParseBool(t *testing.T) {
	cases := map[string]bool{
		"1": true, "true": true, "TRUE": true, "yes": true, "on": true,
		"0": false, "false": false, "no": false, "off": false, "": false, "garbage": false,
	}
	for in, want := range cases {
		if got := parseBool(in); got != want {
			t.Errorf("parseBool(%q): got %v want %v", in, got, want)
		}
	}
}

func TestLoad_EmptyFileIsValid(t *testing.T) {
	p := writeConfig(t, "")
	cfg, warnings := Load(LoadOptions{ConfigPath: p})
	if len(warnings) != 0 {
		t.Errorf("empty file produced warnings: %v", warnings)
	}
	if cfg.Theme != "auto" {
		t.Errorf("empty file should yield defaults, got Theme=%q", cfg.Theme)
	}
}
