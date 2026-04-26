<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos
SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Spy Perf Benchmark Honesty Audit (v0.1.0)

Repo: `/home/knitli/spy` · Spec: `specs/001-popup-reader/spec.md` · Audit date: 2026-04-26

Severity legend: **CRITICAL** = SC promise unverified; **HIGH** = wrong-shape assertion; **MEDIUM** = drift/dead-code; **LOW** = doc/naming.

---

## Summary table (12 SCs)

| SC | Promise | Where it actually gates | Verdict |
|----|---------|-------------------------|---------|
| SC-001 | first-frame ≤100ms, 100-line file | `tests/perf/firstframe_bench_test.go` (in-process model wiring, no binary) | HIGH — measures only the renderer slice, explicitly excludes `~50–80ms Go startup`; spec wording is "from invocation". Asserts. |
| SC-002 | p95 ≤16ms over 100 ScrollDown on 10 000-line file, **zero dropped frames** | `tests/perf/scroll_bench_test.go` | HIGH — input/p95 correct, "zero dropped frames" is **counted but never asserted** (`dropped` only printed in fail-message). |
| SC-003 | search ≤500ms in >1MiB | `tests/perf/search_bench_test.go` | LOW — file is 1.25 MiB, needle on last line, p50 only (single sample). Asserts. |
| SC-004 | p95 ≤16ms re-render, **10 000-line** file, 100 swaps | `tests/perf/theme_swap_bench_test.go` | **CRITICAL** — PR gate runs **60 lines**, full-spec case is `t.Logf`-only (advisory) and never wired into nightly. |
| SC-005 | 1 GiB input, RSS ≤500 MB | `tests/perf/large_file_test.go` (PR), `large_file_perf_test.go` (nightly, `-tags perf`) | HIGH — PR gate is 200 MiB / 250 MiB (different shape). Nightly hits the spec, but RSS is `runtime.MemStats.HeapInuse`, not actual RSS. |
| SC-006 | ≥47/50 corpus files ≤1% Error tokens | `tests/perf/highlight_corpus_test.go` | MEDIUM — 50 fixtures present, **2 fail** (`sample.mm`, `sample.fish` no extension match). Threshold is scaled, so currently passes 50/52, but the spec's strict 47/50 is not what's evaluated. |
| SC-007 | dismiss p95 ≤500ms, 100 invocations on 10 000-line file | `tests/perf/dismiss_bench_test.go` (PR, 10 iter), `dismiss_perf_test.go` (nightly, 100 iter, `-tags perf`) | HIGH — PR gate uses 10 iterations; first-`q`-lost retry credits "200 ms" as the **timer floor** (not measurement). |
| SC-008 | resize p95 ≤16ms, 50 events, widths in [40,200], anchor preserved, wrap cache invalidated | `tests/integration/resize_test.go` | LOW — looks honest. Asserts all three. |
| SC-009 | 32K/5M/49M Kitty fixtures round-trip, exit 0, RSS ≤250MB | `tests/integration/graphics_test.go` | **CRITICAL** — entirely `t.Skip`. `tests/fixtures/img/` directory does not exist. SC-009 is **completely unverified** by the home test. |
| SC-010 | `graphics.PDFPage` under `-tags fitz` + pdfcpu sentinel in non-graphics PTY | `tests/integration/pdf_test.go` | **CRITICAL** — entirely `t.Skip`. `tests/fixtures/pdf/multi-page.pdf` does not exist (only `tests/e2e/fixtures/multi-page.pdf`). The renderer-level `internal/render/pdf_test.go` covers parts of the contract, but the spec's "home" file is dead. |
| SC-011 | three pipeline shapes pass | `tests/e2e/05_pipe.sh` | LOW — passes degenerate-cat (non-TTY) shapes; the actual SC-011 promise (alt-screen frame with `<stdin>` in footer) is admitted as "deferred until T104 PTY harness". So the *interactive* contract is unverified by this script. |
| SC-012 | 3-reviewer panel | out-of-band human work | N/A |

---

## Findings

### CRITICAL-1 — SC-004 strict gate runs at 60 lines, not 10 000

**File**: `tests/perf/theme_swap_bench_test.go:63-83`
**Spec**: SC-004 — re-render the visible viewport on theme swap, p95 ≤16 ms, **10 000-line** file, 100 swaps.
**Test**: `TestThemeSwap_Under16ms` asserts at `visibleScaleLines = 60`. `TestThemeSwap_FullSpecCase` runs at 10 000 lines but passes `failOnBudget=false`. The advisory case has no nightly wiring (`make perf` only runs `-tags perf`; this test has no perf build tag, so nightly never sees it).

The test file is up-front about this: line 56–62 admits the renderer formats every resident line on every paint and that "the strict 16 ms budget cannot be met at the spec's 10 000 lines today." That makes it explicit *engineering* honesty inside the file but the SC is **promised but not gated**.

**Fix**: Either add `//go:build perf` to `TestThemeSwap_FullSpecCase` and run it nightly, or implement viewport-only formatting and lift the strict gate to 10 000 lines. Update SC-004 in spec.md if the 60-line scope is the real gate.

---

### CRITICAL-2 — SC-009 is unverified (graphics integration test is `t.Skip`)

**File**: `tests/integration/graphics_test.go:35-50`
**Spec**: SC-009 specifies 32 KB / 5 MB / 49 MB image fixtures round-trip through Kitty in a PTY, exit 0, RSS ≤250 MB. `tests/integration/graphics_test.go` is named as the home for this measurement.
**Test**: Both `TestGraphics_KittyPayloadDispatch` and `TestGraphics_CleanupOnQuit` are `t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the runtime")`. The fixture directory `tests/fixtures/img/` does not exist (verified `ls`).

`internal/graphics/kitty_test.go` covers the protocol-encoding shape against a 16×16 PNG. That is **not** the SC-009 contract — no 49 MB fixture, no PTY exit code, no RSS bound.

**Fix**: Either implement T104 + the fixtures and unskip, or downgrade SC-009 in the spec to the encoding-level contract. Tagging v0.1.0 with this skipped is shipping an unverified SC.

---

### CRITICAL-3 — SC-010 is unverified (PDF integration test is `t.Skip`)

**File**: `tests/integration/pdf_test.go:42-44`
**Spec**: SC-010 explicitly names `tests/integration/pdf_test.go` as the home: rasterize page 1 via `graphics.PDFPage` under `-tags fitz` in a graphics PTY, AND extract page text via `pdfcpu` in a non-graphics PTY containing the sentinel "Dummy PDF file". Spec also references `tests/fixtures/pdf/multi-page.pdf`.
**Test**: `TestPDF_GraphicsAndTextFallback` is a single `t.Skip`. The fixture path `tests/fixtures/pdf/multi-page.pdf` does not exist (only `tests/e2e/fixtures/multi-page.pdf`).

`internal/render/pdf_fitz_test.go` and `internal/render/pdf_nofitz_test.go` cover the rasterization + sentinel-text contract at the renderer layer (they pass under `-tags fitz` — verified). That covers the *logic* but not the spec's PTY round-trip + exit code + RSS bound.

**Fix**: Same as CRITICAL-2. Either land the PTY harness or relax the SC to renderer-level only.

---

### HIGH-1 — SC-002's "zero dropped frames" is counted but not asserted

**File**: `tests/perf/scroll_bench_test.go:69-90`
**Spec**: "p95 ≤16 ms ... and zero dropped frames."
**Test**: `dropped` is incremented when `elapsed > 16ms`, but the only failure path checks `p95 > limit`. A test where p95 is 15 ms and `dropped == 4` (4/100 frames over budget) **passes**. The dropped count only appears inside the failure message body or the `t.Logf`.

The spec uses **two** thresholds joined by AND. The test enforces only one.

**Fix**: Add `if dropped > 0 { t.Fatalf("SC-002: %d dropped frames", dropped) }` after the p95 check (or fold into the existing assertion). The current run shows `dropped=0/100` so this is latent, but the gate doesn't actually enforce it.

---

### HIGH-2 — SC-001 measures the renderer, not "from invocation"

**File**: `tests/perf/firstframe_bench_test.go:33-89`
**Spec**: "in under 100ms from invocation"
**Test**: Doc comment is honest: "does NOT spawn the binary because the spec budget is 'from invocation', excluding the ~50–80 ms typical Go runtime startup that's platform-dependent and outside the renderer's control." The test wires `loader.Open` + `ui.NewModel.View()` directly; total wall-clock excludes process startup, arg parsing, stdio probe, terminal detection, theme probe, alt-screen entry.

This is a deliberate scope reduction — defensible engineering choice, but the SC as written promises something the test does not measure. Either the spec wording or the test scope needs to align.

Also: p95 across only 20 samples with `idx = (20*95)/100 = 19` is the *max* (p100), not p95. For n=20, p95 by nearest-rank would be ceil(20*0.95)=19 (1-indexed) → index 18 0-indexed. Off-by-one in either direction; matters less for small n but inflates apparent budget.

**Fix**: (a) Update spec to say "first frame paint" not "from invocation", or add a separate `tests/e2e` test that spawns the binary and times exec → first-paint via the PTY harness. (b) Compute p95 as `durations[int(math.Ceil(0.95*float64(n)))-1]` to get the documented semantics across all sample sizes.

---

### HIGH-3 — SC-007 PR gate uses 10 iterations and a 200 ms timer floor

**File**: `tests/perf/dismiss_bench_test.go:99-113`
**Spec**: "p95 ≤500 ms across 100 invocations"
**Test**: PR gate runs 10 iterations (acknowledged); the nightly tier runs 100 (`dismiss_perf_test.go`). Bigger problem: when the first `q` propagates and the process exits within the 200 ms poll, the recorded `elapsed` is set to the literal poll interval `200 * time.Millisecond` ("we credit the iteration with the polling tick — a strict upper bound on the real dismiss latency"). 

Effect: on every iteration where Bubble Tea's input pipeline does NOT race the first keystroke, the recorded sample is **synthetic** and equal to the poll tick. p95 across 10 such samples is 200 ms regardless of the real dismiss latency. The 500 ms budget is not the spec's ≤500 ms p95 — it's "≤500 ms or 200 ms upper-bound stub". A regression from 50 ms to 199 ms real dismiss would be invisible.

**Fix**: Use a finer poll (e.g., 5–10 ms) or wait for an actual exit signal before recording. Better: capture the t0 just before `p.Send("q")` unconditionally and let the actual `WaitForExit` time-stamp t1.

---

### HIGH-4 — SC-005 PR gate measures HeapInuse, not RSS, and at 200 MiB

**File**: `tests/perf/large_file_test.go:30-106`
**Spec**: "1 GiB input, RSS ≤500 MB"
**Test**: PR gate writes 200 MiB and asserts `delta ≤ 250 MiB`. Different file size, different memory bound. Nightly (`large_file_perf_test.go`) has the right size (1 GiB) and bound (500 MiB) and passes correctly.

But both tiers measure `runtime.MemStats.HeapInuse`, not RSS. The function comment is honest: "It's a closer-to-RSS approximation than HeapAlloc." HeapInuse excludes off-heap allocations (mmap'd buffers, cgo allocations from libmupdf, OS page cache mapped via the loader's reader). For this loader the gap is probably small, but for SC-009/SC-010 paths (cgo through fitz) it would understate dramatically. The spec says RSS — read `/proc/self/statm` or use `prometheus/procfs`/`golang.org/x/sys/unix`.

**Fix**: Read actual RSS on Linux via `/proc/self/statm` (multiply field 2 by page size). Document the platform-conditional behaviour. Worst case: keep HeapInuse as a leading indicator on darwin/windows but use real RSS on Linux runners.

---

### HIGH-5 — SC-011 e2e exercises non-TTY shape only; alt-screen footer contract is admitted-deferred

**File**: `tests/e2e/05_pipe.sh:7-22`
**Spec**: "Three pipeline shapes pass end-to-end ... displays Go-highlighted content with `<stdin>` in the footer"
**Test**: Script header is candid: "the interactive parts (alt-screen frame with `<stdin>` in the footer, scroll, `q` exit) need the PTY harness from T104; the scaffolding here exercises the non-TTY pipeline shape — the degenerate-cat contract." So the SC-011 *interactive* contract — the bit about the footer — is not verified anywhere.

**Fix**: Same root cause as CRITICAL-2/3 — needs the PTY harness. Until then, SC-011 is half-verified.

---

### MEDIUM-1 — Highlight corpus has 52 fixtures, 2 fail; SC-006 says 50/47

**File**: `tests/perf/highlight_corpus_test.go:124-153`
**Spec**: "≥ 47/50 files (94 %)"
**Test**: Corpus has 52 fixtures (verified `ls`). Run shows `sample.mm` and `sample.fish` failing extension match. Test scales the threshold (`wantPass = ceil(52*47/50) = 49`) and reports 50/52 passing. This is more permissive in absolute count (49 vs 47) and effectively the same percentage (94.2% vs 94%).

The scaling is documented and reasonable, but it means the spec promise "≥ 47/50" is not literally evaluated — the corpus has grown past 50 with two known-broken fixtures and the test silently accepts. Two failures on `sample.mm` (Objective-C++ extension Chroma doesn't recognise) and `sample.fish` (fish lexer absent) should be either renamed/excluded or fixed.

**Fix**: Decide whether `.mm` and `.fish` are part of the Linguist top-50 (they likely are not — pin the corpus to exactly 50 files matching the spec). If they're in, file an issue against Chroma or vendor the lexers.

---

### MEDIUM-2 — Percentile math is off-by-one for small n

**Files**:
- `tests/perf/firstframe_bench_test.go:80` (n=20 → idx=19 = max)
- `tests/perf/dismiss_bench_test.go:129` (PR-gate n=10 → idx=9 = max)
- `tests/perf/scroll_bench_test.go:83` (n=100 → idx=95, OK)
- `tests/perf/theme_swap_bench_test.go:141` (n=100 → idx=95, OK)
- `tests/integration/resize_test.go:134` (n=50 → idx=47, OK-ish)

**Issue**: `p95 := durations[(n*95)/100]` with integer division. For n=10, idx=9 picks the maximum; for n=20, idx=19 picks the maximum. For n=10 these "p95" values are p100, which is more conservative — fine for assertions, but mislabels the metric in the log line (`"SC-007: dismiss p95=…"` is actually max).

For n=100 and n=50 the formula is correct (or close enough by nearest-rank).

**Fix**: Either use `n=100`+ samples consistently, or compute `idx = int(math.Ceil(0.95*float64(n))) - 1` so the metric is correctly labelled regardless of sample size.

---

### MEDIUM-3 — Loader UpdatesBuffer concurrency observation hazard

**File**: All perf tests draining `stream.Updates` synchronously, e.g. `tests/perf/scroll_bench_test.go:44-46`
**Spec/Code**: `internal/loader/stream.go:99` defaults `UpdatesBuffer = 4`. The loader goroutine blocks on `case updates <- c:` (line 162) when the channel is full — the documented backpressure contract.

**Issue**: `for range stream.Updates {}` in the same goroutine drains as fast as the producer can send. For any test that wants to measure backpressure or simulate a slow consumer, this is wrong; for the *current* perf tests (which all wait for full drain *before* timing the action under test), it's a correctness non-issue but it does mean these benches will not catch a regression where the producer outruns the consumer by allocating intermediate buffers (since the tight drain loop hides the bound). The bench therefore observes an artificial best-case channel utilisation.

For SC-005 specifically (1 GiB load), the drain loop's tightness means the loader's 4-chunk × 256-line × 256-byte ≈ 256 KiB resident channel buffer never matters — but in production the user is the consumer (no goroutine), so the on-the-wire memory footprint differs.

**Fix**: Add a separate test that drains with a deliberate 1 ms tick between reads to verify backpressure stays bounded; the existing tests are fine for what they actually measure but should not be relied on to validate the bounded-channel contract.

---

### MEDIUM-4 — `make perf` invokes `-tags perf ./tests/perf/...` only

**File**: `Makefile:85-90`, `.github/workflows/nightly-perf.yml:36-41`
**Issue**: Nightly only sees files with `//go:build perf`. The "advisory" `TestThemeSwap_FullSpecCase` lacks that tag, so nightly never runs the 10 000-line shape. Also, the nightly does NOT run the resize integration test (which lives outside `tests/perf/`); SC-008 has no nightly guard.

**Fix**: Either add `//go:build perf` to `TestThemeSwap_FullSpecCase` and `TestResize_PreservesViewportAnchor` and let nightly re-run them on a clean runner, or extend the nightly to `-tags perf ./...`.

---

### MEDIUM-5 — `cgo` / build-tag isolation for SC-010

**File**: `internal/render/pdf_fitz_test.go` (lives behind `-tags fitz`)
**Issue**: The renderer-level fitz test correctly gates on the build tag (verified the run requires `-tags fitz`). However, the spec's home for SC-010 (`tests/integration/pdf_test.go`) is `t.Skip`'d regardless of tag — there is no `_fitz` integration variant. So under default tag, no integration coverage; under `-tags fitz`, still no integration coverage (the skip is unconditional).

**Fix**: When implementing the PTY harness, split into `pdf_fitz_test.go` (build-tag fitz, runs the rasterize half) and `pdf_nofitz_test.go` (default tag, runs the pdfcpu sentinel half). Mirrors the renderer-package layout already present.

---

### LOW-1 — `TestSearch_Under500ms` is a single-sample timer

**File**: `tests/perf/search_bench_test.go:58-73`
**Spec**: SC-003 doesn't ask for a percentile — single threshold is fine.
**Note**: One run = one wall-clock observation. CI noise on a shared runner could occasionally fail this; budget is 13× the observed time (37 ms vs 500 ms), so probably safe. If it ever flakes, run `n=5` and take the median.

---

### LOW-2 — Misleading `t.Logf` "p95" labels for n=10/n=20

See MEDIUM-2.

---

### LOW-3 — `osOpenAppend` shim in search_bench

**File**: `tests/perf/search_bench_test.go:23-25`
**Issue**: The "tiny shim so the helper above doesn't pull os into the import block" is dead reasoning — `os` is already imported in the same file (line 11). Trivial.

---

## Cross-cutting concerns

**1. Whole "deferred until T104 PTY harness" pattern.** Six tests under `tests/integration/` (footer, graphics, pdf, search, signal, stdin, theme, text_review) and one perf test (dismiss) are gated behind a hand-rolled PTY harness. The harness itself **does exist** — `tests/integration/pty.go` (314 lines, used by `dismiss_bench_test.go` and `pty_sanity_test.go`). So why are six other integration tests still skipped citing "T104 not done"? Either T104 *is* done and these need to be unskipped + implemented, or the harness is not yet trusted for those scenarios. Either way the t.Skip messages lie about the state of the harness.

**2. Race-flag exclusion via `//go:build !race`.** Every perf bench excludes the race detector. Defensible (race overhead distorts wall-clock) but means CI's `make test-race` never executes these gates. Combined with "PR gate runs only `make test-race` + `make cover`" (`ci.yml:43-47`), **none** of `tests/perf/*` runs in PR CI. So SC-001, SC-002, SC-003, SC-004 (small case), SC-005 (PR gate), SC-006, SC-007 (PR gate) only execute on developer laptops or when someone runs `make perf` manually.

The Makefile shows `make perf` runs only `-tags perf`, which means even invoking `make perf` does not pick up the default-tag perf tests. **The PR-gate tier as designed is not actually gated by any CI workflow.** This is the biggest honesty gap in the suite — the perf tests are built like gates but no automation runs them.

**Fix**: Add a CI step that runs `go test ./tests/perf/...` (no `-race`, no `-tags perf`) on every PR. Without this, regressions land silently.

---

## Recommendations (ranked)

1. **Wire the perf PR-tier into `.github/workflows/ci.yml`** — without this every other "PR gate" claim in this audit is moot. (Cross-cutting #2.)
2. **Unskip integration tests now that `pty.go` exists**, or update the skip messages to name the actual blocker.
3. **Land SC-009 + SC-010 PTY tests** or downgrade the SCs in spec.md to the renderer-level contract that's actually tested.
4. **Fix the dismiss-bench timer** (HIGH-3) — current measurement is structurally noisy.
5. **Assert "zero dropped frames" in SC-002** (HIGH-1) — one-line fix.
6. **Switch SC-005 to real RSS on Linux** (HIGH-4) — `/proc/self/statm`.
7. **Fix the percentile formula** to nearest-rank (MEDIUM-2) so log labels match what's measured.
8. **Pin the SC-006 corpus to exactly 50 files** matching the Linguist top-50; remove `.mm` and `.fish` or fix the lexers (MEDIUM-1).
9. **Add `//go:build perf` to the advisory full-spec-case theme-swap test** so nightly catches it (CRITICAL-1 / MEDIUM-4).

---

## Tests that look honest as-is

- `TestResize_PreservesViewportAnchor` (SC-008) — input matches spec, all three sub-conditions asserted, p95 calc is correct for n=50.
- `TestSearch_Under500ms` (SC-003) — input slightly above spec's 1 MiB, needle position forces full scan, asserts.
- `TestHighlightCorpus_LinguistTop50` (SC-006) — modulo MEDIUM-1, the per-fixture logic is sound and the threshold is enforced.
- `TestLargeFile_Nightly` (SC-005 nightly tier) — runs the right size with the right bound; the only quibble is HeapInuse vs RSS.
