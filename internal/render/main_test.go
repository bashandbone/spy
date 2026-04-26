// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain forces the lipgloss default renderer into a deterministic
// 256-color profile for the duration of the package's test run. The
// match-highlight + theme + status-bar tests assert that lipgloss-styled
// output carries ANSI escapes; without this, the assertions are at the
// mercy of CI runner TERM (e.g., GitHub Actions runs with TERM=dumb,
// which downgrades the auto-detected profile to ASCII and strips every
// escape — see PR #14 review).
//
// Tests that need a different profile can call
// `lipgloss.SetColorProfile(...)` on the side; the package-level reset
// here happens once before any subtest fires.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	os.Exit(m.Run())
}
