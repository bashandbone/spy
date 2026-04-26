<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Development guide

This document describes how to build, test, and contribute to `spy`.
The product surface, keybindings, configuration schema, and behavioral
contracts are specified under
[specs/001-popup-reader/](specs/001-popup-reader/).

## Prerequisites

- Go 1.26.2 or later (matches `go.mod`)
- `make`
- For coverage merging: `gocovmerge` —
  `go install github.com/wadey/gocovmerge@latest`
- Optional: `goimports` for the lint target —
  `go install golang.org/x/tools/cmd/goimports@latest`
- Optional: `reuse` (Python 3) for SPDX/license checks —
  `pipx install reuse` or `uv tool install reuse`
- For the `-tags fitz` build only: a C toolchain plus `mupdf` headers
  (`apt install libmupdf-dev` on Debian/Ubuntu;
  `brew install mupdf-tools` on macOS). The default build has no cgo
  dependency.

## Make targets

The `Makefile` is the single source of truth for the build/test surface.

| Target | What it does |
|--------|--------------|
| `make build` | Default pure-Go build → `bin/spy`. |
| `make build-fitz` | `-tags fitz` cgo build → `bin/spy-fitz` (PDF rasterization). |
| `make test` | `go test ./...` (default tags). |
| `make test-race` | `go test ./... -race`. |
| `make cover-default` | Per-package coverage on default build → `cov-default.out`. |
| `make cover-fitz` | Per-package coverage with `-tags fitz` → `cov-fitz.out`. |
| `make cover` | Merge default + fitz profiles into `coverage.out` (requires `gocovmerge`). |
| `make lint` | `gofmt -l .` and `goimports -l .`; non-zero on diff. |
| `make vet` | `go vet ./...` and `go vet -tags fitz ./...`. |
| `make fmt` | `go fmt ./...` (rewrites in-place). |
| `make reuse` | `reuse lint` (SPDX/license compliance). |
| `make perf` | `go test -tags perf ./tests/perf/...` (nightly suite). |
| `make all` | `fmt vet lint test-race build` — the dev-loop default. |
| `make clean` | Remove `bin/`, `cov-*.out`, `coverage.out`. |

The `cover` target merges the default and `fitz` profiles so files gated
by build tags (notably `internal/graphics/pdf_*.go`) are visible to the
≥ 80%/package coverage gate; without the merge, `pdf_fitz.go` is invisible
to the threshold check on a default-tags run.

## Build tags

| Tag | What it gates |
|-----|---------------|
| *(none)* | Pure-Go build. PDF rendering returns `ErrUnsupported`; rasterization is unavailable but the rest of the viewer (text, code, markdown, images) works fully. |
| `fitz` | Enables `internal/graphics/pdf_fitz.go` via [`gen2brain/go-fitz`](https://github.com/gen2brain/go-fitz) (cgo, links against `mupdf`). |
| `perf` | Gates `tests/perf/large_file_test.go` nightly tier and `dismiss_bench_test.go` heavyweight cases — see "Performance suites" below. |

`go vet -tags fitz ./...` is part of `make vet` so cgo-only files don't
silently bit-rot.

## Test layers

```
tests/
├── e2e/             # bash-driven, no TTY required, run by tests/e2e/run.sh
├── integration/     # PTY-driven, exercises alt-screen + signal paths
└── perf/            # benchmarks for SC-001..SC-008 (Phase 9)
```

### Unit tests

Co-located `*_test.go` files in every package. Run via `go test ./...`
or `make test`. Race detection is on by default in CI (`make test-race`).

### End-to-end (`tests/e2e/`)

Shell scripts that exercise non-TTY pipelines: stdin handling, exit
codes, the degenerate-cat contract, footer rendering when stdout is not
a TTY, etc. They run against the binary at `bin/spy` and don't need a
PTY. Driver: `tests/e2e/run.sh`.

### Integration (`tests/integration/`)

PTY-driven tests for behaviour that requires a real terminal: alt-screen
entry/exit, signal handling, graphics protocol emission, theme detection
probes. The harness in `tests/integration/pty.go` builds the binary
inside `t.TempDir()` and spawns it under a PTY (via
[`github.com/creack/pty`](https://github.com/creack/pty)), exposing
`Send`, `Read`, `Snapshot`, `Close`, and `ExitCode` helpers that test
files compose into golden-file or substring assertions.

The harness automatically skips tests on platforms without PTY support.
Tests that depend on a particular graphics protocol (e.g., Kitty)
configure the spawned process's environment via `NewPTYProgram`'s `env`
parameter.

### Performance suites (`tests/perf/`)

Benchmark tests guarding the success criteria from
[spec.md](specs/001-popup-reader/spec.md). The lightweight tier (no
build tag) runs on every PR; the heavyweight nightly tier is gated by
`-tags perf` and runs from
[`.github/workflows/nightly-perf.yml`](.github/workflows/nightly-perf.yml).

| Criterion | Test | PR gate | Nightly |
|-----------|------|---------|---------|
| SC-001 (≤ 100 ms first frame) | `firstframe_bench_test.go` | ✓ | — |
| SC-002 (60 fps scroll) | `scroll_bench_test.go` | ✓ | — |
| SC-003 (≤ 500 ms search on 1 MiB) | `search_bench_test.go` | ✓ | — |
| SC-004 (≤ 16 ms p95 theme swap) | `theme_swap_bench_test.go` | ✓ | — |
| SC-005 (200 MiB PR / 1 GiB nightly) | `large_file_test.go` | ✓ (200 MiB) | ✓ (1 GiB, `-tags perf`) |
| SC-006 (47/50 highlight corpus) | `highlight_corpus_test.go` | ✓ | — |
| SC-007 (≤ 500 ms p95 dismiss) | `dismiss_bench_test.go` | ✓ | — |
| SC-008 (≤ 16 ms p95 resize) | `tests/integration/resize_test.go` | ✓ | — |

## Coverage gate

Per-package coverage threshold is **≥ 80%**, computed against the merged
profile (`make cover`). The CI gate fails on the first package below
threshold; the dev-loop equivalent is:

```bash
make cover && go tool cover -func=coverage.out | awk '$3+0 < 80'
```

## REUSE / SPDX

Every file in the repository carries SPDX headers and is dual-licensed
`MIT OR Apache-2.0`. The `LICENSES/` directory holds the canonical
license texts; `REUSE.toml` enumerates files that can't carry inline
headers (binaries, test fixtures, etc.). Verify with `make reuse`
before merging any change that adds a file.

## Repository layout

See the "Project structure" section of [README.md](README.md). The
`internal/*` packages have a `doc.go` describing the package's purpose
and the contracts it implements; consult the corresponding section of
[contracts/internal-apis.md](specs/001-popup-reader/contracts/internal-apis.md)
when changing public types.

## Workflow

1. Pull the latest `main`.
2. Read the relevant section of `specs/001-popup-reader/`.
3. Write tests first (Constitution Principle II); tests must FAIL
   before the implementation lands.
4. `make all` before committing.
5. Use feature branches; commits include the SPDX header in the message
   only when introducing a new file format.

## Debugging

```bash
spy --debug=/tmp/spy.log <file>
```

writes a structured log with one event per line (level, ts, msg, fields).
Empty `--debug` (the default) disables logging entirely; nothing is
written to disk unless the flag is set.
