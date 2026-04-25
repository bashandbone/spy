// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestParseFlags_Defaults(t *testing.T) {
	pf, err := ParseFlags(nil)
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if pf.Help || pf.Version {
		t.Errorf("Help/Version default to true, got %+v", pf)
	}
	if pf.Theme != "" {
		t.Errorf("Theme default empty (env/config decides): got %q", pf.Theme)
	}
	if pf.Graphics != "" {
		t.Errorf("Graphics default empty: got %q", pf.Graphics)
	}
	if pf.Vim {
		t.Errorf("Vim default false: got true")
	}
	if pf.NoLineNumbers || pf.NoWrap {
		t.Errorf("NoLineNumbers / NoWrap default false")
	}
}

func TestParseFlags_LongForm(t *testing.T) {
	pf, err := ParseFlags([]string{
		"--theme", "dark", "--vim", "--lang", "go",
		"--regex", "--no-color", "--graphics", "kitty",
		"--no-line-numbers", "--no-wrap",
		"--highlight-cap", "1024",
		"--config", "/tmp/spy.toml",
		"--debug", "/tmp/spy.log",
		"file.go",
	})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if pf.Theme != "dark" {
		t.Errorf("Theme: got %q", pf.Theme)
	}
	if !pf.Vim {
		t.Errorf("Vim should be true")
	}
	if pf.Lang != "go" {
		t.Errorf("Lang: got %q", pf.Lang)
	}
	if !pf.Regex {
		t.Errorf("Regex should be true")
	}
	if !pf.NoColor {
		t.Errorf("NoColor should be true")
	}
	if pf.Graphics != "kitty" {
		t.Errorf("Graphics: got %q", pf.Graphics)
	}
	if !pf.NoLineNumbers || !pf.NoWrap {
		t.Errorf("NoLineNumbers/NoWrap should be true")
	}
	if pf.HighlightCap == nil || *pf.HighlightCap != 1024 {
		t.Errorf("HighlightCap: got %v want &1024", pf.HighlightCap)
	}
	if pf.ConfigPath != "/tmp/spy.toml" {
		t.Errorf("ConfigPath: got %q", pf.ConfigPath)
	}
	if pf.DebugPath != "/tmp/spy.log" {
		t.Errorf("DebugPath: got %q", pf.DebugPath)
	}
	if len(pf.Args) != 1 || pf.Args[0] != "file.go" {
		t.Errorf("Args: got %v", pf.Args)
	}
}

func TestParseFlags_ShortForm(t *testing.T) {
	pf, err := ParseFlags([]string{"-h"})
	if err != nil {
		t.Fatalf("ParseFlags -h: %v", err)
	}
	if !pf.Help {
		t.Errorf("-h should set Help")
	}

	pf, err = ParseFlags([]string{"-V"})
	if err != nil {
		t.Fatalf("ParseFlags -V: %v", err)
	}
	if !pf.Version {
		t.Errorf("-V should set Version")
	}

	pf, err = ParseFlags([]string{"-l", "python"})
	if err != nil {
		t.Fatalf("ParseFlags -l: %v", err)
	}
	if pf.Lang != "python" {
		t.Errorf("-l: Lang got %q", pf.Lang)
	}
}

func TestParseFlags_UnknownFlag(t *testing.T) {
	_, err := ParseFlags([]string{"--mystery"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestParseFlags_ConfigVsNoConfigMutex(t *testing.T) {
	_, err := ParseFlags([]string{"--config", "/x", "--no-config"})
	if err == nil {
		t.Fatal("expected error for --config and --no-config together")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error message should mention mutual exclusion: %v", err)
	}
}

func TestParseFlags_HighlightCapZero(t *testing.T) {
	// --highlight-cap=0 is a valid user-supplied value (cli.md "Set to
	// 0 to disable highlighting entirely") and must round-trip as a
	// non-nil pointer (Copilot review PR#7 #28).
	pf, err := ParseFlags([]string{"--highlight-cap", "0"})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if pf.HighlightCap == nil {
		t.Fatal("--highlight-cap=0 should produce a non-nil pointer")
	}
	if *pf.HighlightCap != 0 {
		t.Errorf("--highlight-cap=0: got %d want 0", *pf.HighlightCap)
	}
}

func TestParseFlags_HighlightCapUnset(t *testing.T) {
	pf, err := ParseFlags(nil)
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if pf.HighlightCap != nil {
		t.Errorf("unset HighlightCap should be nil, got &%d", *pf.HighlightCap)
	}
}

func TestParseFlags_HighlightCapNegativeRejected(t *testing.T) {
	_, err := ParseFlags([]string{"--highlight-cap", "-1"})
	if err == nil {
		t.Fatal("expected error for negative --highlight-cap")
	}
}

func TestParseFlags_HighlightCapBadValueRejected(t *testing.T) {
	_, err := ParseFlags([]string{"--highlight-cap", "huge"})
	if err == nil {
		t.Fatal("expected error for non-numeric --highlight-cap")
	}
}

func TestWriteHelp_IncludesAllFlags(t *testing.T) {
	var buf strings.Builder
	WriteHelp(&buf)
	out := buf.String()
	for _, name := range []string{
		"theme", "vim", "lang", "regex", "no-color", "graphics",
		"no-line-numbers", "no-wrap", "highlight-cap", "config",
		"no-config", "debug", "help", "version",
	} {
		if !strings.Contains(out, name) {
			t.Errorf("WriteHelp output missing flag %q", name)
		}
	}
	if !strings.Contains(out, "Examples:") {
		t.Errorf("WriteHelp output missing Examples section")
	}
}

func TestParseFlags_DashPositional(t *testing.T) {
	pf, err := ParseFlags([]string{"-"})
	if err != nil {
		t.Fatalf("ParseFlags -: %v", err)
	}
	if len(pf.Args) != 1 || pf.Args[0] != "-" {
		t.Errorf("Args should preserve '-': got %v", pf.Args)
	}
}

func TestParseFlags_HelpExits(t *testing.T) {
	// --help shouldn't error during parse; main.go reads .Help and exits.
	pf, err := ParseFlags([]string{"--help"})
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !pf.Help {
		t.Errorf("--help should set Help")
	}
}
