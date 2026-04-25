// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ReadGolden reads the golden file at the given relative path under
// tests/integration/golden/ and returns its bytes. Fails the test if
// missing.
func ReadGolden(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("golden", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read golden %s: %v", p, err)
	}
	return b
}

// WriteGolden writes b to tests/integration/golden/<name>. Used when
// regenerating goldens with `go test -update` (the flag is wired in Phase 2).
func WriteGolden(t *testing.T, name string, b []byte) {
	t.Helper()
	p := filepath.Join("golden", name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir golden: %v", err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatalf("write golden %s: %v", p, err)
	}
}

// DiffFrames returns "" when got and want are byte-equal; otherwise a short
// human-readable summary. A real diff (line-by-line, with caret markers
// for ANSI escape boundaries) is wired in Phase 2 alongside the first
// golden test (T040).
func DiffFrames(got, want []byte) string {
	if bytes.Equal(got, want) {
		return ""
	}
	return fmt.Sprintf("frames differ (lengths got=%d want=%d)", len(got), len(want))
}
