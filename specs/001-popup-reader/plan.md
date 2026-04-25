<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Implementation Plan: Popup Reader

**Branch**: `001-popup-reader` | **Date**: 2026-04-25 | **Spec**: [./spec.md](./spec.md)
**Input**: [specs/001-popup-reader/spec.md](./spec.md)

## Summary

`spy` is a focused popup viewer for text, code, PDFs, and images, optimised
for review work inside multiplexed terminals (tmux and friends). The MVP
covers six prioritised user stories: quick text review with syntax
highlighting (P1), code navigation/search (P1), dark/light theme adaptation
(P1), inline PDF/image rendering on capable terminals with graceful fallback
(P2), pipe-input support à la `bat` (P2), and footer metadata (P3).

The implementation extends the existing Bubble Tea + Lip Gloss + Chroma +
pdfcpu skeleton with concurrent progressive loading, capability-driven
graphics protocols (Kitty / iTerm2 / sixel), OSC-11 theme detection, smart
search, and a layered config loader (TOML + env + flags). Errors are emitted
to stderr without launching the viewer. Stdin and very large files share a
single chunked-loader pipeline with optional windowing for files above
256 MiB to honour the 500 MB working-set ceiling.

Detailed unknowns and decisions live in [./research.md](./research.md);
runtime types in [./data-model.md](./data-model.md); user-facing and
internal API contracts in [./contracts/](./contracts/); validation walk-through
in [./quickstart.md](./quickstart.md).

## Technical Context

**Language/Version**: Go 1.26.2 (per `go.mod`).
**Primary Dependencies**:

- `github.com/charmbracelet/bubbletea` — TUI runtime.
- `github.com/charmbracelet/bubbles/viewport` — scrollable region (replacing the current hand-rolled viewport).
- `github.com/charmbracelet/lipgloss` — terminal styling.
- `github.com/charmbracelet/glamour` — markdown rendering.
- `github.com/alecthomas/chroma/v2` — syntax highlighting.
- `github.com/pdfcpu/pdfcpu` — PDF metadata + text extraction (already vendored).
- `github.com/gen2brain/go-fitz` — PDF rasterization (cgo; gated by `fitz` build tag).
- `github.com/mattn/go-sixel` — sixel encoding for terminals that support it.
- `github.com/muesli/termenv` — already transitive; used for OSC-11 background detection.
- `github.com/BurntSushi/toml` — config-file parser.
- `golang.org/x/term` — TTY detection and terminal-state save/restore.

**Storage**: None (read-only viewer). Optional config file at
`$XDG_CONFIG_HOME/spy/config.toml`; no other persistent state.

**Testing**: `go test ./... -race -cover`, table-driven tests, plus a
`tests/integration` harness using `golang.org/x/term` PTY helpers and a
golden-file output check for renderer output. Quickstart-style end-to-end
flows captured as scripted shell tests under `tests/e2e`.

**Target Platform**: Linux and macOS terminals (xterm-compatible). Windows
via `windows-1252` codepage detection out of scope for v1; binary build
should still compile under `GOOS=windows` but graphics protocols are
disabled.

**Project Type**: CLI / desktop-app. Single Go module; no front/back split.

**Performance Goals**:

- ≤ 100 ms time-to-first-frame for ≤ 100-line files (SC-001).
- 60 fps perceived smoothness while scrolling 10k-line files (SC-002).
- ≤ 500 ms search across files up to 1 MiB (SC-003).
- Resident memory ≤ 500 MB on 1 GB inputs (SC-005).

**Constraints**:

- Pure-Go default build; cgo-using features (PDF rasterization) gated behind
  `fitz` build tag so static-binary distros can opt out.
- Reuse-compliant licensing already in place; new files MUST carry SPDX
  headers (MIT OR Apache-2.0).
- No telemetry or network calls.

**Scale/Scope**: Single-user, single-process; one viewer session per
invocation. Up to ~1 GB inputs via windowed mode. Config file ≤ a few KiB.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Constitution version evaluated**: v1.0.0 (ratified 2026-04-25). Principles I–VI.

| Principle | Plan compliance |
|-----------|-----------------|
| I. Spec-Driven Development (NON-NEGOTIABLE) | This plan plus `spec.md`, `research.md`, `data-model.md`, `contracts/`, and `tasks.md` together satisfy the workflow. |
| II. Test-First Discipline (NON-NEGOTIABLE) | `tasks.md` preamble enforces failing-test-first per implementation task; `-race` and ≥ 80 % per-package coverage are PR gates (T003, T111). PTY harness for capability paths (T004). |
| III. Unix Philosophy & Composability | Errors → stderr (FR-013, `contracts/cli.md`); stable documented exit codes; stdin is first-class (FR-002, US5); no temp files for piped input (research R5); composes via flags/pipes only. |
| IV. Pure-Go by Default, cgo Opt-In | Default build is pure-Go (T001); cgo features (`go-fitz` PDF rasterization) gated behind the `fitz` build tag with a pure-Go text-extraction fallback (research R3, T079). Cross-compile targets supported without a cgo toolchain. |
| V. Capability-Aware Graceful Degradation | `internal/term.Detect` covers TTY/color/graphics/luminance probes (T011–T014, T064–T065); image/PDF metadata fallback (T080–T081); 80×24 graceful degradation (Q4, T098); panic-safe terminal restore + graphics cleanup (research R10, T032/T083). |
| VI. REUSE-Compliant Licensing | Every new file carries the SPDX dual-license header (T002, T107); `reuse lint` is a CI gate (T003, T111). No GPL/AGPL transitive deps in production paths. |

**Constitution gate result**: **PASS** — all six principles satisfied by the artifacts in this directory.

**Re-evaluation after Phase 1 design**: **PASS (no new violations)**. The contracts in `contracts/internal-apis.md` keep cross-package dependencies acyclic (`term` → leaf; `source` → `term`; `loader` → `source`; `highlight` → `source`; `graphics` → `term`; `render` → all of the above; `ui` → `render` + `keys`).

Applying this update closes T103 in `tasks.md` (Polish phase).

## Project Structure

### Documentation (this feature)

```text
specs/001-popup-reader/
├── plan.md              # This file (/speckit-plan output)
├── spec.md              # /speckit-specify + /speckit-clarify output
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/
│   ├── cli.md           # CLI surface
│   ├── keys.md          # Key bindings
│   ├── config.md        # Config file schema
│   └── internal-apis.md # Internal package signatures
├── checklists/
│   └── requirements.md  # /speckit-specify output
└── tasks.md             # /speckit-tasks output (NOT created here)
```

### Source Code (repository root)

The existing layout (`cmd/spy` + `internal/{config,reader,renderer,ui}`) is
extended into the structure below. Renames are deliberate to align with the
package boundaries pinned in [contracts/internal-apis.md](./contracts/internal-apis.md).
The current `internal/reader` becomes `internal/source`, and
`internal/renderer` becomes `internal/render` to free those names for the
new chunked-loader and capability-aware renderer respectively. /speckit-tasks
will produce migration tasks; this plan documents the destination shape.

```text
spy/
├── cmd/
│   └── spy/
│       ├── main.go              # Flag parsing, capability probe, wiring
│       └── flags.go             # Flag/env definitions; pure (testable)
├── internal/
│   ├── config/                  # Existing; expanded with TOML loader, XDG lookup
│   │   ├── config.go
│   │   ├── load.go              # TOML + env + flag merge
│   │   └── load_test.go
│   ├── term/                    # NEW: TTY + capability detection + restore
│   │   ├── capabilities.go
│   │   ├── theme.go             # OSC 11 luminance probe
│   │   └── *_test.go
│   ├── source/                  # RENAMED from reader; concept of Source / Kind / Metadata
│   │   ├── source.go
│   │   ├── file.go
│   │   ├── stdin.go
│   │   ├── detect.go
│   │   └── *_test.go
│   ├── loader/                  # NEW: chunked + windowed reader, goroutine pipeline
│   │   ├── stream.go
│   │   ├── window.go
│   │   └── *_test.go
│   ├── highlight/               # NEW: Chroma streaming highlighter
│   │   ├── highlighter.go
│   │   └── *_test.go
│   ├── graphics/                # NEW: image / PDF graphics protocols
│   │   ├── kitty.go
│   │   ├── iterm2.go
│   │   ├── sixel.go
│   │   ├── pdf_fitz.go          # build tag: fitz
│   │   ├── pdf_nofitz.go        # build tag: !fitz
│   │   └── *_test.go
│   ├── render/                  # RENAMED from renderer; per-Kind renderers
│   │   ├── renderer.go
│   │   ├── code.go
│   │   ├── markdown.go
│   │   ├── image.go
│   │   ├── pdf.go
│   │   ├── theme.go
│   │   ├── statusbar.go
│   │   └── *_test.go
│   ├── search/                  # NEW
│   │   ├── search.go
│   │   ├── matcher.go
│   │   └── *_test.go
│   ├── keys/                    # NEW
│   │   ├── keymap.go
│   │   ├── default.go
│   │   ├── vim.go
│   │   └── *_test.go
│   └── ui/                      # Existing; rewired to use new packages
│       ├── model.go
│       ├── update.go
│       ├── view.go
│       ├── help.go
│       └── *_test.go
├── tests/
│   ├── integration/             # PTY-driven tests covering capability paths
│   └── e2e/                     # Scripted shell flows mirroring quickstart.md
├── examples/
│   └── config.toml              # Reference config matching contracts/config.md
├── DEVELOPMENT.md
├── README.md
├── Makefile
├── REUSE.toml
├── _typos.toml
├── go.mod
└── go.sum
```

**Structure Decision**: Keep the existing single-module CLI layout. The new
features are accommodated by splitting `internal/reader` and
`internal/renderer` into purpose-specific packages (`term`, `source`,
`loader`, `highlight`, `graphics`, `render`, `search`, `keys`), preserving a
DAG of dependencies and matching the contract in
[contracts/internal-apis.md](./contracts/internal-apis.md). All production
code stays under `internal/` so we retain freedom to reshape package
boundaries before any external API stabilises.

## Phase 0: Outline & Research

Output: [./research.md](./research.md).

Resolved 13 unknowns covering capability detection, image protocols, PDF
preview strategy, progressive loading, stdin handling, theme detection,
streaming highlighting, search/jump UX, viewport choice, signal handling,
configuration loading, key-binding model, and the constitution-template gap.

All `NEEDS CLARIFICATION` items from the spec's clarification phase were
already resolved into FR-005, FR-012, FR-013, and the Q4 minimum-size
assumption; Phase 0 surfaced no further unresolved ambiguities.

## Phase 1: Design & Contracts

Outputs:

- [./data-model.md](./data-model.md) — runtime entities and validation rules.
- [./contracts/cli.md](./contracts/cli.md) — CLI surface (positional, flags,
  env, exit codes, error formatting).
- [./contracts/keys.md](./contracts/keys.md) — key bindings (default + vim).
- [./contracts/config.md](./contracts/config.md) — TOML config schema.
- [./contracts/internal-apis.md](./contracts/internal-apis.md) — internal
  package signatures for parallel /speckit-tasks work.
- [./quickstart.md](./quickstart.md) — 15-step end-to-end validation.

Agent context update: `CLAUDE.md`'s SPECKIT marker now references
`specs/001-popup-reader/plan.md` so future agent sessions on this branch
load the correct plan first.

## Complexity Tracking

The constitution check was a non-blocking PASS (template not yet ratified),
so no complexity-tracking entries are required. The only deliberately added
complexity is the `fitz` build-tag gate around PDF rasterization, which is
justified by the AGPL-vs-MIT licensing constraint on alternatives and the
need to preserve cgo-free static builds for distributors. This is recorded
in research §R3 rather than as a constitution violation.
