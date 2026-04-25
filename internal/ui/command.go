// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

// CommandLineState captures the per-frame view of an active `:` / `/`
// `?` prompt. data-model.md provides higher-level documentation, but
// the canonical Go type signature is defined here so the UI's Update
// path has a single source of truth.
//
// The schema deliberately keeps two history slices (`HistoryColon` and
// `HistorySlash`) rather than one shared ring: `↑` from a `:` prompt
// must not surface recent `/` searches, otherwise the recall flow
// becomes noisy. data-model.md was updated alongside this type so the
// spec and the implementation stay aligned.
type CommandLineState struct {
	// Active is true while a `:` / `/` / `?` prompt is open.
	Active bool
	// Prefix carries the prompt sigil — one of `:`, `/`, or `?`. The
	// empty rune is reserved for "no prompt active".
	Prefix rune
	// Buffer is the user's in-progress input (no leading prefix).
	Buffer string
	// History stores submitted prompts oldest-first, deduped per prefix.
	// `:` commands and `/?` searches share one ring per prefix; the UI
	// looks up via `historyFor(prefix)`.
	HistoryColon []string
	HistorySlash []string
	// HistoryCursor indexes back into the appropriate history slice
	// when the user is recalling a prior entry. -1 means "current
	// buffer (not from history)".
	HistoryCursor int
}

// reset clears the command-line state so the next prompt starts clean.
// Called on submit / cancel / esc.
func (c *CommandLineState) reset() {
	c.Active = false
	c.Prefix = 0
	c.Buffer = ""
	c.HistoryCursor = -1
}

// historyFor returns the history slice the cursor steps through given
// the active prefix. Search and command histories are kept distinct so
// `↑` from a `:` prompt doesn't surface a recent `/` search.
func (c *CommandLineState) historyFor(prefix rune) []string {
	switch prefix {
	case ':':
		return c.HistoryColon
	case '/', '?':
		return c.HistorySlash
	}
	return nil
}

// pushHistory records the submitted entry into the prefix's history,
// deduplicating consecutive equals so repeating `:42` doesn't swamp
// the ring.
func (c *CommandLineState) pushHistory(prefix rune, entry string) {
	if entry == "" {
		return
	}
	switch prefix {
	case ':':
		c.HistoryColon = appendDedup(c.HistoryColon, entry)
	case '/', '?':
		c.HistorySlash = appendDedup(c.HistorySlash, entry)
	}
}

// appendDedup appends `s` unless it already equals the last element.
func appendDedup(history []string, s string) []string {
	if n := len(history); n > 0 && history[n-1] == s {
		return history
	}
	return append(history, s)
}
