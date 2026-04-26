// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzConfigLoad is the T109b.b TOML-parser robustness gate: feed
// arbitrary bytes to [Load] and assert it never panics or hangs.
// Failures (malformed TOML, deeply nested tables, huge integers,
// invalid UTF-8 in strings) are required to surface as warnings —
// per the T024/T025 contract Load returns a non-nil *Config alongside
// any warnings rather than crashing.
//
// Seeded from examples/config.toml plus a curated set of malformed
// shapes so the corpus covers the well-formed → adversarial spectrum.
//
// Run with `go test -fuzz=FuzzConfigLoad ./internal/config/...` for
// ≥ 60 s during the v0.1.0 security review.
//
// CAMPAIGN HISTORY (acceptance-review finding M12):
//   - 2026-04-26 — 60 s on Linux/amd64 (Go 1.23): 71 236 executions,
//     79 new interesting inputs, **zero failures**. Go's fuzz engine
//     only commits seeds to testdata/fuzz/<TargetName> on a failure;
//     interesting inputs that don't trigger a regression stay in the
//     local fuzz cache. The empty testdata/fuzz/FuzzConfigLoad/
//     directory is therefore the expected post-campaign state, not a
//     missing-corpus bug — the seeds in the f.Add(...) calls above
//     are the regression corpus that runs on every `go test ./...`.
//   - Re-run the campaign during each release security review and
//     append the result here.
func FuzzConfigLoad(f *testing.F) {
	// Well-formed seed.
	f.Add([]byte(`
theme = "dark"
vim_mode = false
regex_default = false
case_mode = "smart"
word_wrap = true
line_numbers = true
tab_width = 4
max_resident_bytes = 268435456
window_size = 8192
highlight_cap_bytes = 5242880
`))

	// Adversarial seeds — intentionally malformed.
	f.Add([]byte(""))                              // empty file
	f.Add([]byte("\x00\x00\x00"))                  // null bytes
	f.Add([]byte("[[[[[[[[[[[[[[[[[[[[[[[[[[[[[")) // unbalanced brackets
	f.Add([]byte("a = 99999999999999999999999"))   // huge integer
	f.Add([]byte("a = \"\xff\xfe invalid utf8\""))
	f.Add([]byte("[a]\n[a]\n[a]\n[a.b]\n[a.b.c]\n[a.b.c.d]\n[a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p]"))
	f.Add([]byte("theme = \"\\u0000\""))
	f.Add([]byte("theme = \"" + string(make([]byte, 1024)) + "\""))

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "fuzz.toml")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Skipf("write tmp: %v", err)
		}
		cfg, _ := Load(LoadOptions{ConfigPath: path, ExplicitConfigPath: true})
		if cfg == nil {
			t.Fatalf("Load returned nil config; corpus byte len=%d", len(data))
		}
	})
}
