// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package search

import "testing"

// Phase 2 search ships only the [State] / [Match] / [Direction] /
// [CaseMode] types — the search engine (Compile, Matcher, Scan) lands
// in US2 (T053–T054). These smoke tests pin the type vocabulary so a
// future rename surfaces here rather than in render / ui / loader
// callers.

func TestStateZeroValueIsInactive(t *testing.T) {
	var s State
	if s.Query != "" {
		t.Errorf("zero State.Query: got %q want \"\"", s.Query)
	}
	if s.Direction != DirForward {
		t.Errorf("zero State.Direction: got %v want DirForward", s.Direction)
	}
	if s.CaseMode != CaseSmart {
		t.Errorf("zero State.CaseMode: got %v want CaseSmart", s.CaseMode)
	}
}

func TestDirectionsAreDistinct(t *testing.T) {
	if DirForward == DirBackward {
		t.Errorf("DirForward and DirBackward share a value")
	}
}

func TestCaseModesAreDistinct(t *testing.T) {
	if CaseSmart == CaseSensitive || CaseSensitive == CaseInsensitive || CaseSmart == CaseInsensitive {
		t.Errorf("CaseMode values must all be distinct")
	}
}

func TestMatchFieldsRoundTrip(t *testing.T) {
	m := Match{Line: 42, Start: 3, End: 7}
	if m.Line != 42 || m.Start != 3 || m.End != 7 {
		t.Errorf("Match round-trip: got %+v", m)
	}
}
