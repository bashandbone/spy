// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

package perf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// TestHighlightCorpus_LinguistTop50 enforces SC-006: against a corpus
// of representative source files (one per GitHub Linguist top language,
// each ≤ 4 KiB), Chroma must (a) pick a non-`fallback` lexer and (b)
// emit ≤ 1 % of bytes as `chroma.Error` tokens. Pass threshold: ≥ 47/50
// files (94 %).
//
// The fixtures live under tests/fixtures/_highlight-corpus/ — the
// `_` prefix tells Go to skip the directory when scanning for build
// inputs (otherwise the C/C++/asm sample files would be picked up as
// cgo source). Each file is named <lang>.<ext> where the extension
// drives Chroma's extension-based lexer selection. Adding a fixture
// with a fresh extension is enough to extend the corpus; the test
// discovers files by scanning the directory.
//
// When the corpus has fewer than 50 fixtures (the minimum the spec
// asks for) the test reports the gap as a soft skip — adding more
// fixtures is tracked as part of T106a's incremental rollout. The
// hard fail-the-PR gate is only the per-fixture pass-rate.
func TestHighlightCorpus_LinguistTop50(t *testing.T) {
	const passThreshold = 47
	const totalRequired = 50
	const errorBytePct = 0.01

	dir := filepath.Join(corpusRoot(t), "tests/fixtures/_highlight-corpus")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read corpus dir %s: %v", dir, err)
	}

	type result struct {
		name   string
		lexer  string
		errPct float64
		ok     bool
		reason string
	}
	var results []result
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		buf, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read fixture %s: %v", full, err)
		}
		if len(buf) > 4*1024 {
			t.Fatalf("fixture %s exceeds 4 KiB cap (%d bytes); spec requires ≤ 4 KiB", e.Name(), len(buf))
		}
		lex := lexers.Match(e.Name())
		if lex == nil {
			results = append(results, result{name: e.Name(), reason: "no extension match"})
			continue
		}
		cfg := lex.Config()
		if isPlaintextLexer(cfg.Name) {
			results = append(results, result{name: e.Name(), lexer: cfg.Name, reason: "fallback/plaintext lexer"})
			continue
		}
		iter, err := lex.Tokenise(nil, string(buf))
		if err != nil {
			results = append(results, result{name: e.Name(), lexer: cfg.Name, reason: "tokenise error: " + err.Error()})
			continue
		}
		var errBytes int
		var totalBytes int
		for {
			tok := iter()
			if tok == chroma.EOF {
				break
			}
			n := len(tok.Value)
			totalBytes += n
			if tok.Type == chroma.Error {
				errBytes += n
			}
		}
		if totalBytes == 0 {
			results = append(results, result{name: e.Name(), lexer: cfg.Name, reason: "empty tokenisation"})
			continue
		}
		pct := float64(errBytes) / float64(totalBytes)
		ok := pct <= errorBytePct
		results = append(results, result{
			name:   e.Name(),
			lexer:  cfg.Name,
			errPct: pct,
			ok:     ok,
			reason: "",
		})
	}

	passed := 0
	for _, r := range results {
		if r.ok {
			passed++
			t.Logf("PASS %s → %s (err=%.3f%%)", r.name, r.lexer, r.errPct*100)
		} else {
			t.Logf("FAIL %s (lexer=%s err=%.3f%% reason=%s)",
				r.name, r.lexer, r.errPct*100, r.reason)
		}
	}
	t.Logf("highlight corpus: %d/%d files passed (threshold %d, target %d)",
		passed, len(results), passThreshold, totalRequired)

	if len(results) < totalRequired {
		// Pass-rate is the hard gate; the corpus-completeness gap is
		// reported as a soft warning so the test starts catching
		// regressions immediately rather than waiting for the full 50
		// fixtures to land.
		t.Logf("WARNING: corpus has %d fixtures, spec target is %d — see T106a", len(results), totalRequired)
	}
	if len(results) == 0 {
		t.Fatal("highlight corpus is empty; no fixtures under tests/fixtures/_highlight-corpus/")
	}

	// Hard gate: pass-rate. We scale the threshold to whatever's
	// actually present so partial corpora still gate against
	// regressions. With the full 50 fixtures the threshold is 47.
	wantPass := passThreshold
	if len(results) < totalRequired {
		// 94 % of whatever's in the corpus.
		wantPass = (len(results) * passThreshold) / totalRequired
		if wantPass < 1 {
			wantPass = len(results)
		}
	}
	if passed < wantPass {
		t.Fatalf("SC-006: %d/%d files passed (need ≥ %d for the current corpus)",
			passed, len(results), wantPass)
	}
}

// corpusRoot returns the module root by walking up from the package
// directory until go.mod is found. Used so the test runs equivalently
// from any directory `go test` was invoked from.
func corpusRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("module root not found above %s", wd)
		}
		dir = parent
	}
}

// isPlaintextLexer mirrors the function of the same name in
// internal/source/detect.go: a Chroma lexer named "plaintext", "text",
// or "fallback" is treated as not having matched a real language.
func isPlaintextLexer(name string) bool {
	n := strings.ToLower(name)
	return n == "" || n == "plaintext" || n == "text" || n == "fallback"
}
