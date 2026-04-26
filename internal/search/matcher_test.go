// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package search

import (
	"errors"
	"strings"
	"testing"
)

func TestCompile_LiteralCaseSensitive(t *testing.T) {
	t.Parallel()
	m, err := Compile("Foo", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := m.Find("foo Foo bar Foo")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches; got %d (%+v)", len(got), got)
	}
	if got[0].Start != 4 || got[0].End != 7 {
		t.Errorf("first match: got %+v want {Start:4 End:7}", got[0])
	}
	if got[1].Start != 12 || got[1].End != 15 {
		t.Errorf("second match: got %+v want {Start:12 End:15}", got[1])
	}
}

func TestCompile_LiteralCaseInsensitive(t *testing.T) {
	t.Parallel()
	m, err := Compile("foo", false, CaseInsensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := m.Find("FOO foo Foo")
	if len(got) != 3 {
		t.Errorf("expected 3 matches; got %d (%+v)", len(got), got)
	}
}

func TestCompile_LiteralSmartCase(t *testing.T) {
	t.Parallel()
	// Lowercase query → case-insensitive
	m, err := Compile("foo", false, CaseSmart)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := m.Find("FOO Foo foo"); len(got) != 3 {
		t.Errorf("smart-case lowercase query: got %d matches (%+v) want 3", len(got), got)
	}
	// Mixed-case query → case-sensitive
	m2, err := Compile("Foo", false, CaseSmart)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := m2.Find("FOO Foo foo")
	if len(got) != 1 {
		t.Errorf("smart-case mixed query: got %d matches want 1", len(got))
	}
}

func TestCompile_RegexCaseSensitive(t *testing.T) {
	t.Parallel()
	m, err := Compile(`f[oO]+`, true, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// spellchecker:ignore-next-line
	got := m.Find("foo FoO bar")
	if len(got) != 1 {
		t.Errorf("regex case-sensitive: got %d matches (%+v) want 1", len(got), got)
	}
}

func TestCompile_RegexCaseInsensitive(t *testing.T) {
	t.Parallel()
	m, err := Compile(`foo`, true, CaseInsensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := m.Find("FOO Foo foo")
	if len(got) != 3 {
		t.Errorf("regex case-insensitive: got %d matches want 3", len(got))
	}
}

func TestCompile_RegexInvalid(t *testing.T) {
	t.Parallel()
	_, err := Compile(`[invalid`, true, CaseSensitive)
	if err == nil {
		t.Fatal("expected error from invalid regex")
	}
	if !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("expected ErrInvalidPattern wrapped, got %v", err)
	}
}

func TestCompile_EmptyQueryMatchesNothing(t *testing.T) {
	t.Parallel()
	m, err := Compile("", false, CaseSmart)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := m.Find("anything"); len(got) != 0 {
		t.Errorf("empty query should match nothing; got %+v", got)
	}
}

func TestMatcher_NoMatches(t *testing.T) {
	t.Parallel()
	m, err := Compile("xyz", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := m.Find("the quick brown fox"); len(got) != 0 {
		t.Errorf("expected no matches, got %+v", got)
	}
}

func TestMatcher_OverlappingNonGreedy(t *testing.T) {
	t.Parallel()
	// Literal matcher advances past the matched span — no overlapping
	// matches for "aa" in "aaaa". Two non-overlapping matches expected.
	m, err := Compile("aa", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := m.Find("aaaa")
	if len(got) != 2 {
		t.Errorf("expected 2 non-overlapping matches; got %d (%+v)", len(got), got)
	}
}

func TestCompile_LiteralCaseInsensitiveUnicodeOffsets(t *testing.T) {
	t.Parallel()
	// The bug we fixed: the previous case-insensitive literal matcher
	// folded the haystack via strings.ToLower then ran strings.Index
	// against the folded copy, returning indices in the *folded*
	// coordinate system. For lines that contain Unicode characters
	// whose case folding changes byte length (e.g. `İ`/U+0130 which
	// Go's strings.ToLower maps to `i`/U+0069 — 2 bytes → 1 byte),
	// those indices are off-by-N when sliced back into the original.
	//
	// After the fix the matcher delegates to the regex engine with
	// `(?i)` + regexp.QuoteMeta, so byte offsets reference the
	// original line. The strongest invariant we can assert without
	// pinning Go's exact case-folding rules is: returned offsets are
	// in range AND the slice they refer to case-folds to the query
	// (Copilot review PR#9 round-3 #4).
	m, err := Compile("foo", false, CaseInsensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// Line contains the multi-byte `İ` ahead of the substring that
	// should match — exercising the byte-offset arithmetic the bug
	// fix protects.
	line := "İstanbul Foo bar"
	matches := m.Find(line)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match in %q; got %d (%+v)", line, len(matches), matches)
	}
	got := matches[0]
	if got.Start < 0 || got.End > len(line) || got.Start >= got.End {
		t.Fatalf("match offsets out of range for %q: %+v", line, got)
	}
	span := line[got.Start:got.End]
	if !strings.EqualFold(span, "foo") {
		t.Errorf("span %q at [%d,%d] does not case-fold to %q", span, got.Start, got.End, "foo")
	}
}

func TestResolveCase_SmartHeuristic(t *testing.T) {
	t.Parallel()
	cases := []struct {
		query string
		mode  CaseMode
		want  bool
	}{
		{"foo", CaseSmart, false},
		{"Foo", CaseSmart, true},
		{"FOO", CaseSmart, true},
		{"foo", CaseSensitive, true},
		{"Foo", CaseInsensitive, false},
		{"123", CaseSmart, false},
	}
	for _, tc := range cases {
		got := resolveCase(tc.query, tc.mode)
		if got != tc.want {
			t.Errorf("resolveCase(%q, %v) = %v want %v", tc.query, tc.mode, got, tc.want)
		}
	}
}
