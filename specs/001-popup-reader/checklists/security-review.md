<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Security Review — v0.1.0

**Spec**: 001-popup-reader
**Reviewer**: Adam Poulemanos (implementer self-review)
**Date**: 2026-04-25
**Tracks**: T109b in
[tasks.md](../tasks.md)

This checklist covers the six categories called out in T109b. Findings
are recorded as `PASS` (no action required), `FIX` (in scope for
v0.1.0), or `FOLLOWUP` (tracked for a later release).

---

## (a) Path handling

> Source: [internal/source/source.go](../../../internal/source/source.go),
> [internal/source/file.go](../../../internal/source/file.go)

- File arguments resolve through `filepath.Clean` and `filepath.EvalSymlinks`
  before opening (`source.go:194`). Symlink targets are accepted per
  `contracts/cli.md` ("Symlinks are followed").
- Pseudo-fs root denylist (`/proc/self/mem`, `/dev/zero`, `/dev/random`,
  `/sys`): **FOLLOWUP**. The current implementation opens whatever
  resolved path the user asked for. Concretely, `spy /proc/self/mem`
  would attempt to read process memory and fail at the `read` syscall,
  but it would not be rejected up front.
- File-mode check against directories / FIFOs / character devices:
  PASS — `newFileSource` calls `os.Stat` and rejects non-regular files
  with `ErrUnsupported`.

**Status**: PASS for the v0.1.0 surface; pseudo-fs denylist deferred
behind a `FOLLOWUP` label (low real-world risk against an interactive
viewer; users who can pass arguments to `spy` can already
`cat /proc/self/mem` directly).

## (b) TOML parser robustness

> Source: [internal/config/load.go](../../../internal/config/load.go),
> [internal/config/fuzz_test.go](../../../internal/config/fuzz_test.go)

- `BurntSushi/toml` v1.6.0 is the parser. It surfaces parse failures
  as `ErrConfigParse` warnings; `Load` always returns a non-nil
  `*Config` so a malformed file degrades to defaults rather than
  crashing.
- `FuzzConfigLoad` was added in this phase: seeded with a well-formed
  corpus plus 7 adversarial inputs (empty bytes, null bytes,
  unbalanced brackets, huge integers, invalid UTF-8, deep nesting,
  embedded null character, 1 KiB-string). Run with
  `go test -fuzz=FuzzConfigLoad ./internal/config/...` for ≥ 60 s
  before tagging v0.1.0.

**Status**: PASS. The fuzz seeds run clean; a longer (60 s+) fuzz
campaign on a developer workstation is sufficient pre-tag work.

## (c) Terminal escape injection from file content

> Source: [internal/render/sanitize.go](../../../internal/render/sanitize.go),
> [tests/integration/escape_injection_test.go](../../../tests/integration/escape_injection_test.go)

Files containing `\x1b]2;...\x07` (OSC 2 "set window title") would,
without sanitisation, drive the user's terminal title. The renderer
now neutralises every `\x1b` (0x1b) and `\x9b` (8-bit CSI) byte in raw
content before emission via `neutralizeEscapes`:

- `code.go` `styleLine` — every fallback path (mono mode, no
  highlighter, empty tokens, formatter failure).
- `code.go` `Render` — match-overlay mono path and the wrap path.
- `match.go` `applyMatchHighlights` — sanitises before slicing so
  match offsets remain byte-aligned.
- `text.go` — both the no-wrap and wrap fallback paths.

The substitution is byte-for-byte (`\x1b` → `?`) so search-match
offsets and visual width math stay valid.

`tests/integration/escape_injection_test.go::TestEscapeInjection_OSCSequenceNeutralised`
asserts that:
1. The rendered frame contains no live OSC payload.
2. The surrounding (benign) content survives.
3. Every remaining `\x1b` is followed by `[` (CSI introducer) — only
   Chroma's SGR colour escapes are allowed in a v0.1.0 frame.

**Status**: PASS. Trade-off: files with intentional ANSI colour
escapes (e.g., `git diff --color=always > /tmp/diff.txt`) lose their
colour. A future `--allow-ansi-passthrough` flag with an
SGR-but-not-OSC discriminator can opt back in.

## (d) OSC 11 background-color reply parsing

> Source: [internal/term/theme.go](../../../internal/term/theme.go)

- `parseOSC11Reply` validates against the strict regex
  ``^\x1b\]11;rgb:([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})(?:\x07|\x1b\\)$``
  and returns `math.NaN()` on any mismatch.
- `theme_test.go` covers the adversarial-reply matrix: empty,
  embedded CSI, malformed RGB triplets, missing terminator,
  unsupported response IDs.

**Status**: PASS.

## (e) Graphics decoder safety

> Source: [internal/graphics/graphics.go](../../../internal/graphics/graphics.go),
> [internal/graphics/graphics_test.go](../../../internal/graphics/graphics_test.go)

- Image decoding flows through `image.Decode` (stdlib) and
  `golang.org/x/image` decoders.
- PDF rasterization (only with `-tags fitz`) flows through
  `gen2brain/go-fitz`, which wraps `mupdf`. `mupdf` panics on certain
  malformed inputs; the renderer's deferred recovery in
  `internal/graphics/graphics.go:60` catches them and surfaces the
  failure as `ErrUnsupported`.
- `graphics_test.go::TestGraphics_RecoversFromDecoderPanic` asserts
  the recovery path keeps the program alive.

**Status**: PASS.

## (f) No accidental network calls

> Verified by the CI gate
> [.github/workflows/ci.yml](../../../.github/workflows/ci.yml) "no-network" job.

The grep gate fails the build on any product-code reference to
`http.Get/Post/Head/Do/Client`, `net.Dial*`, or `.Get(`. Verified
locally against `internal/` and `cmd/` (excluding `_test.go`):

```text
$ grep -rE '(\bhttp\.(Get|Post|Head|Do|Client)|\bnet\.Dial|\.Get\()' internal/ cmd/ \
    --include='*.go' | grep -v '_test\.go:'
(no output)
```

**Status**: PASS.

---

## Summary

| Category | Status | Notes |
|----------|--------|-------|
| (a) Path handling | PASS | Pseudo-fs denylist deferred (FOLLOWUP). |
| (b) TOML parser robustness | PASS | Fuzz seeds in place; 60s+ fuzz run before tag. |
| (c) Escape injection | PASS | `neutralizeEscapes` covers all emit boundaries. |
| (d) OSC 11 parsing | PASS | Strict regex + adversarial-reply tests. |
| (e) Graphics decoder safety | PASS | Deferred recovery + ErrUnsupported. |
| (f) No accidental network | PASS | CI grep gate. |

**Tag-blocking issues**: none. A `FOLLOWUP` for the pseudo-fs denylist
is filed for a follow-up release; everything else is in place for the
v0.1.0 tag.
