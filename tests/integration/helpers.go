// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package integration

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/knitli/spy/internal/loader"
)

// DrainStreamErrs reads the loader stream's error channel to EOF and
// fails the test (via t.Fatalf) on any error that is NOT a documented
// warning sentinel. Warning-class errors (ErrLineTruncated,
// ErrStdinNonSeekable) accumulate in the returned slice so the caller
// can inspect them — but they don't fail the test.
//
// Tests that consume stream.Errs from internal/loader should funnel
// through here rather than spinning a `for range stream.Errs {}` that
// silently discards fatal load errors. A perf gate that records
// timings against a half-loaded stream is a false-green; a regression
// test that asserts on rendered content from a partial buffer is a
// false-positive. Both are caught by funneling here.
func DrainStreamErrs(t *testing.T, errs <-chan error) []error {
	t.Helper()
	var warnings []error
	for err := range errs {
		if err == nil {
			continue
		}
		if isLoaderWarning(err) {
			warnings = append(warnings, err)
			continue
		}
		t.Fatalf("loader stream emitted fatal error: %v", err)
	}
	return warnings
}

// isLoaderWarning reports whether `err` is one of the documented
// non-fatal warning sentinels emitted on [loader.Stream.Errs].
func isLoaderWarning(err error) bool {
	return errors.Is(err, loader.ErrLineTruncated) ||
		errors.Is(err, loader.ErrStdinNonSeekable)
}

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

// stripANSI removes CSI / OSC / SGR escape sequences from `s` so
// substring assertions over rendered frames can match content that
// Chroma's tokeniser splits across SGR boundaries (e.g.
// "fmt"+RESET+SET+"Println").
//
// Lives in helpers.go (non-test, no build tag) because resize_test.go
// — the original home — is gated by `//go:build !race` and the
// function would otherwise disappear under `-race`.
func stripANSI(s string) string {
	var b bytes.Buffer
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			i++
			continue
		}
		switch s[i+1] {
		case '[':
			// CSI: ESC [ params... <final byte 0x40-0x7e>
			j := i + 2
			for j < len(s) {
				c := s[j]
				if c >= 0x40 && c <= 0x7e {
					j++
					break
				}
				j++
			}
			i = j
		case ']':
			// OSC: ESC ] params... ST (BEL or ESC \\)
			j := i + 2
			for j < len(s) {
				if s[j] == 0x07 {
					j++
					break
				}
				if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			i = j
		default:
			// Two-byte sequence; consume both.
			i += 2
		}
	}
	return b.String()
}
