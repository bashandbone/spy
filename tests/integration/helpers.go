// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
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
	if len(got) == len(want) {
		eq := true
		for i := range got {
			if got[i] != want[i] {
				eq = false
				break
			}
		}
		if eq {
			return ""
		}
	}
	return "frames differ (lengths got=" +
		itoa(len(got)) + " want=" + itoa(len(want)) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
