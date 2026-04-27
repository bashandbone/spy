// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package term

import (
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
		{"a'b'c", `'a'\''b'\''c'`},
		{"/usr/bin/spy", "'/usr/bin/spy'"},
		{"--flag=value", "'--flag=value'"},
	}
	for _, tc := range cases {
		got := shellQuote(tc.in)
		if got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestShellQuote_SafeForShell(t *testing.T) {
	// Verify that single-quoting a string with embedded single quotes
	// produces a valid shell token by checking that the output only ever
	// contains the '\'' escape and is properly bookended.
	tricky := "don't stop; rm -rf / # evil"
	q := shellQuote(tricky)
	if !strings.HasPrefix(q, "'") {
		t.Errorf("shellQuote result must start with single quote, got %q", q)
	}
	// The result must not contain an unescaped literal single quote
	// (i.e. a ' that isn't part of the '\'' escape sequence).
	stripped := strings.ReplaceAll(q, `'\''`, "")
	stripped = strings.TrimPrefix(stripped, "'")
	stripped = strings.TrimSuffix(stripped, "'")
	if strings.Contains(stripped, "'") {
		t.Errorf("shellQuote result contains unescaped single quote: %q", q)
	}
}

func TestPopupSentinelEnv(t *testing.T) {
	// Constant must not be empty and must be a valid env var name.
	if PopupSentinelEnv == "" {
		t.Fatal("PopupSentinelEnv must not be empty")
	}
	for _, c := range PopupSentinelEnv {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			t.Errorf("PopupSentinelEnv %q contains non-identifier character %q", PopupSentinelEnv, c)
		}
	}
}
