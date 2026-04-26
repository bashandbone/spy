// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package search

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ErrInvalidPattern wraps regex compile errors so callers can detect a
// bad user-supplied pattern via [errors.Is].
var ErrInvalidPattern = errors.New("invalid pattern")

// Matcher reports the Match positions inside a single line. Implementations
// are pure: a Matcher returned by [Compile] holds no per-scan state and is
// safe for concurrent use.
type Matcher interface {
	// Find returns the Match positions inside `line`. Match.Line is left
	// at zero — the caller (Scan) fills it with the absolute source line
	// number. Returns a nil/empty slice when there are no matches.
	Find(line string) []Match
}

// Compile builds a [Matcher] for the given query, honoring the supplied
// regex flag and case mode. Smart-case (the default) downgrades to
// case-insensitive when the query is entirely lowercase; any uppercase
// rune in the query forces case-sensitive matching. An empty query
// produces a Matcher that matches nothing — useful for "clear search"
// without nil-checks at call sites.
//
// Per contracts/keys.md, the user-prompt prefix codes ( `\v` / `\V` /
// `\c` / `\C` ) are interpreted by the UI, not here — by the time
// Compile runs, the regex / caseMode booleans already reflect the
// resolved values.
func Compile(query string, regex bool, caseMode CaseMode) (Matcher, error) {
	if query == "" {
		return emptyMatcher{}, nil
	}
	cs := resolveCase(query, caseMode)
	if regex {
		return compileRegex(query, cs)
	}
	return compileLiteral(query, cs), nil
}

// resolveCase implements the smart-case heuristic plus explicit
// overrides: any uppercase rune in the query forces case-sensitive,
// otherwise the smart default is case-insensitive. CaseSensitive and
// CaseInsensitive bypass the heuristic.
func resolveCase(query string, mode CaseMode) bool {
	switch mode {
	case CaseSensitive:
		return true
	case CaseInsensitive:
		return false
	}
	// CaseSmart: any uppercase rune forces sensitive.
	for _, r := range query {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

// compileRegex wires Go's regexp engine. When `caseSensitive` is false
// we prefix the pattern with `(?i)` so the engine itself does the
// folding — this keeps PSI / Unicode case folding consistent with the
// literal path.
func compileRegex(query string, caseSensitive bool) (Matcher, error) {
	pattern := query
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPattern, err)
	}
	return &regexMatcher{re: re}, nil
}

// compileLiteral builds the literal substring matcher.
//
// Case-sensitive uses a tight strings.Index loop. Case-insensitive
// delegates to the regex engine with `(?i)` + [regexp.QuoteMeta] so
// match offsets reference the *original* haystack — not a folded
// copy. Folding via [strings.ToLower] is unsafe for highlight offsets
// because some Unicode characters change byte length under case
// folding (e.g. `İ` (U+0130, 2 bytes) → `i̇` (U+0069 U+0307, 3 bytes)),
// which would shift Start/End past the matched substring in the
// original line (Copilot review PR#9 round-3 #4). [regexp.QuoteMeta]
// escapes every regex metacharacter, so the resulting pattern is
// always a valid regex and [regexp.Compile] cannot fail here.
func compileLiteral(query string, caseSensitive bool) Matcher {
	if caseSensitive {
		return &literalMatcher{needle: query}
	}
	// Cannot return an error: QuoteMeta + (?i) is always a valid Go
	// regex; [regexp.MustCompile] would also be safe but we keep the
	// non-panicking form for symmetry with [compileRegex].
	re, _ := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	return &regexMatcher{re: re}
}

// literalMatcher does case-sensitive substring scanning.
type literalMatcher struct {
	needle string
}

func (m *literalMatcher) Find(line string) []Match {
	if m.needle == "" {
		return nil
	}
	var out []Match
	off := 0
	for {
		i := strings.Index(line[off:], m.needle)
		if i < 0 {
			return out
		}
		start := off + i
		end := start + len(m.needle)
		out = append(out, Match{Start: start, End: end})
		off = end
		if off > len(line) {
			return out
		}
	}
}

// regexMatcher wraps a compiled [regexp.Regexp]. Each Find call yields
// the byte ranges of the regex's full matches; submatch groups are not
// surfaced (US2 only highlights the entire match span).
type regexMatcher struct {
	re *regexp.Regexp
}

func (m *regexMatcher) Find(line string) []Match {
	idxs := m.re.FindAllStringIndex(line, -1)
	if len(idxs) == 0 {
		return nil
	}
	// Filter out zero-width matches: patterns like `^`, `\b`, or
	// `(?:)` produce them, but render.applyMatchHighlights skips
	// empty ranges and search navigation can't usefully jump to a
	// zero-width position. Without this filter, a `\b` query against
	// a 10 000-line file produces tens of thousands of phantom
	// matches that pollute SearchState and burn CPU during
	// highlight (Copilot review PR#9 round-2 #3).
	out := make([]Match, 0, len(idxs))
	for _, p := range idxs {
		if p[0] == p[1] {
			continue
		}
		out = append(out, Match{Start: p[0], End: p[1]})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// emptyMatcher matches nothing. Returned by [Compile] when the query is
// empty so call sites can still call Find without nil-checks.
type emptyMatcher struct{}

func (emptyMatcher) Find(_ string) []Match { return nil }
