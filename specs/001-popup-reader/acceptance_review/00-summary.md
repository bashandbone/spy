<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos
SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Acceptance Review — `spy` v0.1.0 / spec 001-popup-reader

**Date**: 2026-04-26
**Branch reviewed**: `main` at commit `d2e18f5` (just-merged PR #14, phase 9)
**Method**: Five specialist agents reviewed in parallel + manual cross-checks
**Decision**: **REQUEST CHANGES — do not tag v0.1.0 yet**

Per-agent reports kept verbatim in `/tmp/`:

- `/tmp/spy-traceability-report.md` — spec → code coverage map
- `/tmp/spy-go-review.md` — Go code review
- `/tmp/spy-security-review.md` — independent security review (vs T109b checklist)
- `/tmp/spy-test-quality.md` — test-suite quality audit
- `/tmp/spy-perf-audit.md` — benchmark honesty audit

---

## Executive Summary

The build is clean (`go build ./...` and `-tags fitz` both green), `go test ./... -race` passes, every internal package clears the 80% coverage gate, and the security and quickstart checklists were filled out. The architecture and concurrency design (loader pipeline, bounded channels, defer chain in `cmd/spy/main.go`) are solid.

**However, there is a single dominant defect repeated across CRITICAL findings**: when the PTY harness landed in Phase 9 (T104), nobody walked the list of integration tests that had been authored as `t.Skip` stubs with a comment saying "PTY harness not yet implemented — Phase 9 T104 will provide the runtime". **All eight of those tests are still skipped.** They cover FR-015 (signal handling), SC-008 (resize behaviour through PTY), SC-009 (graphics protocol round-trip), SC-010 (PDF), and the user-facing acceptance scenarios for US1, US2, US3, US5, US6.

Layered on top of this: the `tests/perf/` benchmarks are **not** wired into CI. The PR workflow runs `make test-race` and `make cover`, neither of which touches `tests/perf/`. So the SC-001..SC-008 "PR gate" benchmarks gate nothing in practice.

A handful of additional findings — one wrong claim in the security checklist, two genuine escape-injection holes, missing fixture directories, missing signal handlers — round out the picture.

---

## CRITICAL (block tag)

### C1. All deferred PTY integration tests still `t.Skip`

The harness shipped in `tests/integration/pty.go` (real `creack/pty` subprocess driver — `Send`, `Snapshot`, `Resize`, `Signal`, `ExitCode`). It is used by `pty_sanity_test.go` and `dismiss_bench_test.go`. Yet the eight US/SC tests still have stale skips:

| File | Line | Spec covered |
|---|---|---|
| `tests/integration/text_review_test.go:33` | US1 / FR-003 |
| `tests/integration/search_test.go:39` | US2 / FR-006/7 |
| `tests/integration/theme_test.go:51` | US3 / FR-004 |
| `tests/integration/pdf_test.go:43` | SC-010 |
| `tests/integration/graphics_test.go:36,49` | SC-009 |
| `tests/integration/stdin_test.go:39` | US5 / FR-002 |
| `tests/integration/footer_test.go:41` | US6 |
| `tests/integration/signal_test.go:33,39` | FR-015 / SC-008 (Constitution Principle II PR blocker) |

All eight files have the planned assertions documented in the test bodies. The lift from `t.Skip` → live test is mechanical given the harness exists.

**Action**: lift all 8 skips. Expect some to fail initially — that's the point.

### C2. Wrong exit codes for SIGINT and SIGTERM (`cmd/spy/main.go:200-215`)

`contracts/cli.md` requires exit 130 (SIGINT) and 143 (SIGTERM). Bubble Tea v1.3.10 catches SIGINT internally, sends `tea.Quit`, and `prog.Run()` returns nil — at which point `cmd/spy/main.go:215` returns `exitOK` (0). There is no `os/signal` handler anywhere in `cmd/spy/`:

```
$ grep -n "signal\." /home/knitli/spy/cmd/spy/main.go
(no output)
```

Even the regression-protection test for this is `signal_test.go:33,39` — both skipped (C1).

**Action**: install `signal.Notify` before `tea.NewProgram`, record which signal fires, translate to `os.Exit(128 + signum)` (or re-raise after cleanup).

### C3. CI does not run `tests/perf/`

`.github/workflows/ci.yml:37-44` runs `make lint`, `make vet`, `make test-race`, `make cover`. `make perf` is `go test -tags perf ./tests/perf/...` — runs only on the nightly workflow. Therefore every "PR gate" benchmark in `tests/perf/` (SC-001 first frame, SC-002 scroll, SC-003 search, SC-004 theme swap, SC-005 200 MiB tier, SC-006 highlight corpus, SC-007 dismiss) is enforced by **no automation at all** between merges. They will silently rot.

**Action**: add `go test ./tests/perf/...` (no `-tags perf`) to the PR workflow alongside `make test-race`.

### C4. Markdown and PDF outputs bypass `neutralizeEscapes`

The implementer's security checklist (T109b.c) claims escape injection is closed via `neutralizeEscapes` in `code.go`, `match.go`, `text.go`. Confirmed those three are wired. **But**:

- `internal/render/markdown.go:78`/`86-89` passes raw line bytes to `glamour.Render`. Glamour's goldmark backend does not strip `\x1b`/`\x9b` from non-code-block content. A markdown file with an embedded OSC sequence (e.g., a comment ` <!-- \x1b]2;evil\x07 --> `) reaches the terminal verbatim.
- `internal/render/pdf.go:186-205` calls `formatTextPage` with bytes from `p.GetPlainText(nil)` (the **default** code path in all no-fitz builds). A crafted PDF with OSC sequences in its content stream is enough.
- `internal/render/statusbar.go:119,181`, `internal/render/image.go:134,142`, `internal/render/pdf.go:199,201,215`, and `cmd/spy/main.go:295,304,307,310,313` all emit `DisplayName()` / file-path strings without sanitisation. Linux filenames can contain `\x1b`. `spy <evil_filename>` writes the bytes through `theme.Footer.Render` to the terminal — window-title hijack, clipboard write on emulators that honour OSC 52.

**Action**: apply `neutralizeEscapes` to (a) markdown raw input pre-Glamour, (b) PDF text-extraction output, (c) every `DisplayName()` / file-path emission site listed above.

### C5. Security checklist falsely claims `defer recover()` for graphics decoders

`specs/001-popup-reader/checklists/security-review.md:108-122` claims `internal/graphics/graphics.go:60` has `defer recover()` around `image.Decode` / `go-fitz`, asserted by `TestGraphics_RecoversFromDecoderPanic`. Reality:

```
$ grep -rn "recover()" /home/knitli/spy/internal/
internal/loader/window.go:301:	defer func() { _ = recover() }()    # <- in sendWarning, not graphics
internal/graphics/graphics_test.go:70:		if r := recover(); r != nil {  # <- test-only, not production
$ grep -rn "TestGraphics_RecoversFromDecoderPanic" /home/knitli/spy/
(no output)
```

A panic from `image.Decode` (`internal/render/image.go:117`), `fitz.NewFromMemory` (`internal/render/pdf_fitz.go:38`), or `doc.Image` (`internal/render/pdf_fitz.go:46`) tears down the program and corrupts the terminal (defer chain still runs — `restore` is reached — but the process dies on a malformed image).

**Action**: either (a) actually add the deferred recover and the test, or (b) update the checklist to be honest about the gap and downgrade the v0.1.0 promise.

### C6. SC-009 / SC-010 fixture directories don't exist

Spec promises fixtures at `tests/fixtures/img/{small.png 32 KB, medium.jpg 5 MB, large.gif 49 MB}` (SC-009) and `tests/fixtures/pdf/multi-page.pdf` (SC-010).

```
$ ls /home/knitli/spy/tests/fixtures/img/
ls: cannot access ...: No such file or directory
$ ls /home/knitli/spy/tests/fixtures/pdf/
ls: cannot access ...: No such file or directory
```

The PDFs that exist live at `tests/e2e/fixtures/{dummy.pdf,multi-page.pdf}`. Image fixtures don't exist anywhere.

**Action**: either move fixtures to the spec'd path, or amend the spec to point at `tests/e2e/fixtures/`. Add the missing image fixtures.

### C7. `Stream.Errs` is never consumed by the production UI

`internal/loader/stream.go:65-72` documents the warning channel; `internal/loader/window.go:309` references it in a comment. Production consumers in `internal/ui/`:

```
$ grep -n "stream\.Errs\|\.Errs\b" /home/knitli/spy/internal/ /home/knitli/spy/cmd/ --include='*.go' | grep -v _test.go
internal/loader/window.go:309:    # comment only
internal/loader/stream.go:72:     # comment only
```

Result: `WarnLineTruncated` (per-line cap, FR-013-adjacent) and `WarnStdinNonSeekable` (FR-012) warnings reach the channel and are silently dropped. The user is never told that content was clipped or that scroll-back is disabled.

**Action**: add a `waitForStreamErr` Bubble Tea command analogous to `waitForChunk` and route arrivals to `m.statusAdvisory` (the existing 5 s auto-clear pipeline).

---

## HIGH

### H1. T111 marked `[X]` despite contradicting itself

`specs/001-popup-reader/tasks.md:372` — T111 is the "final gate" task, marked `[X]`, but its own description explicitly says "T109 (macOS) and T109a (independent reviewers) remain as separate human-reviewer blockers for the actual v0.1.0 tag". `quickstart-validation.md:46-87` shows all macOS rows and Reviewers 2 & 3 PENDING. **SC-012 has zero functional enforcement.**

**Action**: revert T111 to `[ ]` until T109 + T109a actually complete. The "task is done" semantics are load-bearing for the changelog.

### H2. SC-008 resize PR gate is fragile

`tests/integration/resize_test.go` enforces p95 ≤ 16 ms across 50 random-width resize events on a 10 000-line file. The test passes locally on this machine (0.587 s) but the traceability agent measured `p95 = 23.34 ms` on its run. Likely flaky under CI noise. If C3 ever lands (perf in PR CI), this will be the first to flake.

**Action**: characterise variance over 10 runs, then either widen the budget with documentation, separate the assertion from the benchmark, or pin a runner profile.

### H3. SC-004 PR gate scaled to 60 lines, full-spec case is `t.Logf`

`tests/perf/theme_swap_bench_test.go:63-83`: PR-gate test asserts on a 60-line file (1/166 of spec). The 10 000-line "TestThemeSwap_FullSpecCase" has `failOnBudget=false` — measured 813 ms p95 (~50× budget) and the failure is logged-only. Documented as advisory, no follow-up issue filed.

**Action**: file an explicit follow-up issue + link from `tasks.md`. Either fix viewport-only formatting (the documented PERF NOTE) or amend SC-004.

### H4. SC-005 measures `HeapInuse`, not RSS

`tests/perf/large_file_test.go:100` and the nightly tier both read `runtime.MemStats.HeapInuse`. Spec says RSS. Cgo allocations (notably `go-fitz` for SC-010) are completely absent from `HeapInuse`, so the 500 MB promise is unverified for the cgo build.

**Action**: read `/proc/self/status:VmRSS` (Linux) or `getrusage` for honest RSS. Or amend the spec.

### H5. Synchronous full-buffer search blocks the event loop

`internal/ui/update.go:435-451`: `runSearch` drains the unbuffered `search.Scan` channel on the Bubble Tea goroutine. With `MaxResidentBytes == 0` (whole file resident), a 50 MB / 1 M-line file freezes key/resize events for seconds. `search.State.Pending` exists but is never set; the async path is documented but unimplemented.

**Action**: return a `tea.Cmd` that drains the channel and emits a `searchResultMsg`; wire `Pending` and per-search context cancellation.

### H6. Contract drift: `graphics.PDFPage(path, n, dpi)` doesn't exist

`contracts/internal-apis.md` documents `graphics.PDFPage(path, n, dpi)`. Actual implementation: unexported `internal/render/pdf_fitz.go:26 rasterizePDFPage(src source.Source, page int)` — wrong package, no DPI parameter. Plus `internal/render/pdf.go` uses `ledongthuc/pdf`, not `pdfcpu` as spec.md SC-010 and tasks T072b/T079 say.

**Action**: update `contracts/internal-apis.md` to match reality, and update `spec.md` SC-010 to name the actual library. Drift in the contract is OK; drift in the spec's success-criterion-pinning library name will look like a half-implemented promise to a future reader.

### H7. Filename-driven escape injection (defense-in-depth)

Linux filenames may contain `\x1b]2;...\x07`. The footer/statusbar/error-message paths render `DisplayName()` without sanitisation (covered partly under C4 but worth a separate exploit-level note). `spy '<evil filename>'` modifies the terminal title before the alt-screen even opens, because the early stderr writes in `cmd/spy/main.go:295,304,307,310,313` echo the filename verbatim.

**Action**: sanitise file paths at the stderr boundary in `cmd/spy/main.go` and at every `DisplayName()` emit site (overlaps C4).

---

## MEDIUM

| # | Location | Issue |
|---|---|---|
| M1 | `internal/source/file.go:46` | File-mode rejection only blocks directories. FIFOs, sockets, char/block devices accepted. (Pseudo-fs denylist is a documented FOLLOWUP — fine. This is narrower.) |
| M2 | `internal/source/file.go:42,77` | TOCTOU: `EvalSymlinks` → `Stat` → `Open` without `O_NOFOLLOW`. `ActionReload` re-uses cached detection. |
| M3 | `internal/render/pdf.go:38-68` | `pdfRenderer` cache mutation has no synchronization. Safe today (single goroutine), but undocumented invariant. |
| M4 | `internal/loader/stream.go:117-128` | Double-`rc.Close` on error path — call `defer rc.Close()` for ownership clarity. |
| M5 | `internal/loader/window.go:168-175` | `Total()` returns 0 before first chunk arrives → `search.scanLoop` bails with no match and no message if user types `/` immediately after open. |
| M6 | `internal/ui/update.go:703-727` | `:open` in-flight goroutine leaks on rapid quit. Bubble Tea drops the resulting `tea.Cmd`'s message; new stream's cancel is never called. |
| M7 | `tests/integration/pty_sanity_test.go:51-57`, `dismiss_bench_test.go:99-113` | Known PTY flake: first `q` after first paint is dropped. Worked around with retry loops. Root cause never investigated. |
| M8 | `tests/perf/scroll_bench_test.go:69-90` | "Zero dropped frames" is counted but never asserted (only mentioned in the failure message). |
| M9 | `tests/perf/firstframe_bench_test.go:39` | SC-001 measures the renderer slice, explicitly excluding ~50–80 ms of Go startup. Spec says "from invocation". |
| M10 | `tests/perf/dismiss_bench_test.go:99-113` | SC-007 has a 200 ms poll-tick floor — regressions from 50 ms → 199 ms are invisible. |
| M11 | `tests/e2e/05_pipe.sh` | SC-011 e2e is non-TTY only. The `<stdin>`-in-footer interactive contract is admitted-deferred in the script comment. |
| M12 | `internal/config/fuzz_test.go` | 8 inline seeds; no `testdata/fuzz/...` corpus. The ≥60 s pre-tag fuzz campaign in `security-review.md:55-57` has no recorded run. |
| M13 | `contracts/internal-apis.md` | Other drift: `render.Renderer` adds undocumented `RowToLine` method; `loader.Stream` adds `Buffer *LineBuffer` field; `render.Dependencies` and `ui.ModelOptions` carry extra load-bearing fields. |
| M14 | `internal/ui/view.go` and statusbar | When `Total() == -1` during streaming, footer flips to `…`. Once stream ends the count locks in. The `metaUpdatedMsg` plumbing exists; verify no race writes a final count between two paint frames. |

---

## LOW

- `internal/render/sanitize.go:37` — `'?'` replacement is ambiguous; `U+FFFD` is the conventional choice.
- `internal/render/text.go:119` — wrap uses rune-count, not terminal cell-width; CJK/emoji overflow by one cell per wide rune (documented Phase 2 limitation).
- `cmd/spy/main.go:320-325` — `boolPtr(false) == nil` conflates explicit `false` with "not set".
- `internal/loader/window.go:300-306` — `recover()` guard in `sendWarning` is overbroad; coordinate with `sync.Once` instead.
- Stale `t.Skip("PTY harness not yet implemented…")` messages litter 8 files even after the harness landed.

---

## Validation summary

| Check | Result | Notes |
|---|---|---|
| `go build ./...` | PASS | |
| `go build -tags fitz ./...` | PASS | |
| `go vet ./...` | PASS | |
| `go test ./... -race` | PASS | All 13 packages pass |
| Per-package coverage ≥ 80% | PASS for `internal/*` | `cmd/spy` 76.6% (advisory in CI script); `tests/integration` 61% (advisory) |
| `tests/perf/` benches | PASS locally | But not run by CI (C3) |
| `reuse lint` (CI) | not exercised in this review | trust the CI gate |
| `make perf` | not exercised in this review | nightly-only by design |

---

## Recommended action plan (in order)

1. **C1**: lift all 8 `t.Skip`s. Some will fail; that's the point. (~1 day)
2. **C2**: install `signal.Notify`; emit exit codes 130/143. (~30 min)
3. **C4 + H7**: wrap markdown/PDF/statusbar/image/stderr emit boundaries with `neutralizeEscapes`. (~1 hour)
4. **C5**: either add the `defer recover()` around graphics decoders or correct the security checklist. (~1 hour or 5 min)
5. **C6**: create or relocate fixture dirs to match spec. (~30 min)
6. **C7**: wire `Stream.Errs` to a status-bar advisory. (~1 hour)
7. **C3**: add `go test ./tests/perf/...` to PR workflow. (~5 min)
8. **H1**: revert T111 to `[ ]` and tag-block on T109/T109a. (~1 min)
9. **H5**: async search command. (~half day)
10. **H6**: reconcile contract + spec library name. (~30 min)
11. **MEDIUMs and LOWs**: triage; M1/M2/M3/M6/M8/M9/M10/M12 worth fixing pre-tag, others can ship as known issues with follow-up issues filed.

Once C1–C7 land, re-run this acceptance review.

---

## What looked good

- Loader concurrency: bounded `updates` channel (cap 4), context cancellation propagated, stale-chunk detection via stream pointer comparison, mutex coverage complete.
- Defer chain in `cmd/spy/main.go` correctly LIFO (graphics cleanup before terminal restore) per research R10.
- TOML fuzz seeds run clean (20 s spot run, 109 new inputs).
- OSC 11 regex is anchored, hex-only, dual-terminator, 64 B cap.
- CI no-network gate: real and narrowly scoped (`http.Get|Post|...`, `net.Dial*`, deliberately skips `.Get(`).
- Build-tag pairing `pdf_fitz.go` / `pdf_nofitz.go` is honest.
- Test naming is descriptive; AAA-style.
- `internal/keys` 98.9% coverage; `internal/highlight` 93.1%; `internal/graphics` 91.1%.
- `internal/source.FromArgs` handles the file/stdin/`-`/TTY matrix correctly.
- `defer rc.Close()` discipline is present in most Source open paths.

---

*End of acceptance review.*
