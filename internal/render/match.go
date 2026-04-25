// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"strings"

	"github.com/knitli/spy/internal/search"
)

// matchesForLine returns the matches whose Line field equals `line`,
// in source-position order, plus a flag for whether the currently-active
// match falls on this line and at which Match-slice index. The returned
// slice indexes back into ctx.Search.Matches so the caller can decide
// which range gets the active style.
func matchesForLine(state search.State, line int64) []search.Match {
	if state.Query == "" || len(state.Matches) == 0 {
		return nil
	}
	var out []search.Match
	for _, m := range state.Matches {
		if m.Line == line {
			out = append(out, m)
		}
	}
	return out
}

// activeMatch returns a copy of the currently-selected match in the
// state, or the zero Match when no current match is set. Callers compare
// the returned (Line, Start, End) triple against per-line matches to
// pick the SearchActive style.
func activeMatch(state search.State) (search.Match, bool) {
	if state.Query == "" || state.CurrentMatch < 0 || state.CurrentMatch >= len(state.Matches) {
		return search.Match{}, false
	}
	return state.Matches[state.CurrentMatch], true
}

// applyMatchHighlights wraps the byte ranges named by `matches` in the
// raw line `raw` with the supplied lipgloss-style codes. Returns the
// styled line. `active` is the currently-selected match (if any) — the
// matching range gets the activeStyle wrapper instead of the hitStyle
// wrapper.
//
// The function operates on raw bytes, not the chroma-styled output, so
// it never sees the chroma ANSI escapes. The caller — the code renderer
// — chooses to skip chroma styling for matched lines and hand the raw
// line + matches here so the highlight is precise. This is the
// documented Phase 4 limitation: matched lines lose chroma syntax
// colour while highlighted (Copilot review of US2 `T057`).
func applyMatchHighlights(raw string, matches []search.Match, active search.Match, hasActive bool, hitStyle, activeStyle styleRenderer) string {
	if len(matches) == 0 {
		return raw
	}
	var b strings.Builder
	pos := 0
	for _, m := range matches {
		if m.Start < 0 || m.End <= m.Start || m.End > len(raw) {
			continue
		}
		if m.Start > pos {
			b.WriteString(raw[pos:m.Start])
		}
		seg := raw[m.Start:m.End]
		isActive := hasActive && m.Line == active.Line && m.Start == active.Start && m.End == active.End
		if isActive {
			b.WriteString(activeStyle.Render(seg))
		} else {
			b.WriteString(hitStyle.Render(seg))
		}
		pos = m.End
	}
	if pos < len(raw) {
		b.WriteString(raw[pos:])
	}
	return b.String()
}

// styleRenderer is the minimal interface match.go needs from a lipgloss
// Style — Render(string) string. Defining it as an interface keeps the
// helper testable without pulling lipgloss into the test fixtures.
type styleRenderer interface {
	Render(strs ...string) string
}
