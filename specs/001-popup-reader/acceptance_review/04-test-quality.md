<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos
SPDX-License-Identifier: MIT OR Apache-2.0
-->

# spy v0.1.0 Test-Suite Quality Audit

**Audited:** 2026-04-26
**Scope:** repo HEAD on `main`, pre-tag.
**Default tags:** `go test -race` clean; `go test -race -count=2` clean (no flakes).

## TL;DR

The PTY harness in `tests/integration/pty.go` is real — it spawns the binary
under `creack/pty`, sends raw bytes, and reaps exit codes. The dismiss
benchmark and the resize test consume it for real assertions. **However,
every spec-mandated US1–US6 PTY integration test (T040, T051, T062, T072b,
T088, T096) plus the FR-015 / SC-008 signal test (T035b) is still
`t.Skip`'d** with a "PTY harness not yet implemented — Phase 9 T104 will
provide the runtime" message that is now stale. T104 landed; the harness
exists; the assertions never got wired. Constitution Principle II requires
PR-blocking `-race` integration coverage for the user stories — that
guarantee currently ships unverified end-to-end.

Beyond the skip cluster, the SC-002 / SC-001 benchmarks **bypass the PTY**
and exercise the model in-process, contradicting spec.md SC-002's wording
("driven through the PTY harness"); SC-009 has no fixtures and no test;
SC-010 is one of the silently-skipped files; SC-011's e2e shell script
deliberately defers PTY assertions; SC-012 (3-reviewer panel) is open
(T109/T109a marked `[ ]`); the FuzzConfigLoad ≥60s campaign is documented
as required pre-tag but no seed corpus directory exists and there's no
evidence it was run.

---

## CRITICAL findings — spec-mandated tests not enforcing

### C1. US1 highlighted-file PTY integration test is skipped
- **File:** `/home/knitli/spy/tests/integration/text_review_test.go:33`
- **Spec:** US1 acceptance #1 (SC-001 wording, FR-003).
- **Task:** T040 marked `[X]`.
- **Status:** `t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")`. Phase 9 (T104) is `[X]`; the harness ships; the assertions are documented in the file body but never lifted into runnable code.
- **Gap-closer:** Replace the skip with the 6 assertions in the docblock (alt-screen entry, "package" SGR-wrapped, "fmt.Println" present, gutter increments on Down arrow, exit 0 on `q`). The harness's `WaitFor` / `Snapshot` / `Send` / `ExitCode` cover all six.

### C2. US2 search/jump PTY integration test is skipped
- **File:** `/home/knitli/spy/tests/integration/search_test.go:39`
- **Spec:** US2 acceptance #2/#3/#4 (`/`, `?`, `n`, `:N`, `:$`, search wrap status).
- **Task:** T051 `[X]`.
- **Status:** `t.Skip(...)` — same stale reason. The 9-step assertion list (forward search, `:1`, `:$`, `n`-wrap, vim-mode `gg`/`G`/`Ctrl-D`/`Ctrl-U`) is documented in the comment but unimplemented.
- **Gap-closer:** Lift the documented asserts; they are within the harness's surface today.

### C3. US3 theme-auto/OSC-11 PTY integration test is skipped
- **File:** `/home/knitli/spy/tests/integration/theme_test.go:51`
- **Spec:** US3 acceptance #1/#2/#3 (auto-light from OSC 11, `--theme` flag override, env override, `NO_COLOR`).
- **Task:** T062 `[X]`.
- **Status:** Skipped. The OSC 11 PTY responder (the part the harness can't yet do — needs an OSC 11 query handler in the PTY-side responder) is genuinely missing infrastructure, but the flag-override and env-override sub-cases (steps 7–9 in the docblock) need only the existing harness.
- **Gap-closer:** Split into two tests: (a) flag/env override coverage that runs today, (b) OSC-11-responder case kept as deferred until the harness grows an OSC handler. Today the entire scenario is silently skipped.

### C4. US4 PDF graphics + text-fallback test is skipped (SC-010)
- **File:** `/home/knitli/spy/tests/integration/pdf_test.go:43`
- **Spec:** SC-010 names this exact path: "Measured by tests/integration/pdf_test.go".
- **Task:** T072b `[X]`.
- **Status:** Skipped. The non-graphics text-fallback case (extract via pdfcpu and assert the rendered frame contains "Dummy PDF file") has no PTY-responder dependency; only the `-tags fitz` Kitty-payload round-trip needs the reference decoder.
- **Gap-closer:** At minimum, the (b) sub-case (text fallback, `dummy.pdf`, sentinel substring) can run today against the existing harness and fixture (`tests/e2e/fixtures/dummy.pdf` exists). SC-010 currently has zero machine-checked enforcement.

### C5. US4 graphics protocol dispatch test is skipped (SC-009)
- **File:** `/home/knitli/spy/tests/integration/graphics_test.go:36` and `:49` (TestGraphics_KittyPayloadDispatch + TestGraphics_CleanupOnQuit)
- **Spec:** SC-009 names `tests/integration/graphics_test.go`.
- **Status:** Both tests skipped. Worse: **the fixtures the spec demands don't exist** — `tests/fixtures/img/{small.png, medium.jpg, large.gif}` is missing. `find /home/knitli/spy/tests/fixtures -mindepth 2 -type d` returns nothing. Only `tests/fixtures/_highlight-corpus/` exists.
- **Gap-closer:** Create `tests/fixtures/img/` with the three fixtures and lift the documented assertions. Until then SC-009 is unverified.

### C6. US5 stdin pipe PTY test is skipped
- **File:** `/home/knitli/spy/tests/integration/stdin_test.go:39`
- **Spec:** US5 acceptance #1/#2 (`<stdin>` footer, language detection, no-disk-write).
- **Task:** T088 `[X]`.
- **Status:** Skipped. The `tests/e2e/05_pipe.sh` script covers degenerate-cat verbatim contracts but explicitly defers footer / highlight assertions to the harness (script comments say so). So the in-app `<stdin>` footer is unverified end-to-end.
- **Gap-closer:** Lift the 7-step assertion list — feeding stdin via a pipe while stdout is the PTY is supported by `creack/pty` (need to construct `cmd.Stdin = pipeReader` before `pty.StartWithSize`).

### C7. US6 footer counter PTY test is skipped
- **File:** `/home/knitli/spy/tests/integration/footer_test.go:41`
- **Spec:** US6 (file metadata + position).
- **Task:** T096 `[X]`.
- **Status:** Skipped. PageDown advancement of "Line N", basename present, "100 lines" footer, narrow-collapse format — none verified.

### C8. FR-015 / SC-008 signal-handling tests are skipped
- **Files:** `/home/knitli/spy/tests/integration/signal_test.go:33` and `:39`
- **Spec:** FR-015 + SC-008 + Constitution Principle II (PR-blocking signal gate).
- **Task:** T035b `[X]`.
- **Status:** Both `TestSIGINTRestoresTerminal` and `TestSIGTERMRestoresTerminal` skipped. The harness has `Signal()` (with 0% coverage — proof it's never called from any test). Skip reason references "Phase 9 alongside T104" — Phase 9 done.
- **Gap-closer:** `p.Signal(syscall.SIGINT)` after `WaitFor(AltScreenEnter, ...)`, then `p.WaitForExit(1*time.Second)` and assert `ExitCode() == 130` and the snapshot contains `\x1b[?1049l`. All harness primitives exist.

### C9. SC-002 scroll benchmark does NOT use the PTY
- **File:** `/home/knitli/spy/tests/perf/scroll_bench_test.go:37`
- **Spec wording (spec.md:192):** "Measured by tests/perf/scroll_bench_test.go: 100 sequential ScrollDown actions **driven through the PTY harness**".
- **Status:** The test exercises `ui.Model.Update(tea.KeyMsg{Type: tea.KeyDown})` in-process, which bypasses input dispatch, raw-mode handling, and PTY drain latency — exactly the layers SC-002 is meant to gate. The threshold check (p95 ≤ 16 ms) does fail the test, but against a strictly easier path than the spec promises.
- **Gap-closer:** Either (a) re-anchor the benchmark on the PTY (mirror the dismiss benchmark's pattern), or (b) update spec.md SC-002 to acknowledge the in-process measurement and add a separate PTY-driven smoothness test. Today the spec's wording and the test do not match.

### C10. SC-001 first-frame benchmark does NOT use the PTY
- **File:** `/home/knitli/spy/tests/perf/firstframe_bench_test.go:39`
- **Spec wording:** "open and view ... in under 100ms from invocation".
- **Status:** The test omits the binary spawn altogether — comment at line 36 says "does NOT spawn the binary because the spec budget is 'from invocation', excluding the ~50–80 ms typical Go runtime startup that's platform-dependent". This is a deliberate scope reduction; the spec says "from invocation" and the benchmark measures everything except invocation. Whether the spec wording or the test is correct is a spec-clarification question, but as written they conflict.
- **Gap-closer:** Either tighten the spec wording (acknowledge the exclusion of cold-start Go runtime) or run a real binary-spawn variant. Today an SC-001 regression caused by the binary's startup path would not be caught.

### C11. FuzzConfigLoad ≥60s pre-tag campaign not run
- **File:** `/home/knitli/spy/internal/config/fuzz_test.go:25`
- **Spec / checklist:** `specs/001-popup-reader/checklists/security-review.md:55-57` states "Run with `go test -fuzz=FuzzConfigLoad ./internal/config/...` for ≥ 60 s before tagging v0.1.0".
- **Status:** The fuzz function has 8 inline `f.Add` seeds. There is **no `internal/config/testdata/fuzz/FuzzConfigLoad/` corpus directory** — i.e., no committed crashes or coverage-driven inputs. The 60 s campaign is referenced as a manual pre-tag step; no CI job runs it; no evidence it has been run for this branch.
- **Gap-closer:** Run `go test -run=^$ -fuzz=FuzzConfigLoad -fuzztime=60s ./internal/config/...` and commit any resulting `testdata/fuzz/.../*` artifacts. Add a nightly CI job (similar to the existing `-tags perf` nightly).

---

## HIGH findings — shallow / mismatched coverage

### H1. SC-004 PR gate is scaled to 60 lines, not the spec's 10 000
- **File:** `/home/knitli/spy/tests/perf/theme_swap_bench_test.go:63-69`
- **Spec wording (SC-004):** "averaging 100 swaps against a 10 000-line file ... ≤ 16 ms p95".
- **Status:** `TestThemeSwap_Under16ms` runs against 60 lines (justified by a renderer architectural gap acknowledged in the comment). `TestThemeSwap_FullSpecCase` runs the spec's 10 000-line case but is **advisory only** (`failOnBudget=false`). Net: SC-004 the PR gate enforces a 60-line proxy; the 10 000-line spec case logs a number but never fails. T104c marks this advisory in the task body, but the SC promised a blocking gate.
- **Gap-closer:** Either fix the renderer's per-line formatting cost so the strict 10 000-line gate is achievable (the comment marks this as a known gap), or update spec SC-004 to acknowledge the 60-line proxy and pin the 10 000-line case as advisory only.

### H2. SC-005 PR-gate measures HeapInuse, not RSS
- **File:** `/home/knitli/spy/tests/perf/large_file_test.go:100-106`
- **Spec wording:** "consuming more than 500MB of memory" (RSS).
- **Status:** The benchmark measures `runtime.MemStats.HeapInuse` after a `runtime.GC()`. The comment explicitly acknowledges this is "a closer-to-RSS approximation than HeapAlloc" but is not RSS — it excludes the Go runtime overhead, mmap'd image data via cgo, etc. A real RSS regression that lives outside the heap (e.g., mmap'd window backing for the large-file streaming path) is invisible to this gate.
- **Gap-closer:** Read `/proc/self/status` (`VmRSS:` line) on Linux for the actual RSS. Keep HeapInuse as a secondary log line.

### H3. SC-011 e2e script verifies degenerate-cat path, not US5 acceptance
- **File:** `/home/knitli/spy/tests/e2e/05_pipe.sh:18-22`
- **Spec wording (SC-011):** "displays Go-highlighted content with `<stdin>` in the footer; ... displays diff-highlighted content; ... All three exit 0 on `q`."
- **Status:** The script comments deliberately defer the `<stdin>` footer + highlight + `q`-exit assertions to "when T104 lands"; instead it pins the verbatim degenerate-cat contract (cmp -s of byte-equal output). Useful guard, but does not enforce SC-011 as written. SC-011 is currently unverified end-to-end.
- **Gap-closer:** Lift the deferred assertions in the same way as the integration tests. The harness is ready; this script is the only existing surface for the alt-screen-with-piped-stdin shape.

### H4. PTY harness has dead helpers — Resize, Signal, Read, CopyOutput, ReadGolden, WriteGolden, DiffFrames, indexByte
- **File:** `/home/knitli/spy/tests/integration/pty.go` and `/home/knitli/spy/tests/integration/helpers.go`
- **Status:** Coverage on the harness package is 61.0%; the unused functions are all signals of deferred work that never landed:
  - `pty.go:141 Read` (0%): never called — every consumer uses `Snapshot` then `Read`-style consumption is replaced by re-snapshot.
  - `pty.go:198 Resize` (0%): never called — SC-008 resize test bypasses the binary and exercises the model directly. Real PTY resize behaviour is unverified.
  - `pty.go:210 Signal` (0%): never called — directly proves C8 (signal tests skipped).
  - `pty.go:412 CopyOutput` (0%): debugging helper — fine to leave but flag it as such.
  - `pty.go:399 indexByte` (0%): reimplementation of `bytes.IndexByte` for `mergeEnv` — `mergeEnv` itself is at 21.4% coverage because no test passes a non-nil env (the test that would, doesn't exist yet — likely the theme env-override test from C3).
  - `helpers.go:48 isLoaderWarning`, `:56 ReadGolden`, `:68 WriteGolden`, `:83 DiffFrames` (all 0%): the golden-file infrastructure was scaffolded for T040 (per the comment at `:67-77`) and never used. T038's golden-file equality test was deferred per tasks.md:153.
- **Gap-closer:** Either land the deferred tests and lift coverage, or delete the dead helpers and inline the documentation that references them. Current state is "test infrastructure that documents promised tests" — cosmetically clean, functionally hollow.

### H5. cmd/spy `main()` and `run()` are 0% / 50.8% covered
- **File:** `/home/knitli/spy/cmd/spy/main.go:43,56`
- **Status:** `main()` 0% (acknowledged advisory in T111 — needs binary-instrumented coverage). `run()` 50.8% — the unit tests cover error / version / help paths but the main success path (alt-screen launch, Bubble Tea program loop) is not covered by the cmd/spy unit tests *and* is not covered end-to-end by integration tests (because of C1–C8). The cmd/spy package coverage of 76.6% is not the issue; the issue is that the alt-screen launch path has neither unit nor integration coverage despite the v0.1.0 promise.
- **Gap-closer:** Closing C1–C8 covers most of `run()`'s remaining gap. The 0% on `main()` itself is a Go limitation pre-1.20-style binary instrumentation; document explicitly rather than treating "76.6 advisory" as "fine".

---

## MEDIUM findings — dead helpers, stale TODOs, marginal coverage

### M1. SC-009 fixtures missing — entire SC has no test
- **Path:** `tests/fixtures/img/{small.png, medium.jpg, large.gif}` per spec.md:199.
- **Status:** Directory does not exist. C5 captures the test side; this is the fixture side.

### M2. Three `t.Skip` calls in window_test.go are conditional on threshold-not-crossed
- **File:** `/home/knitli/spy/internal/loader/window_test.go:93,120,236`
- **Status:** Each skip is "if buffer.Windowed() returned false, this scenario isn't reachable". Defensive but indicates the test setup is not deterministic — the windowed-mode flip depends on a 50-line `WindowSize` and a 10 KiB `MaxResidentBytes` against a 500-line × 200-byte body. If a future refactor changes the eviction trigger, these tests silently no-op.
- **Gap-closer:** Pre-assert `buf.Windowed() == true` instead of skip; choose fixture sizes that guarantee the flip.

### M3. Three `t.Skip` calls in capabilities_test.go / theme_test.go skip when running with a real TTY
- **Files:** `/home/knitli/spy/internal/term/capabilities_test.go:184,195` and `/home/knitli/spy/internal/term/theme_test.go:273`
- **Status:** Justified — the env-fallback / non-bypass paths can only be exercised when stdin/stdout aren't a TTY. CI is fine here; local interactive runs may silently skip. Worth noting in the test docstring.

### M4. TestUpdate_ResizePreservesYOffset can no-op in some scenarios
- **File:** `/home/knitli/spy/internal/ui/model_test.go:277`
- **Status:** Skips if "viewport refused to scroll". Same M2 pattern — make the setup deterministic so the SC-008-adjacent assertion always runs.

### M5. cmd/spy main_test.go OS-dependent skips are healthy
- **File:** `/home/knitli/spy/cmd/spy/main_test.go:86,89` and `internal/source/file_test.go:107,110,138`
- **Status:** Justified — Windows permission semantics + root bypass. Keep.

### M6. internal/ui at 85.5% with shallow `View()` exercise via newTestModel
- **Status:** model_test exercises `Update` deeply but `View()` mostly via `applyResize` driver. The footer-collapse-at-narrow-width contract (US6, footer_test.go:35-37) is not unit-tested either — only documented in the skipped integration test.
- **Gap-closer:** Add a unit test that asserts `View()` output contains/lacks the `|` separators at 60 vs 100 cols.

### M7. Resize test runs only `!race` — no race coverage of the resize path
- **File:** `/home/knitli/spy/tests/integration/resize_test.go:5` (`//go:build !race`)
- **Status:** Justified for the wall-clock budget but means race conditions in the resize-handler / wrap-cache invalidation path go uncaught. A resize-flood test under `-race` (without budget assertions) would close this gap.

---

## LOW findings — naming / style

### L1. Sanity-check helper `contains` re-implements `strings.Contains`
- **File:** `/home/knitli/spy/tests/integration/pty_sanity_test.go:106`
- **Status:** Comment says "tiny helper ... so we don't pull in strings.Contains for a one-liner". `strings` is already imported transitively in this package via other test files. Not a bug.

### L2. Test names are descriptive — no findings
- All test names follow `TestThing_BehaviourWhenCondition` shape. Constitution-compliant.

### L3. Skip messages are accurate for harness-not-implemented but stale post-T104
- The "PTY harness not yet implemented — Phase 9 T104 will provide the runtime" string appears in 8+ locations. Post-T104 these messages are wrong-on-the-merits. Update or delete.

---

## Coverage summary (re-validated against `go test ./... -race -coverprofile=/tmp/spy-cov-default.out -short`)

| Package | Coverage | Honesty |
|---------|----------|---------|
| `cmd/spy` | 76.6% | Advisory per T111 — `main()` 0%, `run()` 50.8%; alt-screen launch path has no unit *or* integration coverage (C1–C8 cluster) |
| `internal/config` | 89.9% | Solid; FuzzConfigLoad seed-only — no 60 s campaign run (C11) |
| `internal/graphics` | 91.1% | High but the *cleanup-on-panic / cleanup-on-SIGINT* cross-process path is unverified (C5, C8) |
| `internal/highlight` | 93.1% | Solid |
| `internal/keys` | 98.9% | Solid |
| `internal/loader` | 88.3% | Three conditional skips (M2) |
| `internal/render` | 83.8% | Just over threshold; sanitize.go path is unit-covered (escape_injection_test.go) |
| `internal/search` | 81.8% | Right at threshold |
| `internal/source` | 85.5% | OS-dependent skips justified |
| `internal/term` | 84.2% | TTY-dependent skips justified |
| `internal/ui` | 85.5% | Footer-collapse contract unit-untested (M6); resize path under `!race` only (M7) |
| `tests/integration` | 61.0% | Dead helpers (H4) — Resize 0%, Signal 0%, Read 0%, ReadGolden 0%, WriteGolden 0%, DiffFrames 0%, isLoaderWarning 0%, CopyOutput 0%, indexByte 0%, mergeEnv 21.4%. Each 0% function is direct evidence of a deferred test that never landed. |

`go test -race -count=2 ./internal/...`: clean, no flakes detected.
`go test ./tests/perf/... -short`: clean (everything skips under `-short` except the corpus test).

---

## Net assessment

The repo's test infrastructure is **structurally sound**: real PTY harness,
real fuzz fixture, sensible build tags (`!race` for wall-clock benches,
`perf` for nightly), CI gates documented. The product code quality of the
tests is also fine where they exist (table-driven, descriptive names,
neutralisation-byte-by-byte sanitiser asserts, etc.).

The problem is **completion**: the same staging strategy ("ship a `t.Skip`
that documents the assertions; lift them when the harness ships") was
applied to 8 spec-mandated integration tests, the harness shipped, and
nobody walked the list. The result is a v0.1.0 candidate where:

- 6/6 user stories have unit-level coverage but **zero PR-gating
  end-to-end coverage** (C1, C2, C3, C4/C5, C6, C7).
- The only PR-gating signal-handling test pair is skipped (C8) — direct
  Constitution Principle II violation.
- Two of the eight measurable Success Criteria (SC-001, SC-002) measure
  a strictly easier path than the spec promises (C9, C10).
- One of the eight (SC-004) gates a 60-line proxy and lets the
  10 000-line spec case log silently (H1).
- One of the eight (SC-011) gates the verbatim-cat contract instead of
  the `<stdin>`-footer / highlighted-frame promise (H3).
- Two of the eight (SC-009, SC-010) are completely unverified (C4, C5).
- SC-012 is correctly tracked as `[ ]` pending human reviewers (T109a) —
  honest.

**Pre-tag recommendation:** the C-class findings are PR-blockers under
Constitution Principle II as written. The mechanical lift from
"documented assertions in skipped test bodies" to "assertions in `func
Test...`" is small (the harness has every primitive needed) and is the
honest path to v0.1.0. The stale skip messages should not survive the
audit — either lift the test or delete it and update the spec to match.
