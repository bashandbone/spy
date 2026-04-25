// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package search

// Match identifies a single matching range inside a line.
type Match struct {
	Line  int64
	Start int
	End   int
}

// Direction indicates forward/backward search travel.
type Direction int

const (
	DirForward Direction = iota
	DirBackward
)

// CaseMode controls case-sensitivity behaviour. SmartCase lowercases
// queries to "case-insensitive" and any non-lowercase character forces
// case-sensitive matching.
type CaseMode int

const (
	CaseSmart CaseMode = iota
	CaseSensitive
	CaseInsensitive
)

// State is the per-frame view of the active search; consumed by render
// and surfaced in ui. Inactive when Query == "". The full search engine
// (Compile, Matcher, Scan) lands in US2 (T053–T054); this skeleton
// gives render and ui a stable type to pass through during Phase 2.
type State struct {
	Query        string
	Direction    Direction
	Regex        bool
	CaseMode     CaseMode
	Matches      []Match
	CurrentMatch int  // -1 when no match selected
	Wrapped      bool // last navigation wrapped around
	Pending      bool // a background scan is still running
}
