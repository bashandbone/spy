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

// TestFlagWasSet_TracksExplicitFlags pins LOW-3: ParsedFlags.SetFlags
// records every flag the user explicitly passed, regardless of whether
// the flag's value matches its default. This is what lets main
// distinguish "user passed --vim=false" from "user did not pass --vim".
func TestFlagWasSet_TracksExplicitFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want map[string]bool // expected FlagWasSet for each name
	}{
		{
			name: "no flags",
			args: nil,
			want: map[string]bool{"vim": false, "regex": false},
		},
		{
			name: "vim true",
			args: []string{"--vim"},
			want: map[string]bool{"vim": true, "regex": false},
		},
		{
			name: "vim explicit false",
			args: []string{"--vim=false"},
			want: map[string]bool{"vim": true, "regex": false},
		},
		{
			name: "regex explicit true",
			args: []string{"--regex=true"},
			want: map[string]bool{"vim": false, "regex": true},
		},
		{
			name: "both explicit",
			args: []string{"--vim=false", "--regex=true"},
			want: map[string]bool{"vim": true, "regex": true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pf, err := ParseFlags(tc.args)
			if err != nil {
				t.Fatalf("ParseFlags(%v): %v", tc.args, err)
			}
			for name, want := range tc.want {
				if got := pf.FlagWasSet(name); got != want {
					t.Errorf("FlagWasSet(%q) = %v, want %v (args=%v)", name, got, want, tc.args)
				}
			}
		})
	}
}

// TestFlagBoolPtr_DistinguishesUnsetFromExplicitFalse pins LOW-3:
// flagBoolPtr returns nil when the flag was not passed (so config
// layer falls back to TOML/default), but a non-nil &false when the
// user explicitly passed --vim=false (so the override actually wins).
func TestFlagBoolPtr_DistinguishesUnsetFromExplicitFalse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		args      []string
		flag      string
		wantNil   bool
		wantValue bool
	}{
		{
			name:    "vim unset → nil",
			args:    nil,
			flag:    "vim",
			wantNil: true,
		},
		{
			name:      "vim=true → &true",
			args:      []string{"--vim"},
			flag:      "vim",
			wantNil:   false,
			wantValue: true,
		},
		{
			name:      "vim=false → &false (NOT nil)",
			args:      []string{"--vim=false"},
			flag:      "vim",
			wantNil:   false,
			wantValue: false,
		},
		{
			name:      "regex=false → &false (NOT nil)",
			args:      []string{"--regex=false"},
			flag:      "regex",
			wantNil:   false,
			wantValue: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pf, err := ParseFlags(tc.args)
			if err != nil {
				t.Fatalf("ParseFlags(%v): %v", tc.args, err)
			}
			var val bool
			switch tc.flag {
			case "vim":
				val = pf.Vim
			case "regex":
				val = pf.Regex
			default:
				t.Fatalf("unknown flag %q in test case", tc.flag)
			}
			ptr := flagBoolPtr(pf, tc.flag, val)
			if tc.wantNil {
				if ptr != nil {
					t.Errorf("flagBoolPtr(%s) = &%v, want nil", tc.flag, *ptr)
				}
				return
			}
			if ptr == nil {
				t.Fatalf("flagBoolPtr(%s) = nil, want &%v", tc.flag, tc.wantValue)
			}
			if *ptr != tc.wantValue {
				t.Errorf("flagBoolPtr(%s) = &%v, want &%v", tc.flag, *ptr, tc.wantValue)
			}
		})
	}
}

// TestFlagWasSet_HandlesNilParsedFlags guards FlagWasSet against the
// hand-constructed-ParsedFlags-in-tests case (SetFlags map nil).
func TestFlagWasSet_HandlesNilParsedFlags(t *testing.T) {
	t.Parallel()
	var pf *ParsedFlags
	if pf.FlagWasSet("vim") {
		t.Error("FlagWasSet on nil *ParsedFlags should return false")
	}
	pf2 := &ParsedFlags{}
	if pf2.FlagWasSet("vim") {
		t.Error("FlagWasSet on ParsedFlags with nil SetFlags should return false")
	}
}
