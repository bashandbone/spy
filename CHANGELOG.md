<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Changelog

All notable changes to `spy` will be documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Performance suites under `tests/perf/` covering SC-001..SC-008
  (first-frame, scroll, search, theme-swap, large-file, highlight
  corpus, dismiss latency, resize handling).
- PTY-driven integration harness in `tests/integration/pty.go` and the
  full set of integration tests previously documented as
  `t.Skip` placeholders.
- Nightly performance workflow (`.github/workflows/nightly-perf.yml`)
  that runs the `-tags perf` heavyweight tier and files an issue on
  regression.
- Security-review checklist (`specs/001-popup-reader/checklists/`) and
  the supporting tests: terminal-escape neutralization in
  `internal/render/code.go` and a TOML fuzz target in
  `internal/config`.

## [0.1.0] — initial release

The first tagged release of `spy` — the spec-driven popup reader.

### Added

- **US1 — Quick text review with syntax highlighting.**
  Open a file or pipe, get a Chroma-highlighted alt-screen viewport
  with line numbers, soft-wrap, and a footer.
- **US2 — Code navigation: search + jump-to-line.**
  Forward / backward search with smart-case (toggleable), wrap-around,
  and `:N` / `:0` / `:$` line jumps via the command line.
- **US3 — Dark/light theme detection + override.**
  Auto theme via terminal background luminance (OSC 11), with `--theme`
  / `SPY_THEME` overrides covering `auto | dark | light | <chroma-style>`
  and a runtime `:set theme` command.
- **US4 — PDF and image support.**
  Kitty, iTerm2, and Sixel graphics protocols (auto-detected, override
  via `--graphics`); PDF page navigation under `[` / `]`. PDF
  rasterization requires the `-tags fitz` cgo build; pure-Go builds
  fall back to text extraction.
- **US5 — Pipe input support.**
  `git diff | spy`, `cat hello.go | spy -l go`, explicit `-` positional,
  and the degenerate-cat contract (`spy | cat` exits 0 with verbatim
  content).
- **US6 — File metadata footer.**
  Footer shows `<basename> | <N> lines | Line <cur>` (responsive
  collapse to `<basename> · L<cur>` below 80 columns), with `<stdin>`
  as the literal name for piped sources.
- Streaming and windowed loaders (256 MiB resident threshold; 1 GiB
  ceiling at 500 MiB resident, per SC-005).
- Per-package coverage gate ≥ 80% on the merged default + `fitz`
  coverage profile.
- REUSE 3.3 compliance with SPDX headers on every source file.

### Documented

- Behavioral contracts: CLI surface, keybindings, config schema,
  internal package APIs (`specs/001-popup-reader/contracts/`).
- Constitution v1.0.0 with 9 non-negotiable principles, anchored to the
  spec via the Constitution Check section in `plan.md`.

[Unreleased]: https://github.com/knitli/spy/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/knitli/spy/releases/tag/v0.1.0
