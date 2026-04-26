<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos
SPDX-License-Identifier: MIT OR Apache-2.0
-->

# spy `001-popup-reader` Acceptance-Review Traceability

**Repo**: `/home/knitli/spy` · **Branch at audit**: `main` (merged) · **Date**: 2026-04-26
**Sources audited**: `specs/001-popup-reader/{spec,plan,research,data-model,quickstart,tasks}.md`,
`contracts/{cli,keys,config,internal-apis}.md`,
`checklists/{quickstart-validation,security-review}.md`, `cmd/spy/`, `internal/*`,
`tests/{integration,perf,e2e}/`.

> Headline: every functional requirement has shipping code; every measurable
> SC has a test file; **but** every PTY-driven user-story integration test
> (T035b/T040/T051/T062/T072b/T073/T083(a)/T088/T096) is still
> `t.Skip("PTY harness not yet implemented — Phase 9 T104 will provide the
> runtime")` even though the harness IS implemented in
> `tests/integration/pty.go:39-296` and is exercised by
> `tests/integration/pty_sanity_test.go` and
> `tests/perf/dismiss_bench_test.go`. This is the dominant gap.

---

## 1. FR coverage table

| FR | Requirement | Code location | Test location | Status | Notes |
|----|-------------|---------------|---------------|--------|-------|
| FR-001 | Accept FILE arg | `cmd/spy/main.go:125`, `internal/source/source.go:132` (`FromArgs`) | `internal/source/source_test.go`, `cmd/spy/flags_test.go` | MET | |
| FR-002 | Accept piped stdin | `cmd/spy/main.go:125`, `internal/source/stdin.go`, `internal/source/source.go:155-171` | `internal/source/stdin_test.go`, `tests/e2e/05_pipe.sh:61-72` | MET | Live alt-screen frame coverage gated on PTY (T088 skipped). |
| FR-003 | Syntax highlighting by ext / inferred | `internal/highlight/highlighter.go`, `internal/render/code.go`, `internal/source/detect.go` | `internal/highlight/highlighter_test.go`, `tests/perf/highlight_corpus_test.go` (SC-006) | MET | |
| FR-004 | Dark/light themes adapt to terminal | `internal/render/theme.go:52-78`, `internal/term/theme.go` (OSC 11) | `internal/term/theme_test.go`, `internal/render/theme_test.go` | MET | Live OSC 11 round-trip in PTY deferred (T062). |
| FR-005 | Arrow-key nav + optional vim | `internal/keys/keymap.go`, `internal/keys/vim.go` | `internal/keys/keymap_test.go`, `internal/keys/vim_test.go` | MET | |
| FR-006 | `:N` jump-to-line | `internal/ui/update.go` (`:` handler), `internal/ui/command_test.go` | `internal/ui/command_test.go` | MET | |
| FR-007 | `/` and `?` search with `n`/`N` | `internal/search/{matcher,search}.go`, `internal/ui/update.go` | `internal/search/{matcher,search}_test.go`, `internal/ui/command_test.go` | MET | PTY end-to-end deferred (T051). |
| FR-008 | `q`/`Esc` dismissal w/o terminal corruption | `cmd/spy/main.go:75-77` (`defer restore`), `internal/ui/update.go` (quit) | `internal/ui/update_test.go`, `tests/integration/pty_sanity_test.go` | PARTIAL | `pty_sanity_test.go:51-57` shows first `q` keystroke doesn't always trigger exit, falls back to `Ctrl-C`; `dismiss_bench_test.go:99-113` has the same workaround. Real bug or PTY race never investigated. |
| FR-009 | Footer with name / line count / position | `internal/render/statusbar.go`, `internal/ui/view.go` | `internal/render/statusbar_test.go` | MET | Visible PTY assertion deferred (T096). |
| FR-010 | Inline image rendering on capable terminals | `internal/graphics/{kitty,iterm2,sixel}.go`, `internal/render/image.go` | `internal/graphics/*_test.go` (golden-byte), `internal/render/image_test.go` | MET | PTY frame golden-diff deferred (T073). |
| FR-011 | PDF preview w/ graceful fallback | `internal/render/pdf.go`, `internal/render/pdf_fitz.go`, `internal/render/pdf_nofitz.go` | `internal/render/pdf_test.go` | MET | Uses `ledongthuc/pdf` (NOT `pdfcpu` as contract states — see drift §4). |
| FR-012 | Progressive load → 256 MiB → windowed → 1 GiB cap | `internal/loader/{stream,window}.go` | `internal/loader/window_test.go`, `tests/perf/large_file_{test,perf_test}.go` | MET | `TestLargeFile_Nightly` (1 GiB / 500 MB) is `-tags perf` only — see SC-005. |
| FR-013 | Errors → stderr, exit codes per `contracts/cli.md` | `cmd/spy/main.go:282-318` (`exitForSourceError`) | `cmd/spy/main_test.go`, `cmd/spy/flags_test.go` | MET | |
| FR-014 | Resize/reflow | `internal/ui/update.go` (WindowSizeMsg), `internal/loader/window.go:114` (ClearWrapCaches) | `tests/integration/resize_test.go`, `internal/ui/update_test.go` | PARTIAL | **`resize_test.go` p95 budget FAILS at 23.3 ms vs 16 ms in this environment** (audit run 2026-04-26). See §3 SC-008. |
| FR-015 | Clean exit on SIGINT/SIGTERM | `cmd/spy/main.go:75-77`, `internal/graphics/graphics.go:63` (`CleanupFunc`) | `tests/integration/signal_test.go` (T035b) | GAP | `TestSIGINTRestoresTerminal` and `TestSIGTERMRestoresTerminal` are STILL `t.Skip` even though the harness is real. No real assertion exists for terminal restoration on signal. |

---

## 2. US acceptance scenarios coverage table

| US | Scenario | Code location | Test location | Status | Notes |
|----|----------|---------------|---------------|--------|-------|
| US1 #1 | `spy hello.go` paints `<100 ms` w/ Go highlight | `cmd/spy/main.go:43-216`, `internal/render/code.go` | `tests/perf/firstframe_bench_test.go` (SC-001) | MET | Test exercises the model not the binary; "from invocation" includes Go startup that's outside renderer scope. |
| US1 #2 | `↓`/`j`/`PgDn`/`Home`/`End` scroll | `internal/ui/update.go`, `internal/keys/default.go` | `internal/ui/update_test.go` | MET | |
| US1 #3 | `q`/`Esc`/`Ctrl-C` exit cleanly with proper code | `cmd/spy/main.go:75-77` | `tests/integration/signal_test.go` (skipped) | GAP | See FR-015. Code 130 / 143 path has no live assertion. |
| US1 #4 | END indicator past last line | `internal/render/statusbar.go`, `internal/ui/view.go` | `internal/render/statusbar_test.go` | MET | |
| US2 #1 | Lexer auto-selection vs fallback | `internal/highlight/highlighter.go`, `internal/source/detect.go` | `tests/perf/highlight_corpus_test.go` (SC-006) | MET | |
| US2 #2 | `:42` jumps; `:0`/`:$` aliases; clamp + warn | `internal/ui/update.go` (`:` handler) | `internal/ui/command_test.go` | MET | |
| US2 #3 | `/`/`?` highlight active vs other matches; footer count | `internal/render/match.go`, `internal/ui/update.go` | `internal/ui/command_test.go`, `internal/search/*_test.go` | MET | Markdown match-overlay is best-effort (documented in T057). |
| US2 #4 | `n`/`N` wrap; status bar; zero-match no-op | `internal/ui/update.go`, `internal/search/search.go:250` (sentinel) | `internal/ui/command_test.go`, `internal/search/search_test.go` | MET | PTY end-to-end deferred (T051). |
| US3 #1 | OSC 11 luminance ≥ 0.5 → light/`github` | `internal/term/theme.go`, `internal/render/theme.go:111-` | `internal/term/theme_test.go`, `internal/render/theme_test.go` | MET | |
| US3 #2 | Luminance < 0.5 OR no reply → dark/`monokai` | same | same | MET | |
| US3 #3 | Flag > env > config > auto precedence | `cmd/spy/main.go:166-169`, `internal/config/load.go` | `internal/config/load_test.go`, `cmd/spy/main_test.go` | MET | PTY-driven precedence sweep deferred (T062). |
| US4 #1 | PDF rasterizes ≥80% viewport in Kitty/iTerm2/WezTerm | `internal/render/pdf.go`, `internal/render/pdf_fitz.go`, `internal/graphics/{kitty,iterm2}.go` | `internal/render/pdf_test.go`, `internal/graphics/*_test.go` | PARTIAL | Per-protocol golden-byte unit tests pass; the human-eye legibility step is unmeasured (the spec puts it on the SC-012 reviewer panel which hasn't run). |
| US4 #2 | Image displays inline ≥80% viewport in capable terminals | `internal/render/image.go`, `internal/graphics/*` | same | PARTIAL | Same — visual legibility gate hasn't run (SC-012 pending). |
| US4 #3 | Non-capable: metadata block; PDF page-1 text | `internal/render/image.go:25-`, `internal/render/pdf.go` (text path) | `internal/render/{image,pdf}_test.go` | MET | |
| US5 #1 | Pipe shows content; footer `<stdin>`; line count `…` until EOF | `internal/source/stdin.go`, `internal/render/statusbar.go` | `internal/source/stdin_test.go`, `internal/ui/command_test.go::TestFooter_StdinDisplayName` | MET | PTY frame deferred (T088). |
| US5 #2 | Lang detection: `--lang` > shebang > Chroma > plain | `internal/source/detect.go` (`shebangInterpreter`), `internal/source/stdin.go` | `internal/source/detect_test.go` | MET | |
| US5 #3 | Stdin never persisted to disk | `internal/source/stdin.go` (memory only), `cmd/spy/main.go:222-233` (`runDegenerate`) | `internal/source/stdin_test.go`, `tests/e2e/05_pipe.sh` | MET | Spec promises `tests/integration/stdin_test.go` snapshots tmp dirs pre/post — the actual integration test is `t.Skip`'d, so the explicit dir-snapshot guarantee is not enforced. |
| US6 #1 | Footer shows file name and total line count | `internal/render/statusbar.go` | `internal/render/statusbar_test.go` | MET | |
| US6 #2 | Footer updates as user navigates | `internal/ui/update.go` + `internal/render/statusbar.go` | `internal/render/statusbar_test.go`, deferred `tests/integration/footer_test.go` (T096) | PARTIAL | PTY-driven scroll-counter test deferred. |

---

## 3. SC measurable outcomes

| SC | Threshold | Test file | Real or advisory? | Measured value |
|----|-----------|-----------|-------------------|----------------|
| SC-001 | First frame ≤ 100 ms (100-line file) | `tests/perf/firstframe_bench_test.go:39` | Real — `t.Fatalf` on miss | Passes; logs `p95` per run |
| SC-002 | Scroll p95 ≤ 16 ms over 100 ScrollDown on 10 000-line file, **zero dropped frames** | `tests/perf/scroll_bench_test.go:37` | Real — `t.Fatalf` on miss | Passes (in-process model, not the PTY harness the spec mandates — the test admits this on lines 32-36) |
| SC-003 | Search ≤ 500 ms on 1 MiB file | `tests/perf/search_bench_test.go:32` | Real — `t.Fatalf` | Passes |
| SC-004 | Theme swap p95 ≤ 16 ms over 100 swaps on 10 000-line file | `tests/perf/theme_swap_bench_test.go:63` (PR gate, **60 lines**) and `:78` (full spec, advisory) | **PARTIAL — PR gate scaled to 60 lines (1/166 of spec); full-spec is advisory `failOnBudget=false`** | Audit-run measured **`p95 = 813 ms` at 10 000 lines** (50× over budget). `theme_swap_bench_test.go:55-62` documents the architectural gap (renderer formats every resident line per paint). |
| SC-005 | ≤ 1 GB file, RSS ≤ 500 MB | PR gate: `tests/perf/large_file_test.go` (200 MiB / 250 MiB); nightly: `large_file_perf_test.go` (`-tags perf`, 1 GiB / 500 MiB) | Real but **the 1 GiB / 500 MB spec gate runs only in the nightly workflow** (`.github/workflows/nightly-perf.yml`) and a failure files an issue rather than blocking PRs (`tasks.md` T106 documents this) | PR gate passes; nightly not run during this audit |
| SC-006 | 50-file Linguist corpus, ≥ 47/50 pass, ≤ 1 % `chroma.Error` bytes | `tests/perf/highlight_corpus_test.go:37` | Real — `t.Fatalf` on miss | Passes |
| SC-007 | Dismiss p95 ≤ 500 ms over 100 invocations on `big.txt` | PR gate: `tests/perf/dismiss_bench_test.go:27` (10 iterations); spec case: `dismiss_perf_test.go` (`-tags perf`, 100 iterations) | Real, but PR-gate measurement floors at **200 ms** (the `WaitForExit` polling tick on the first-keystroke fast path — `dismiss_bench_test.go:101-107`), so the budget is enforced as "≤ 500 ms" only when the first `q` keystroke is dropped (which happens regularly in the sanity tests). When the first `q` does propagate, "elapsed" is credited at the polling-tick upper bound, not the real latency. | Passes the budget, but the measurement methodology is loose. |
| SC-008 | Resize: row-0 line stable; wrap cache invalidated; next paint p95 ≤ 16 ms over 50 events | `tests/integration/resize_test.go:47` | Real — `t.Fatalf` | **FAILS in audit env: `p95 = 23.34 ms` exceeds 16 ms budget**. (Anchor and wrap-invalidation parts pass.) |
| SC-009 | Image fixtures emit Kitty payload, exit 0, RSS ≤ 250 MB | `internal/graphics/*_test.go` (per-protocol golden bytes); the dispatch-through-renderer assertion is in `tests/integration/graphics_test.go::TestGraphics_KittyPayloadDispatch` | **`TestGraphics_KittyPayloadDispatch` is `t.Skip`'d** — the round-trip-through-renderer assertion called for by SC-009 never runs. The unit-test goldens cover the encoder in isolation only. | n/a (skip) |
| SC-010 | PDF: page rasterized w/ fitz; pdfcpu text extracted in non-graphics PTY | `internal/render/pdf*.go` (uses `ledongthuc/pdf`, **NOT pdfcpu**); `tests/integration/pdf_test.go` (T072b) | **`TestPDF_GraphicsAndTextFallback` is `t.Skip`'d**. Unit test `internal/render/pdf_test.go` exercises text-extraction path against `dummy.pdf` (passes), but the SC-010 PTY scenarios (graphics+text round-trip, RSS ceiling, page navigation) never run. | n/a (skip) |
| SC-011 | Three pipeline shapes pass in `tests/e2e/05_pipe.sh` | `tests/e2e/05_pipe.sh` | Real but **the script tests degenerate-cat shapes only** (script lines 6-23 acknowledge interactive PTY parts ship with T104). The required `git diff HEAD~ \| spy` shape (b) is replaced with `cat hello.go \| spy` (script line 72) "since the T104 PTY harness will own the highlighted-frame asserts". The footer-`<stdin>` and exit-on-`q` assertions don't run. | n/a for the alt-screen path |
| SC-012 | 3 reviewers pass quickstart Steps 2/4/12 using only `F1`/`?` | `specs/001-popup-reader/checklists/quickstart-validation.md` | Manual checklist | **2 of 3 reviewers PENDING.** Implementer Linux/xterm passed; macOS/iTerm2+Kitty entry says "PENDING"; Reviewers 2 & 3 (independent) all "—" with `Status: PENDING`. Tasks.md says "Block the v0.1.0 tag (T111) until both reviewers pass" but T111 is marked `[X]` regardless. |

---

## 4. Cross-reference vs `contracts/internal-apis.md`

Spot-check of each documented signature against the actual code.

| Contract | Code | Match? |
|---------|------|--------|
| `term.Detect(ctx context.Context) Capabilities` | `internal/term/capabilities.go:67` | YES |
| `term.Restore() func()` | `internal/term/capabilities.go:191` | YES |
| `term.Capabilities` fields incl. `BackgroundLuminance float64`, `InTmux bool` | `internal/term/capabilities.go:39-47` | YES |
| `source.LineProvider { Slice(start, end int64) []Line; Total() int64 }` | `internal/source/source.go:109-112` | YES; compile-time assertion at `internal/loader/window_test.go:158` (`var _ source.LineProvider = (*LineBuffer)(nil)`) |
| `source.FromArgs(args []string, stdin *os.File, hint string) (Source, error)` | `internal/source/source.go:132` | YES |
| `source.Line { Number, Raw, Tokens, Wrapped }` | `internal/source/source.go:89-94` | YES |
| Sentinel errors `ErrNoInput, ErrBinary, ErrUnsupported, ErrNotFound, ErrPermission, ErrNotSeekable, ErrAlreadyConsumed` | `internal/source/source.go:46-57` | YES + extra `ErrAmbiguousArgs` (additive — fine) |
| `loader.Open(ctx, src, cfg) (*Stream, error)` | `internal/loader/stream.go:89` | YES |
| `loader.Stream { First, Updates, Errs }` | `internal/loader/stream.go:65-70` | DRIFT — additional `Buffer *LineBuffer` field. Additive, but not in contract. Production code (`internal/render/renderer.go:39`) and tests rely on it. |
| `loader.Config { MaxResidentBytes, WindowSize, InitialChunkLines, UpdatesBuffer, MaxLineBytes }` | `internal/loader/stream.go:28-34` | YES |
| `highlight.New(theme *chroma.Style, depth term.ColorDepth, capBytes int64) *Highlighter` | `internal/highlight/highlighter.go:70` | YES (param `style` instead of `theme` — same semantics) |
| `highlight.Highlight(lang, line string) []source.Token` | `internal/highlight/highlighter.go:147` | YES |
| `highlight.HighlightStream(ctx, in, out)` | `internal/highlight/highlighter.go:185` | YES |
| `highlight.SetCap(int64)` and `Warns() <-chan Warning` | `internal/highlight/highlighter.go:107,131` | YES |
| `graphics.Renderer { Render(img, cols, rows) (string, error); Cleanup() string }` | `internal/graphics/graphics.go:23-26` | YES |
| `graphics.RendererFor(proto) Renderer` | `internal/graphics/graphics.go:31` | YES |
| `graphics.Render(proto, img, cols, rows) (string, error)` | `internal/graphics/graphics.go:46` | YES |
| `graphics.Cleanup(proto) string` | `internal/graphics/graphics.go:53` | YES |
| `graphics.CleanupFunc(proto) func()` | `internal/graphics/graphics.go:63` | YES |
| `graphics.PDFPage(path string, n int, dpi float64) (image.Image, error)` | **MISSING from `internal/graphics`.** The actual rasterization lives at `internal/render/pdf_fitz.go:26` as unexported `rasterizePDFPage(src source.Source, page int) (image.Image, error)` (no DPI arg, no public `PDFPage` symbol, wrong package). | DRIFT |
| `render.Renderer { Render(ctx RenderContext) string }` | `internal/render/renderer.go:52-62` | DRIFT — interface has TWO methods: `Render` AND `RowToLine(ctx, visualRow int) int64`. The contract describes one. Additive, but the docs missed it. |
| `render.RenderContext { Buffer, Viewport, Theme, Capabilities, Search, Status, LastError, Page }` | `internal/render/renderer.go:38-47` | YES |
| `render.Dependencies { Theme, Capabilities, Graphics, Highlighter }` | `internal/render/renderer.go:67-91` | DRIFT — adds `LineNumbers bool`, `WordWrap bool`, `Language string`, `Source source.Source`. Additive but unmentioned. |
| `search.State` fields `Wrapped`, `Pending`, `CurrentMatch` etc. | `internal/search/types.go:37-46` | YES |
| `search.Compile / Matcher / Scan / Direction / CaseMode` | `internal/search/{matcher,search,types}.go` | YES |
| `keys.Action / KeyMap / Default / WithVim / ApplyOverrides` | `internal/keys/keymap.go:16,66,…` + `vim.go` | YES |
| `ui.ModelOptions { Source, Stream, Capabilities, Config, Theme, KeyMap }` | `internal/ui/model.go:30-54` | DRIFT — adds `BaseKeyMap keys.KeyMap`, `Highlighter *highlight.Highlighter`, `Cancel context.CancelFunc`. Required by main.go to hook the runtime; contract should be updated. |

The drifts above are mostly additive (no behavioral break against the spec),
but the `graphics.PDFPage` and `render.Renderer.RowToLine` cases mean an
external implementer reading the contracts in isolation would fail to
compile.

---

## 5. Gap list (ranked, actionable)

### CRITICAL

1. **All P1/P2/P3 user-story PTY integration tests are still `t.Skip`** with the
   reason "PTY harness not yet implemented — Phase 9 T104 will provide the
   runtime", **but the harness is real** (`tests/integration/pty.go:39-296`,
   exercised by `pty_sanity_test.go` + `dismiss_bench_test.go`). Affected:
   `text_review_test.go:33` (T040, US1),
   `search_test.go:39` (T051, US2),
   `theme_test.go:51` (T062, US3),
   `pdf_test.go:43` (T072b, US4 / SC-010),
   `graphics_test.go:36,49` (T073/T083(a), US4 / SC-009),
   `stdin_test.go:39` (T088, US5),
   `footer_test.go:41` (T096, US6),
   `signal_test.go:33,39` (T035b, FR-015 / SC-008 signal gate).
   Net effect: **SC-009 / SC-010 / SC-012 do not have any PTY-driven
   enforcement**, and FR-015 (clean exit on signal) has no live coverage at
   all. Tasks.md marks all of these `[X]`.

2. **SC-008 PR-gate test is currently failing** in this environment.
   `tests/integration/resize_test.go:137` measured `p95 = 23.34 ms` vs the
   16 ms budget across 50 resize events. The spec calls this a hard gate; CI
   either passes by luck on the runner hardware or the failure is being
   tolerated.

3. **`pty_sanity_test.go:51-57` and `dismiss_bench_test.go:99-113` both work
   around a real bug**: the first `q` keystroke after first paint is
   sometimes dropped, requiring a fallback to `Ctrl-C` or a retransmit. The
   benches credit the iteration with the polling tick (200 ms) when it
   propagates, masking the actual latency. This is undiagnosed and means
   FR-008 ("dismissal via 'q' or 'ESC'") has a known, unaddressed flake.

### HIGH

4. **SC-004 PR gate is scaled to 60 lines** (1/166 of spec's 10 000-line case);
   the actual 10 000-line case is reported as advisory non-failing.
   `theme_swap_bench_test.go:78` `TestThemeSwap_FullSpecCase` measured
   **813 ms p95** in the audit run — 50× over the 16 ms budget. The
   architectural gap (renderer formats every resident line per paint) is
   acknowledged in `theme_swap_bench_test.go:55-62` and `internal/render/code.go`
   PERF NOTE, but no follow-up issue / task is filed and tasks.md still
   marks T104c `[X]`.

5. **SC-011 is not actually tested** end-to-end. `tests/e2e/05_pipe.sh:6-23`
   acknowledges the interactive parts ship with T104; the required
   `git diff HEAD~ | spy` shape (b) is replaced by a verbatim-copy assertion
   on `cat hello.go | spy`. The `<stdin>` footer assertion and exit-on-`q`
   assertion never run.

6. **SC-005 1 GiB / 500 MiB gate runs only in the nightly workflow**
   (`.github/workflows/nightly-perf.yml`, `-tags perf`), and a failure files
   an issue rather than blocking the PR. The PR gate covers 200 MiB / 250 MiB
   only — sufficient to catch algorithmic regressions but not the spec's
   actual scale promise.

7. **T109 (macOS/iTerm2+Kitty walkthrough) and T109a (independent reviewers)
   are NOT done.** `quickstart-validation.md` shows all macOS rows as
   `PENDING` and Reviewer 2 / Reviewer 3 cells as `—`. Tasks.md T111 (final
   gate) is marked `[X]` despite explicitly saying "T109 and T109a remain as
   separate human-reviewer blockers for the actual v0.1.0 tag". So the
   "tagged at SC-012 verified" promise is unmet — and there is no functional
   SC-012 enforcement (3-reviewer panel never assembled).

### MEDIUM

8. **`graphics.PDFPage` symbol is missing from `internal/graphics`.** The
   `internal-apis.md` contract specifies
   `func PDFPage(path string, n int, dpi float64) (image.Image, error)` in
   `internal/graphics`, but the actual rasterizer is unexported
   `rasterizePDFPage(src source.Source, page int) (image.Image, error)` in
   `internal/render/pdf_fitz.go:26`. Different package, different signature,
   no DPI parameter, not callable from outside `internal/render`.

9. **`render.Renderer` interface has an undocumented second method `RowToLine`.**
   Contract describes `Render(ctx) string` only; code at
   `internal/render/renderer.go:52-62` requires both. Every renderer
   implementation provides it; the docs don't.

10. **`render.Dependencies` and `ui.ModelOptions` both add fields the contract
    doesn't list.** Dependencies adds `LineNumbers / WordWrap / Language /
    Source`; ModelOptions adds `BaseKeyMap / Highlighter / Cancel`. Cleanly
    additive but should be reflected in `internal-apis.md`.

11. **`loader.Stream` adds a public `Buffer *LineBuffer` field** not in the
    contract. Production code depends on it (passed into `RenderContext`,
    consumed by `search.Scan`).

12. **PDF text extraction uses `ledongthuc/pdf`, not `pdfcpu`** as both
    `tasks.md` (T072b, T079) and `spec.md` SC-010 explicitly state. Real
    code: `internal/render/pdf.go:14`. This is a substantive library
    substitution that should at minimum update the spec/tasks language;
    licensing and behavior parity should also be confirmed.

13. **Security review claims `filepath.Clean`** is applied to the file
    argument (`security-review.md:24-25`), but the code at
    `internal/source/source.go:194` only calls `filepath.EvalSymlinks`. The
    review's statement is inaccurate; `EvalSymlinks` does normalize `..` for
    existing files, so the security property holds, but the doc says
    something the code doesn't do.

14. **Pseudo-fs denylist deferred as FOLLOWUP** (`security-review.md:22-41`),
    so `spy /proc/self/mem` would still be attempted (failing only at the
    `read` syscall). Documented; not blocking by the implementer's own
    rubric, but noted.

### LOW

15. **`tasks.md` T108 metadata says the implementer's walkthrough was on
    "WSL2 (kernel 6.6.87.2)" Linux/xterm**, which matches the audit env.
    No issue, just noting consistency.

16. **`tests/e2e/05_pipe.sh` (g) test "stdin redirected from a TTY" is
    explicitly skipped** ("impossible to test in this shell-based harness").
    The unit test `cmd/spy/main_test.go::TestRun_StdinTTYWithoutFileExitsUsage`
    covers the exit-2 path via a nil stdin — sufficient, but worth flagging
    that the live TTY path is unreached.

---

## What "merged" means for this audit

`git status` is clean; the branch is `main`. Every implementation task is
marked `[X]` in `tasks.md` except T109 and T109a. The most consequential gap
is that the deferred PTY tests for the user-story integration assertions
were never lifted from `t.Skip`'d documentation into runnable tests, despite
the PTY harness those tests depend on having shipped (in fact, in the same
phase). The harness is being used by sanity and perf tests, so the skips
are stale rather than load-bearing — lifting them is a mechanical pass that
should fail-then-fix where the workarounds in `pty_sanity_test.go` and
`dismiss_bench_test.go` have already discovered (item #3).
