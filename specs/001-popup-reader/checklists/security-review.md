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
> [internal/render/sanitize_boundary_test.go](../../../internal/render/sanitize_boundary_test.go),
> [tests/integration/escape_injection_test.go](../../../tests/integration/escape_injection_test.go)

Files containing `\x1b]2;...\x07` (OSC 2 "set window title") would,
without sanitisation, drive the user's terminal title. Every emit
boundary now neutralises ESC (0x1b) and CSI (0x9b) bytes via the
exported `render.Neutralize`:

| Layer | Site | Notes |
|---|---|---|
| Code (US1) | `code.go` styleLine fallbacks + Render match overlay | T109b.c original scope |
| Match overlay | `match.go` applyMatchHighlights | sanitises before slicing |
| Text passthrough | `text.go` no-wrap + wrap paths | |
| Markdown | `markdown.go` assembleRaw | Glamour does NOT strip ESC from non-code-block content |
| PDF text | `pdf.go` formatTextPage (text + DisplayName) | the default path on no-fitz builds |
| PDF metadata | `pdf.go` metadataBlock (DisplayName + note) | graphics-unavailable fallback |
| Image metadata | `image.go` metadataBlock (DisplayName + path + note) | terminal-lacks-graphics fallback |
| Status bar | `statusbar.go` StatusBarRender (DisplayName + Advisory) | sanitised at entry-point so wide / collapsed both protected |
| stderr | `cmd/spy/main.go` ParseFlags / config warnings / keymap warnings / exitForSourceError / runDegenerate / tea.Program error | every stderr write now sanitises the user-controlled error message |

The substitution is byte-for-byte (`\x1b` / `\x9b` → `?`) so
search-match offsets and visual width math stay valid.

`internal/render/sanitize_boundary_test.go` adds 8 new boundary
tests; `cmd/spy/main_test.go::TestC4_StderrSanitizesHostileFilename`
adds the stderr-side gate. `tests/integration/escape_injection_test.go`
already covers the integration-level rendered-frame contract.

**Status**: PASS. *Updated 2026-04-26 — original draft only covered
code/match/text. Acceptance review C4 surfaced the markdown / PDF /
image / statusbar / stderr bypasses, all closed in the same patch.*

Trade-off: files with intentional ANSI colour escapes (e.g.,
`git diff --color=always > /tmp/diff.txt`) lose their colour. A
future `--allow-ansi-passthrough` flag with an SGR-but-not-OSC
discriminator can opt back in.

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

> Source: [internal/render/image.go](../../../internal/render/image.go),
> [internal/render/pdf_fitz.go](../../../internal/render/pdf_fitz.go),
> [internal/render/decoder_panic_test.go](../../../internal/render/decoder_panic_test.go)

- Image decoding flows through `image.Decode` (stdlib) and
  `golang.org/x/image` decoders.
- PDF rasterization (only with `-tags fitz`) flows through
  `gen2brain/go-fitz`, which wraps `mupdf`. `mupdf` invokes
  `longjmp` on certain malformed inputs which Go's runtime
  surfaces as a panic.
- Both decoder paths are guarded by `defer recover()`:
  - `imageRenderer.decode` (`internal/render/image.go:111`) wraps
    `image.Decode`; surfaces as `ErrUnsupportedDecoder`.
  - `imageRenderer.dimensions` (`internal/render/image.go:166`)
    wraps `image.DecodeConfig`; falls back to "" dimensions in the
    metadata block.
  - `rasterizePDFPage` (`internal/render/pdf_fitz.go:26`) wraps
    `fitz.NewFromMemory` + `doc.Image`; surfaces as
    `ErrUnsupportedDecoder`.
- `internal/render/decoder_panic_test.go` exercises the recover
  path with a synthetic `panicSource` whose Read panics; both the
  decode-only and full-Render call sites stay alive and surface
  the documented error.

**Status**: PASS. *Updated 2026-04-26 to reflect the actual recover
locations — earlier draft cited a `defer recover` in
`internal/graphics/graphics.go` that did not exist (acceptance
review C5).*

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
