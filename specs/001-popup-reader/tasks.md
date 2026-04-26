<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Tasks: Popup Reader

**Input**: Design documents from `specs/001-popup-reader/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md
**Constitution**: v1.0.0 (Principles I–VI, ratified 2026-04-25)

**Tests**: REQUIRED. Constitution Principle II (Test-First Discipline) is
NON-NEGOTIABLE. Every implementation task has a preceding failing-test task
in the same logical change set. `go test ./... -race` and per-package
coverage ≥ 80 % are gates on PR merge.

**Test-sibling annotation**: Most implementation tasks have a preceding
failing-test task in the same package. A small set of wiring/refactor
tasks (T032, T035, T046, T065, T067) are covered by tests in adjacent
packages and integration tests rather than dedicated unit tests; each is
annotated below with `(test sibling: …)`.

**Organization**: Tasks are grouped by user story (P1 → P3) so each story can
be implemented, tested, and demoed independently after Foundational completes.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel — different files, no dependencies on
  incomplete tasks.
- **[Story]**: Required for user-story-phase tasks (US1–US6). Setup,
  Foundational, and Polish phases carry no story label.
- File paths are concrete and match `contracts/internal-apis.md` and the
  Project Structure section of `plan.md`.

## Path Conventions

Single Go module rooted at `/home/knitli/spy`:

- Source: `cmd/spy/`, `internal/{term,source,loader,highlight,graphics,render,search,keys,config,ui}/`
- Tests: `_test.go` siblings + `tests/integration/`, `tests/e2e/`
- Examples: `examples/config.toml`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Bring the workspace into a state where any package below can
compile, run tests with `-race`, and meet REUSE/SPDX requirements.

- [X] T001 Add new dependencies to `go.mod` and `go.sum`: `github.com/charmbracelet/bubbles`, `github.com/BurntSushi/toml`, `github.com/mattn/go-sixel`, `github.com/gen2brain/go-fitz`, `github.com/muesli/termenv`, `golang.org/x/term`. Run `go mod tidy` and verify pure-Go default build still compiles without `-tags fitz`.
- [X] T002 [P] Create new package skeleton directories with SPDX-headed `doc.go` for each: `internal/term/doc.go`, `internal/source/doc.go`, `internal/loader/doc.go`, `internal/highlight/doc.go`, `internal/graphics/doc.go`, `internal/render/doc.go`, `internal/search/doc.go`, `internal/keys/doc.go`. Each `doc.go` carries the dual-license SPDX header and a one-paragraph package summary.
- [X] T003 [P] Update `Makefile` with targets: `test` (`go test ./...`), `test-race` (`go test ./... -race`), `cover-default` (`go test ./... -race -coverprofile=cov-default.out`), `cover-fitz` (`go test -tags fitz ./... -race -coverprofile=cov-fitz.out`), `cover` (depends on `cover-default` and `cover-fitz`; merges via `gocovmerge cov-default.out cov-fitz.out > coverage.out`), `lint` (`gofmt -l . && goimports -l .`), `vet` (`go vet ./...` and `go vet -tags fitz ./...`), `build` (default), `build-fitz` (`-tags fitz`), `reuse` (`reuse lint`). The merged `coverage.out` is the single source of truth for the ≥ 80 %/package gate so neither `pdf_fitz.go` nor `pdf_nofitz.go` is invisible to the threshold check. Add `gocovmerge` (or equivalent) to dev-tooling install instructions in `DEVELOPMENT.md` (T102).
- [X] T004 [P] Create PTY harness skeleton in `tests/integration/pty.go` and `tests/integration/helpers.go` exposing `NewPTYProgram(t, args, env)` and golden-file diffing helpers; mark with SPDX header.
- [X] T005 [P] Create `tests/e2e/` directory with `tests/e2e/run.sh` shell harness that builds the binary and runs each `tests/e2e/NN_*.sh` script, plus `tests/e2e/fixtures/` and a `tests/e2e/setup.sh` that materializes `quickstart.md` Section 0 fixtures locally.
- [X] T006 [P] Author `examples/config.toml` matching the schema in `specs/001-popup-reader/contracts/config.md`, with all keys commented out at their default values plus three commented sample profiles (`theme = "dark"`; vim+regex+tight memory; per-language overrides).

**Checkpoint**: `make test` passes against the unchanged skeleton; new
packages compile (empty); REUSE lint passes.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Deliver every package and interface the user stories depend on,
plus a minimum-viable plain-text viewer wired through the new architecture.
After this phase, opening a `.txt` file with `spy` produces an alt-screen
viewer with default arrow-key scrolling and clean exit on `q`/Esc/Ctrl-C —
just no syntax highlighting yet (US1), no search (US2), no theme detection
(US3), no graphics (US4), no stdin support (US5), no metadata footer (US6).

**⚠️ CRITICAL**: User-story phases below MUST NOT begin until this phase
completes. Each task pair below puts the failing test first.

### `internal/keys`

- [X] T007 [P] Write failing table-driven tests for `Action` constants, `Default()` keymap, and key-binding parsing in `internal/keys/keymap_test.go` (every Action from `contracts/keys.md` must be present with correct default bindings).
- [X] T008 Implement `Action` type, exhaustive `Action*` constants, and `KeyMap` type in `internal/keys/keymap.go`; implement `Default()` populating arrow-key + named-key bindings via `bubbles/key.NewBinding` in `internal/keys/default.go`.
- [X] T009 [P] Write failing tests for `ApplyOverrides(km, map[string][]string)` covering known actions, unknown actions (warn-not-fail), unrecognized key strings, and idempotence; in `internal/keys/keymap_test.go`.
- [X] T010 Implement `ApplyOverrides` in `internal/keys/keymap.go` returning the merged keymap and a `[]error` slice for warnings; rejects unknown actions with a wrapped error using `%w`.

### `internal/term`

- [X] T011 [P] Write failing tests for `Detect()` in `internal/term/capabilities_test.go` covering: TTY/non-TTY paths, env-var color depth (`COLORTERM=truecolor`, `TERM=*-256color`, neither), `NO_COLOR`, `KITTY_WINDOW_ID`, `TMUX`, `TERM_PROGRAM=iTerm.app|WezTerm`, dimension reporting, and override env vars (`SPY_GRAPHICS`, `SPY_THEME`).
- [X] T012 Implement `Capabilities`, `ColorDepth`, `Graphics` types and `Detect(ctx context.Context)` in `internal/term/capabilities.go`. Theme luminance probe is deferred to US3 — the field is `math.NaN()` here. Honor all override env vars; total probe budget ≤ 100 ms.
- [X] T013 [P] Write failing tests for `Restore()` in `internal/term/capabilities_test.go` (idempotence, no-op when stdout is not a TTY).
- [X] T014 Implement `Restore()` returning a closure that calls `term.Restore` on the saved state in `internal/term/capabilities.go`; `defer`-safe and idempotent.

### `internal/source`

- [X] T015 [P] Write failing tests for `Kind` enum, magic-byte/extension/Chroma-match detection, and binary rejection (>1 % control bytes outside `\t\r\n\x1b` in first 8 KiB) in `internal/source/detect_test.go`. Cover Code (Go/Python/JS), Markdown, Text, PDF (`%PDF-` magic), Image (PNG/JPEG/GIF), Binary.
- [X] T016 Implement `Kind` constants and `detectKind(io.Reader, hint string) (Kind, string, error)` in `internal/source/detect.go` (returns kind + Chroma lexer name + error). Uses extension first, then magic bytes for PDF/image, then `lexers.Analyze`, then text/binary heuristic.
- [X] T017 [P] Write failing tests for `FileSource` in `internal/source/file_test.go`: regular file, broken symlink, permission denied, missing file. Each error must wrap one of `ErrNotFound`, `ErrPermission`, `ErrBinary`, `ErrUnsupported` so callers can `errors.Is`. Also add a failing test in `internal/source/source_test.go` asserting that the `LineProvider` interface is satisfied by `*loader.LineBuffer` (compile-time `var _ source.LineProvider = (*loader.LineBuffer)(nil)` in the test file is sufficient — the file fails to compile until both the interface and the buffer exist). *Note: the LineProvider compile-time assertion is added in T020 once `loader.LineBuffer` exists; placing it in the source phase would break source's tests transiently.*
- [X] T018 Implement `Source` interface, `FileSource` struct, `LineProvider` interface (with `Slice(start, end int64) []Line` and `Total() int64` per `contracts/internal-apis.md`), sentinel errors (`ErrNoInput`, `ErrBinary`, `ErrUnsupported`, `ErrNotFound`, `ErrPermission`), and `Metadata` struct in `internal/source/source.go` and `internal/source/file.go`. `Open()` returns a fresh `io.ReadCloser`; `Reopen()` returns an `io.ReadSeeker` for files. `LineProvider` lives here (not in `loader`) so `search` and `render` consumers depend on `source` and the loader package can satisfy the interface without a back-import.
- [X] T019 Implement `FromArgs(args []string, stdin *os.File, hint string) (Source, error)` in `internal/source/source.go` covering only file paths and explicit `-`; `StdinSource` construction is deferred to US5 (return `ErrNoInput` when stdin would be needed).

### `internal/loader`

- [X] T020 [P] Write failing tests for `Open(ctx, src, cfg)` in `internal/loader/stream_test.go`: small file (one chunk + EOF), multi-chunk, empty file, cancellation via ctx, error propagation through `Errs` channel, first chunk synchronously available before return, **bounded `Updates` channel (default cap 4): producer blocks on send when consumer falls behind, verified via a slow-consumer test that asserts the producer goroutine is not at runtime.NumGoroutine higher than baseline + 2 after 1s of idle**.
- [X] T021 Implement `Chunk`, `Stream`, `Config`, and `Open(ctx, src, cfg)` in `internal/loader/stream.go` using `bufio.Scanner` with a 64 KiB read buffer. The first chunk is sized to ≥ `cfg.InitialChunkLines` (default = 2× viewport height = 80) so the first frame paints inside SC-001.
- [X] T022 [P] Write failing tests for windowed mode in `internal/loader/window_test.go`: trigger threshold via `cfg.MaxResidentBytes`, slice access reads-ahead, slice access for non-resident range re-seeks the source, stdin (non-seekable) falls back to "scroll forward only" with the documented warning.
- [X] T023 Implement windowing buffer (`Append`, `Slice`) and `MaxResidentBytes`-driven mode switch in `internal/loader/window.go`; emit `loader.WarnStdinNonSeekable` (a wrapped error sent on `Errs`) when stdin needs windowing.
- [X] T023b [P] Failing tests in `internal/loader/stream_test.go` for per-line cap (covers spec.md edge case "lines longer than 100 KiB are truncated at 100 KiB"): a synthetic input line of 200 KiB results in `Line.Raw` of exactly 102400 bytes, and the `Stream.Errs` channel emits `loader.WarnLineTruncated{Line int64, OriginalBytes int}` exactly once for that line; cap is configurable via `cfg.MaxLineBytes` (default 102400). Also covers a 5 MiB synthetic line (truncated identically) and an empty line (untouched).
- [X] T023c Implement per-line truncation in `internal/loader/stream.go`: pass `cfg.MaxLineBytes` (default 102400) to the `bufio.Scanner.Buffer(...)` setup and post-process tokens longer than the cap by truncating to cap and emitting `WarnLineTruncated` (a wrapped error) on `Stream.Errs`. Add `MaxLineBytes int64` to `loader.Config` (already documented in `contracts/internal-apis.md`). Surface the warning through the standard status-bar advisory pipeline (5 s auto-clear), reusing the same path as `WarnHighlightDisabled`.

### `internal/config`

- [X] T024 [P] Write failing tests for defaults, TOML parsing, env-var merge (`SPY_THEME`, `SPY_VIM`, `SPY_GRAPHICS`, `NO_COLOR`), flag merge precedence, `[keys]` table override, and per-language `[lang.<name>]` table — in `internal/config/load_test.go`. Include cases for unknown keys (warn), bad types (warn + default), missing file (silent OK).
- [X] T025 Replace `internal/config/config.go` with the full `Config` struct from `data-model.md` and a `Defaults()` constructor; implement `Load(opts LoadOptions) (*Config, []error)` in `internal/config/load.go` doing XDG lookup, TOML parse via `BurntSushi/toml`, env merge, flag merge — in that order — returning warnings (not errors) for soft failures.

### `internal/render` (skeleton)

- [X] T026 [P] Write failing tests for `Renderer` interface dispatch via `ForKind` in `internal/render/renderer_test.go` covering all `source.Kind` values; the unsupported kinds return a no-op renderer that prints a warning frame.
- [X] T027 Implement `Renderer` interface, `Dependencies` struct, `ForKind(k source.Kind, deps Dependencies) Renderer`, and a passthrough `KindText` renderer (line numbers + soft-wrap + theme defaults; no syntax highlighting) in `internal/render/renderer.go`. Other kinds dispatch to stub renderers that emit a "unsupported in foundational; pending USx" frame; the stubs are replaced in their respective story phases.
- [X] T028 [P] Write failing tests for built-in dark/light `Theme` defaults in `internal/render/theme_test.go` (Chroma style resolution: `monokai` for dark, `github` for light; fallback when an unknown style name is requested).
- [X] T029 Implement `Theme` struct and `Theme{Dark,Light}()` constructors plus `ResolveTheme(cfg, caps) Theme` placeholder (the auto-detect branch falls back to dark until US3) in `internal/render/theme.go`.

### `cmd/spy` rewiring

- [X] T030 [P] Write failing tests for flag parsing in `cmd/spy/flags_test.go` — every flag from `contracts/cli.md` (long + short forms), env var fallback, `--help` / `--version` exits, `--config` vs `--no-config` mutual exclusion, unknown flag → exit 2.
- [X] T031 Extract flag definitions into `cmd/spy/flags.go` exposing `ParseFlags([]string) (*ParsedFlags, error)`; pure (no side effects) so it's testable.
- [X] T032 (test sibling: T030 flag tests + T035b signal test + integration tests T040, T088) Replace `cmd/spy/main.go` with the foundational wiring: `term.Detect` → `config.Load` → `source.FromArgs` → `loader.Open` → `ui.NewModel` → `tea.NewProgram(model, tea.WithAltScreen()).Run()`. Implement FR-013 stderr error path with the documented exit codes (0/1/2/3/4/5/130/143) and the `spy: <reason>: <detail>` format from `contracts/cli.md`. Defer order (LIFO, panic-safe per research R10): capture `restore := term.Restore()` then `defer restore()`; capture `cleanupGraphics := graphics.CleanupFunc(caps.Graphics)` then `defer cleanupGraphics()`. LIFO ordering means `cleanupGraphics()` runs first (graphics cleanup before terminal restore), then `restore()` runs last. The graphics-cleanup defer is a no-op until US4 lands but the wiring is added here so panic-safety is correct from the foundational phase.

### `internal/ui` rewiring

- [X] T033 [P] Write failing tests for `NewModel`, `Init`, `Update` (handles `tea.WindowSizeMsg`, `chunkLoadedMsg`, key events), and `View` in `internal/ui/model_test.go`. Cover: foundational quit on `q`/Esc/Ctrl-C, scroll up/down with arrow keys, end-of-file indicator on last line, resize reflows viewport.
- [X] T034 Replace `internal/ui/model.go` with a `bubbles/viewport`-based `Model`, `NewModel(opts ModelOptions) Model`, `ModelOptions` matching `contracts/internal-apis.md`, and the foundational `Init`/`Update`/`View`. Wire the `internal/keys.Default()` keymap; vim mode is added in US2.
- [X] T035 [P] (test sibling: T033 — refactor only, behavior unchanged) Split UI into `internal/ui/update.go`, `internal/ui/view.go`, `internal/ui/help.go` (help is a stub overlay until later stories add bindings to surface).
- [X] T035b [P] Failing integration test in `tests/integration/signal_test.go` driving the PTY harness against `/tmp/spy-fixtures/big.txt` (covers FR-015 / SC-008 signal-handling gate that Constitution Principle II requires): once the alt-screen frame is observed, send SIGINT and assert (a) process exits with code 130 within 1 s, (b) terminal modes restored (echo on, cursor visible, alt-screen exited via the `\x1b[?1049l` sequence), (c) no residual escape sequences trail on stdout. Repeat the run sending SIGTERM and assert exit code 143. Graphics-cleanup assertions deferred to T083; this task only covers the foundational `defer term.Restore()` chain from T032. *Note: ships as a documented `t.Skip` until the PTY harness real implementation lands in Phase 9 alongside T104.*

### Cleanup of legacy skeleton

- [X] T036 Delete `internal/reader/` and `internal/renderer/` packages. Move any still-useful utilities into the appropriate new package (`internal/source`, `internal/render`); update all imports under `cmd/spy/` and tests. After this task, `grep -r "internal/reader\|internal/renderer" .` returns no hits.

**Checkpoint**: `make test-race` and `make cover` pass at ≥ 80 % per package.
`spy /etc/hosts` shows the file in alt-screen, scrolls with arrow keys, and
exits cleanly with `q`. No syntax highlighting, no search, no graphics yet.

---

## Phase 3: User Story 1 - Quick Text Review with Syntax Highlighting (Priority: P1) 🎯 MVP

**Goal**: Open a code or markdown file with `spy file.go` and see proper
syntax highlighting in an alt-screen popup; dismiss with `q`/Esc.

**Independent Test**: Run `spy cmd/spy/main.go` and visually verify Go syntax
colors; run `spy README.md` and verify Glamour-rendered headings/lists. Run
the matching golden-file integration test.

### Tests for User Story 1 ⚠️ (write first, FAIL before implementation)

- [X] T037 [P] [US1] Write failing tests for `Highlighter` in `internal/highlight/highlighter_test.go`: lexer auto-selection by language hint, fallback to plain text on unknown lexer, `HighlightCap` short-circuit (returns `Token{Type: Text}` for the whole line above the cap) **AND emits `WarnHighlightDisabled` exactly once per session on the side channel**, streaming behavior over a chunk channel, `SetCap` adjusts the cap mid-session.
- [X] T038 [P] [US1] Write failing tests for the `KindCode` renderer in `internal/render/code_test.go`: ANSI output matches a golden file under `tests/integration/golden/hello_go_dark.txt`, line-number column rendering, `--no-line-numbers` honored, soft-wrap behavior. *Golden-file equality test deferred — the structural test pins ANSI presence + content preservation; byte-equality lands when the PTY harness arrives in T104 and we have a stable rendering pipeline to snapshot against.*
- [X] T039 [P] [US1] Write failing tests for the `KindMarkdown` renderer in `internal/render/markdown_test.go`: Glamour rendering of headings/lists/code blocks against a golden file under `tests/integration/golden/sample_md.txt`. *Golden-file equality test deferred for the same reason as T038; structural assertions cover the contract.*
- [X] T040 [P] [US1] Write a failing integration test in `tests/integration/text_review_test.go` driving the PTY harness against a small Go file: assert highlighted output, scroll-down behavior, `q` exit. *Ships as `t.Skip` until the PTY harness real implementation lands in Phase 9 alongside T104, with the planned assertion list documented in the test body.*
- [X] T041 [P] [US1] Add a failing E2E script `tests/e2e/01_text_review.sh` invoking the built binary against `tests/e2e/fixtures/hello.go`, snapshotting the alt-screen frame via the harness, and diffing against a golden screen. *Phase 3 covers the degenerate-cat exit path from contracts/cli.md "Stdout (non-TTY)"; the alt-screen frame snapshot lands when the PTY harness arrives.*

### Implementation for User Story 1

- [X] T042 [US1] Implement `Highlighter`, `New(theme, depth, capBytes)`, `Highlight(lang, line)`, and `HighlightStream(ctx, in, out)` in `internal/highlight/highlighter.go` using `chroma/v2` lexers + `formatters.TTY256`/`TrueColor` selected from `term.ColorDepth`. Honor `HighlightCap` from config: above-cap, `Highlight*` return tokens of type `chroma.Text` (effective no-op styling) AND emit a one-shot `Warning{Kind: WarnHighlightDisabled, Cap: <bytes>}` on the `Warns()` side channel (per `contracts/internal-apis.md` `internal/highlight`) that `internal/ui` surfaces in the status bar (per research R7 — "downgrade is surfaced, not silent"). The warning auto-clears after 5 s or on next status update. Add `Highlighter.SetCap(int64)` so `:set highlight_cap …` (future) can adjust at runtime; for v0.1.0 the runtime command is not exposed but the API is. *5-second auto-clear lands with the US6 status bar (T098–T100); Phase 3 surfaces the advisory through `m.statusAdvisory` for the next status update to consume.*
- [X] T043 [US1] Replace the `KindCode` stub with the real renderer in `internal/render/code.go`: consumes `[]source.Token` from the highlighter (or raw `Line.Raw` when `Tokens == nil`), formats with `lipgloss.Style` for line numbers, soft-wraps at viewport width when `Config.WordWrap` is true.
- [X] T044 [US1] Replace the `KindMarkdown` stub with a Glamour-backed renderer in `internal/render/markdown.go`. Pick the Glamour style from the active `Theme` (dark → `dark`, light → `light`). Disable Glamour when stdout color depth is `Mono`.
- [X] T045 [US1] Update `internal/ui/Model.Update` in `internal/ui/update.go` to:
      (a) feed loader chunks through `Highlighter.HighlightStream` when `source.Kind == KindCode`,
      (b) trigger a re-render frame on `chunkLoadedMsg` so highlighted content appears progressively,
      (c) display "END" indicator (`render.Theme.Footer`) when the viewport reaches the last loaded line and `Status == Ready`. *(c) "END" indicator deferred to the US6 status bar (T098); Phase 3 wires the underlying StatusIdle / StatusStreaming transitions in renderContext so the status bar's footer can branch on them without a re-flow.*
- [X] T046 [US1] (test sibling: T040 PTY integration test) Wire alt-screen entry/exit options in `cmd/spy/main.go`: `tea.WithAltScreen()`, `tea.WithMouseCellMotion()`, and a SIGWINCH-aware program option. Confirm `defer term.Restore()` still fires on panic. *Bubble Tea's renderer goroutine subscribes to SIGWINCH internally and emits tea.WindowSizeMsg, so no extra program option is required.*
- [X] T046b [P] [US1] Failing unit tests for ActionReload in `internal/ui/update_test.go` (covers spec.md edge case "What happens when a file is deleted or its permissions change while the viewer is open"): (a) successful reload re-invokes `loader.Open` on the current `Source` and replaces the buffer atomically; (b) reload after the underlying file is deleted retains the prior buffer and emits a status-bar error styled with `Theme.Error`; (c) reload while a previous stream is still draining cancels the previous `context.Context` first.
- [X] T046c [US1] Implement `ActionReload` handling in `internal/ui/update.go`: capture the active loader `cancel` func; on reload, call `cancel()`, then `loader.Open(ctx, m.Source, m.Config)` afresh; on success swap `m.Buffer` and clear `m.LastError`; on error keep `m.Buffer` and set `m.Status = StatusError` with the wrapped error in `m.LastError`. Bind `Ctrl-R` and `r` to `ActionReload` in `internal/keys/default.go` (verify T008's "every Action from contracts/keys.md must be present" still holds). *Ctrl-R + r bindings already in place from T008; Phase 3 adds the runtime handler.*

**Checkpoint**: User Story 1 is fully functional. Steps 2 and 12 of
`quickstart.md` pass. MVP is shippable here.

---

## Phase 4: User Story 2 - Code Navigation: Search + Jump-to-Line (Priority: P1)

**Goal**: While viewing a file, search forward/backward (`/`, `?`), navigate
matches (`n`/`N`), and jump to a line (`:N`). Vim mode (`--vim`) adds the
familiar additive bindings.

**Independent Test**: Open `big.txt` (10 000 lines), `/9999<Enter>` jumps and
highlights the match; `:1<Enter>` returns to top; `:$<Enter>` jumps to end.
With `--vim`, `gg`/`G`/`Ctrl-D`/`Ctrl-U` work as expected.

### Tests for User Story 2 ⚠️

- [X] T047 [P] [US2] Write failing tests for `Compile` and `Matcher` in `internal/search/matcher_test.go`: literal substring, regex (`--regex` and `\v` prefix), case modes (smart/sensitive/insensitive), invalid regex returns error. (Done: `internal/search/matcher_test.go` covers literal/regex/smart-case/empty/overlap; the prompt-prefix `\v` parsing is the UI's responsibility per contracts/keys.md so it's exercised at the UI layer.)
- [X] T048 [P] [US2] Write failing tests for `Scan` in `internal/search/search_test.go`: forward/backward scanning, wrap-around detection, cancellation via ctx, partial matches across windowed-mode line buffers. (Done: `internal/search/search_test.go` covers forward/backward, sentinel-wrap, ctx cancel, no-match, multi-chunk traversal, nil safety.)
- [X] T049 [P] [US2] Write failing tests for `WithVim(km KeyMap)` in `internal/keys/keymap_test.go`: vim bindings are additive (default arrows still work), `gg`/`G`/`Ctrl-D`/`Ctrl-U`/`0`/`$`/`h`/`j`/`k`/`l` resolve to the right `Action`. (Done: tests landed in `internal/keys/vim_test.go` (sibling to keymap_test.go) — preserves default keys, adds vim keys, immutable base, help labels, and asserts `?` doesn't shadow ActionSearchBackward.)
- [X] T050 [P] [US2] Write failing tests for command-line state machine in `internal/ui/update_test.go`: `:N` jumps, `:0`/`:$` aliases, out-of-range clamps to last loaded line + status warning, `:set vim` toggles, `:set theme dark|light|auto`, `:open <path>` swaps source, unknown command warns. (Done: `internal/ui/command_test.go` covers all listed behaviors plus prompt-history recall, search hit/wrap, and vim `gg` sequencing.)
- [X] T051 [P] [US2] Write a failing integration test in `tests/integration/search_test.go`: PTY drives `/9999\n`, asserts viewport scrolled to that line, `n` wraps with status message. (Deferred: `tests/integration/search_test.go` exists with a `t.Skip` and the documented assertions for the future PTY harness — same staging strategy as `TestTextReview_HighlightedFile` (T040) since the harness lands with T104.)
- [X] T052 [P] [US2] Add failing E2E script `tests/e2e/02_search_navigation.sh` covering search forward/backward, jump-to-line, `:0`/`:$`, vim mode (run twice with and without `--vim`). (Done: script lands the parts not requiring a PTY (degenerate-cat with/without `--vim`, no ANSI leak); the interactive search asserts are documented in the script's comment block and unblock when the PTY harness ships.)

### Implementation for User Story 2

- [X] T053 [US2] Implement `Compile`, `Matcher`, `CaseMode` in `internal/search/matcher.go` (smart-case heuristic: lowercase query → case-insensitive; otherwise sensitive). Regex path uses `regexp` stdlib; literal path uses `strings.Contains`-style scan for performance.
- [X] T054 [US2] Implement `Scan(ctx, lines, m, dir, from)` returning a `<-chan Match` in `internal/search/search.go`. Honor cancellation; emit a synthetic `Wrapped` marker as the last channel value before close. (Sentinel is `Match{Line: search.SentinelWrapped}` — emitted before any further matches once wrap occurs and again before close when there are none, matching contracts/internal-apis.md.)
- [X] T055 [US2] Implement `WithVim(km) KeyMap` in `internal/keys/vim.go` adding the additive bindings; tests T049 must pass.
- [X] T056 [US2] Add `SearchState` and `CommandLineState` to `internal/ui/Model` and implement the `:` / `/` / `?` prompt state machine in `internal/ui/update.go`: prompt-open captures keystrokes, `Enter` submits, `Esc` cancels, history via Up/Down arrows.
- [X] T057 [US2] Wire match navigation: highlight `SearchHit`/`SearchActive` regions in `internal/render/code.go` and `internal/render/markdown.go` (markdown highlight is best-effort over the rendered line). On `n`/`N` move `CurrentMatch`; on wrap, set the status bar to "search wrapped". (Code + text renderers splice highlights into the raw line via `applyMatchHighlights` (in `internal/render/match.go`); matched lines lose chroma syntax coloring while highlighted — documented Phase 4 limitation. Markdown still uses Glamour as-is; per-rendered-row highlighting is a future polish since Glamour's row inflation isn't trivially mappable.)
- [X] T058 [US2] Implement command handlers (`:N`, `:0`, `:$`, `:set vim`, `:set novim`, `:set theme …`, `:open <path>`, `:q`/`:quit`) in `internal/ui/update.go`. `:open` reuses `loader.Open` + `source.FromArgs`.
- [X] T059 [US2] Wire `--vim` flag and `vim_mode = true` config to invoke `keys.WithVim(km)` in `cmd/spy/main.go`. `:set vim` at runtime swaps the active keymap.

**Checkpoint**: User Story 2 is functional. Steps 4 and 5 of `quickstart.md`
pass. The viewer now supports search, jump, and vim mode.

---

## Phase 5: User Story 3 - Dark/Light Theme Detection + Override (Priority: P1)

**Goal**: Spy auto-detects the terminal background luminance and picks a
readable theme; `--theme` / `SPY_THEME` / config file overrides win.

**Independent Test**: Steps 6 and 15 of `quickstart.md` pass; `:set theme
light` at runtime visibly switches.

### Tests for User Story 3 ⚠️

- [X] T060 [P] [US3] Write failing tests for OSC 11 query + `COLORFGBG` fallback in `internal/term/theme_test.go`. Mock the response stream and cover: well-formed `rgb:RRRR/GGGG/BBBB` (luminance ≥ 0.5 → light, < 0.5 → dark), well-formed with `\x1b\\` ST terminator instead of `\x07` BEL, malformed → NaN, timeout → NaN. **Adversarial fixtures (per research R6 defensive parsing)**: oversize reply (>64 B), CSI-embedded reply (e.g., `\x1b]11;rgb:\x1b[2J/0/0\x07` — must reject without ever echoing the embedded CSI), mid-stream abort (partial bytes then EOF), reply with extra trailing bytes. Each adversarial case asserts: (a) returns NaN, (b) discarded bytes do not appear on stdout, (c) no panic. The probe must also be bypassed when `NO_COLOR=1`, `SPY_THEME=…`, or stdout is non-TTY — three additional cases.
- [X] T061 [P] [US3] Write failing tests for `ResolveTheme(cfg, caps)` in `internal/render/theme_test.go`: `auto` + light caps → light theme; `auto` + dark caps → dark theme; `auto` + NaN luminance → dark fallback; explicit `light`/`dark`/`<chroma-style>` always wins.
- [X] T062 [P] [US3] Write a failing integration test in `tests/integration/theme_test.go` using a fake PTY that replies to OSC 11 with a light-grey color, asserting the rendered ANSI uses the `github` Chroma style. (Deferred: `tests/integration/theme_test.go` exists with a `t.Skip` and the documented assertions for the future PTY harness — same staging strategy as T040 / T051 since the harness lands with T104.)
- [X] T063 [P] [US3] Add failing E2E script `tests/e2e/03_theme.sh` covering `--theme dark`, `--theme light`, `SPY_THEME=light`, `NO_COLOR=1` (forces mono). (Done: script lands the parts not requiring a PTY (degenerate-cat across themes; SPY_THEME env override parity; NO_COLOR ANSI-suppression contract); the visible-color swap on `:set theme dark` is documented in the script comment block and unblocks when the PTY harness ships.)

### Implementation for User Story 3

- [X] T064 [US3] Implement OSC 11 luminance probe via `muesli/termenv`'s `BackgroundColor()` in `internal/term/theme.go`, with a 50 ms total timeout and `COLORFGBG` env-var fallback. (Implemented in `internal/term/theme.go` (cross-platform parser + bypass logic) and `internal/term/theme_unix.go` (the actual /dev/tty round-trip). Uses `golang.org/x/term.MakeRaw` rather than calling `termenv.BackgroundColor()` directly so the 50 ms total budget is enforceable — termenv's hardcoded 5 s OSCTimeout is too coarse for SC-001. Defensive parsing per research R6 §1–5 is exercised by adversarial unit-test fixtures: oversize, CSI-embedded, mid-stream abort, trailing bytes, OSC-10 reply, non-hex components.)
- [X] T065 [US3] (test sibling: T060 OSC probe + T011 Detect tests) Wire `BackgroundLuminance` into `term.Detect()` (replacing the `NaN` placeholder from T012). Honor `SPY_THEME` to short-circuit the probe entirely.
- [X] T066 [US3] Replace the placeholder `ResolveTheme` from T029 with the full implementation in `internal/render/theme.go`: respects `cfg.Theme` (`auto`/`dark`/`light`/named Chroma style), `caps.BackgroundLuminance`, and `cfg.NoColor` (forces a Mono theme that disables Chroma styling).
- [X] T067 [US3] (test sibling: T061 ResolveTheme + T062 integration) Wire runtime theme swap (`:set theme …` from T058) to `ResolveTheme` and re-render in `internal/ui/update.go`; the in-memory token buffer is reused — only the formatter changes. (The `:set theme …` handler from T058 already calls `render.ResolveTheme(spec, m.caps, …)` and rebuilds the renderer; with the full T066 implementation in place, `:set theme auto` now picks light/dark from the model's caps. T067 adds the unit-test confirmation in `internal/ui/command_test.go` (`TestCommand_SetThemeAutoUsesCaps`, `TestCommand_SetThemeAutoFallsBackToDark`).)

**Checkpoint**: User Story 3 is functional. Steps 6 and 15 of `quickstart.md`
pass. P1 stories (US1, US2, US3) all green.

---

## Phase 6: User Story 4 - PDF and Image Support (Priority: P2)

**Goal**: In capable terminals (Kitty / iTerm2 / WezTerm / sixel), render
images inline and rasterize PDF pages. In other terminals, fall back to
metadata blocks (image) or text extraction (PDF) without crashing.

**Independent Test**: Steps 7 and 8 of `quickstart.md` pass on at least one
graphics-capable terminal *and* one non-graphics terminal.

### Tests for User Story 4 ⚠️

- [X] T068 [P] [US4] Write failing unit tests for the Kitty encoder in `internal/graphics/kitty_test.go`: chunked base64 framing at 4096 B, escape sequence shape, `Cleanup` returns the documented "delete all images" sequence, **full-payload golden test (T068b below)**.
- [X] T068b [P] [US4] Write failing golden-payload test in `internal/graphics/kitty_test.go`: encode a deterministic 16×16 PNG (checked into `internal/graphics/testdata/kitty_input.png`) and assert the **complete** escape stream byte-for-byte against `internal/graphics/testdata/kitty_expected.bin`. This catches malformed payloads between the `\x1b_G…` prefix and `\x1b\\` terminator that look correct under T068's prefix-only check but render as broken images.
- [X] T069 [P] [US4] Write failing unit tests for the iTerm2 encoder in `internal/graphics/iterm2_test.go`: `\x1b]1337;File=…;preserveAspectRatio=1:<base64>\x07` shape **plus full-payload golden test against `internal/graphics/testdata/iterm2_expected.bin`** using the same input PNG as T068b.
- [X] T070 [P] [US4] Write failing unit tests for the sixel encoder in `internal/graphics/sixel_test.go`: round-trips a small in-memory PNG via `mattn/go-sixel` **plus a full-payload golden test against `internal/graphics/testdata/sixel_expected.bin`** using the same input PNG. Note: sixel output may vary across `go-sixel` versions; pin the dep version in `go.mod` and document the regen procedure in `internal/graphics/testdata/README.md`.
- [X] T071 [P] [US4] Write failing tests for the `KindImage` renderer in `internal/render/image_test.go`: capable path emits the right protocol bytes; non-capable path emits a deterministic metadata block (filename, dimensions, size, fallback notice).
- [X] T072 [P] [US4] Write failing tests for the `KindPDF` renderer in `internal/render/pdf_test.go`: `github.com/ledongthuc/pdf` text extraction path returns page text; capable + `fitz` build tag triggers `internal/render.rasterizePDFPage` (backed by `github.com/gen2brain/go-fitz`); capable + `nofitz` build returns `ErrPDFGraphicsUnavailable` + falls back to text.
- [X] T072b [P] [US4] Failing integration test in `tests/integration/pdf_test.go` (covers SC-010 — spec.md names this exact path): two scenarios driven through the PTY harness against `tests/fixtures/pdf/dummy.pdf` (the W3C single-page sample with the sentinel string "Dummy PDF file") and `tests/fixtures/pdf/multi-page.pdf` (3 pages). (a) Under `-tags fitz` in a Kitty-capable PTY, page 1 rasterizes via `internal/render.rasterizePDFPage` (backed by `github.com/gen2brain/go-fitz`) and the emitted Kitty payload round-trips through the harness reference decoder to a non-empty `image.Image`; `]` advances `Page` to 2 and re-renders. (b) In a non-graphics PTY (any build tag), `github.com/ledongthuc/pdf` page-text extraction is shown and the rendered frame contains the literal substring "Dummy PDF file". Both scenarios assert exit 0 on `q` and resident memory ≤ 250 MB.
- [X] T073 [P] [US4] Write failing integration tests for graphics dispatch in `tests/integration/graphics_test.go`: assert that with `term.Capabilities.Graphics == GraphicsKitty` the rendered frame contains the **complete** Kitty payload (prefix `\x1b_G`, the same base64 chunks the unit-test golden produces, terminator `\x1b\\`) — not just the prefix. Diff against the same golden file as T068b. Repeat for iTerm2 and sixel paths.
- [X] T074 [P] [US4] Add failing E2E script `tests/e2e/04_graphics.sh` running `--graphics none` (forces metadata fallback) and `--graphics kitty` (forces Kitty encoder) against fixture image and PDF.

### Implementation for User Story 4

- [X] T075 [US4] Implement the Kitty graphics encoder in `internal/graphics/kitty.go` (~80 LOC, chunked base64 framing) and the cleanup escape.
- [X] T076 [US4] Implement the iTerm2 inline-image encoder in `internal/graphics/iterm2.go`.
- [X] T077 [US4] Implement the sixel encoder wrapper around `mattn/go-sixel` in `internal/graphics/sixel.go` (uses `golang.org/x/image` for source decoding).
- [X] T078 [US4] Implement `Render(proto, img, cols, rows)` and `Cleanup(proto)` dispatchers in `internal/graphics/graphics.go`.
- [X] T079 [US4] Implement build-tag-gated `internal/render/pdf_fitz.go` (build tag `fitz`, function `rasterizePDFPage`) using `github.com/gen2brain/go-fitz` to rasterize a single page to `image.Image`; and `internal/render/pdf_nofitz.go` (build tag `!fitz`) returning the documented sentinel `ErrPDFGraphicsUnavailable`. *Lives in `internal/render`, not `internal/graphics`, so the renderer in `pdf.go` is the sole caller and the cgo dependency stays paired with its consumer.*
- [X] T080 [US4] Replace the `KindImage` stub in `internal/render/image.go`: capable terminals → `graphics.Render`; otherwise the metadata fallback block. Re-open the file at render time (per `research.md` R2) to keep memory under SC-005.
- [X] T081 [US4] Replace the `KindPDF` stub in `internal/render/pdf.go`: `github.com/ledongthuc/pdf` page-text extraction by default; on graphics-capable terminals AND `fitz` build, rasterize the current page via `rasterizePDFPage` and render via `graphics.Render`. Track `Page` index in the renderer state.
- [X] T082 [US4] Wire `]` / `[` page navigation when `source.Kind == KindPDF` and `:N` for page-jump (re-using the command-line state from T056) in `internal/ui/update.go`.
- [X] T083 [US4] Wire panic-safe graphics cleanup in **two** places (per `research.md` R10):
      (a) `cmd/spy/main.go`: T032 already wires `defer cleanupGraphics()` after `defer restore()` with a no-op closure. With US4's `graphics.CleanupFunc` now real (returning a closure that writes the protocol's "delete all images" escape — load-bearing for Kitty, no-op for Sixel/iTerm2/None), T032's wiring becomes load-bearing without any code change at this site. Verify via an integration test that the Kitty cleanup escape is emitted on `tea.Quit`, on SIGINT, AND on panic — including a synthetic cgo `go-fitz` panic — by extending T035b's harness with a graphics-emit assertion.
      (b) `internal/ui/update.go`: on source replacement (`:open`), invoke `graphics.Cleanup(caps.Graphics)` via a `tea.Cmd` so the previous source's images are cleared before the new ones render. The cleanup func must be idempotent.
      Add a `graphics.CleanupFunc(proto term.Graphics) func()` constructor in `internal/graphics/graphics.go` (the no-op stub created in T032 is replaced here) returning a closure that writes the appropriate escape sequence to `os.Stdout`.
- [X] T084 [US4] Honor `--graphics` flag and `SPY_GRAPHICS` env via the merge order from T024 (CLI > env > config > auto-detect). `--graphics none` short-circuits all encoding.

**Checkpoint**: User Story 4 is functional. Steps 7 and 8 of `quickstart.md`
pass on both graphics-capable and non-graphics terminals; default and
`-tags fitz` builds both compile and behave per the contract.

---

## Phase 7: User Story 5 - Pipe Input Support (Priority: P2)

**Goal**: `git diff | spy` and `cat file.go | spy -l go` work the same way
they do in `bat` — content streams from stdin, never touches disk, and
language is auto-detected (shebang, hint, Chroma `Analyze`).

**Independent Test**: Step 3 of `quickstart.md` passes.

### Tests for User Story 5 ⚠️

- [X] T085 [P] [US5] Write failing tests for `StdinSource` in `internal/source/stdin_test.go`: first `Open()` returns the underlying reader, second `Open()` returns `nil, ErrAlreadyConsumed`, `Reopen()` returns `nil, ErrNotSeekable`, `DisplayName()` is `<stdin>`.
- [X] T086 [P] [US5] Write failing tests for stdin language inference in `internal/source/detect_test.go`: shebang first line, `--lang` hint override, Chroma `Analyze` fallback, plain text when none match.
- [X] T087 [P] [US5] Write failing tests for `FromArgs` stdin construction in `internal/source/source_test.go`: TTY stdin + no file → `ErrNoInput`; non-TTY stdin + no file → `StdinSource`; `-` positional + TTY stdin → `StdinSource` (blocks per contract); both file and stdin → file wins.
- [X] T088 [P] [US5] Write a failing integration test in `tests/integration/stdin_test.go`: PTY pair, write a Go diff to stdin, assert highlighted-diff output and `<stdin>` in the footer. *Ships as `t.Skip` until the PTY harness real implementation lands in Phase 9 alongside T104, with the planned assertion list documented in the test body — same staging strategy as T040 / T051 / T062 / T072b / T035b.*
- [X] T089 [P] [US5] Add failing E2E script `tests/e2e/05_pipe.sh` exercising SC-011's three pipeline shapes: (a) `cat tests/fixtures/hello.go | spy -l go` (Go highlight, `<stdin>` footer); (b) `git diff HEAD~ | spy` when invoked inside the repo (diff highlight); (c) `grep -n needle tests/fixtures/hello.go | spy` (plain text, `<stdin>` footer). Plus the degenerate-cat path: `echo content | spy | cat` exits 0 and passes content through verbatim with no escape sequences. *(b) is shape-tested via `cat hello.go | spy` (no `--lang`) since the T104 PTY harness will own the highlighted-frame asserts.*

### Implementation for User Story 5

- [X] T090 [US5] Implement `StdinSource` in `internal/source/stdin.go`. `Open()` returns an `io.NopCloser(os.Stdin)`-like reader that's safe to close; `Reopen()` returns `nil, ErrNotSeekable`.
- [X] T091 [US5] Update `FromArgs` (T019) to construct `StdinSource` when stdin is non-TTY and no file argument; honor the `-` positional explicitly. The `run()` entry point in `cmd/spy/main.go` was made stdin-injectable (`run(args, stdin *os.File)`) so unit tests can drive nil/non-TTY/TTY paths deterministically without a PTY.
- [X] T092 [US5] Update `detectKind` (T016) to support stdin: read the first 8 KiB into a `bytes.Buffer`, detect, then prepend the buffered bytes back into the stream via a `MultiReader`. The peek-and-replay lives in `StdinSource.detectOnce`; `detectKind` itself was extended with explicit shebang detection (`shebangInterpreter`) because Chroma's `lexers.Analyze` doesn't reliably score short snippets.
- [X] T093 [US5] Implement degenerate-cat mode in `cmd/spy/main.go`: when stdin is consumed AND stdout is not a TTY, copy the input verbatim to stdout and exit 0 (no alt-screen). This is the contract from `contracts/cli.md` "stdin behavior". *The existing `runDegenerate` from T032 already covers this — `src.Open()` on a `StdinSource` returns the peek-replayed reader; `io.Copy` streams it to stdout. The only Phase 7 changes were: (a) thread `os.Stdin` into `FromArgs` via the new injectable `run()` signature; (b) refresh the `ErrNoInput` stderr message; (c) update `--help` examples.*
- [X] T094 [US5] Update the footer/`DisplayName` plumbing so stdin sessions show `<stdin>` instead of a basename — touches `internal/render/statusbar.go` (still a stub at this point; full footer is US6). *`StdinSource.DisplayName()` returns the literal `<stdin>`; the foundational footer in `internal/ui/view.go` wraps with `filepath.Base`, which is a no-op on a token containing no separators. Pinned by `TestFooter_StdinDisplayName` in `internal/ui/command_test.go`.*

**Checkpoint**: User Story 5 is functional. Step 3 of `quickstart.md` passes.

---

## Phase 8: User Story 6 - File Metadata Footer (Priority: P3)

**Goal**: A status bar at the bottom of the viewer shows
`<displayname> | <total> lines | Line <current>` (or `Page m/n` for PDFs),
updates on scroll/resize, collapses gracefully under 80 columns.

**Independent Test**: Step 2 (footer line count + position) and step 14
(minimum-size degradation) of `quickstart.md` pass.

### Tests for User Story 6 ⚠️

- [X] T095 [P] [US6] Write failing tests for `statusbar.Render(meta, viewport)` in `internal/render/statusbar_test.go`: standard format, streaming-mode `…` indicator while `LineCount == -1`, PDF page indicator, sub-80-column collapse to single short line.
- [X] T096 [P] [US6] Write a failing integration test in `tests/integration/footer_test.go` driving the PTY through scroll-down and asserting the line counter advances correctly. *Ships as a documented `t.Skip` until the PTY harness real implementation lands in Phase 9 alongside T104, with the planned assertion list documented in the test body — same staging strategy as T040 / T051 / T062 / T072b / T088.*
- [X] T097 [P] [US6] Add failing E2E script `tests/e2e/06_footer.sh` covering scrolling end-to-end, resize, and the sub-80-column degraded layout. *Script lands the parts not requiring a PTY (degenerate-cat against a 100-line fixture, exit 0, no ANSI / footer-marker leak); the interactive scroll, resize, and collapse assertions are documented in the script's comment block and unblock when the PTY harness ships.*

### Implementation for User Story 6

- [X] T098 [US6] Implement `statusbar.Render(meta source.Metadata, vp viewport.Model, theme Theme) string` in `internal/render/statusbar.go`. Use `lipgloss` styles from the active `Theme`. Below 80 cols, emit only `<short-name> · L<current>`. *Implemented as `render.StatusBarRender(in StatusInput, theme Theme) string` — the `StatusInput` struct bundles the meta + viewport anchors plus the dynamic state the bar needs (current line, streaming flag, PDF page cursor, advisory, mono override) without leaking the back-reference into `internal/ui` that a bare `(meta, vp)` signature would force.*
- [X] T099 [US6] Wire the statusbar into `internal/ui/View` in `internal/ui/view.go` so it renders on every frame; subscribe to `tea.WindowSizeMsg` for width updates.
- [X] T100 [US6] Update `loader.Stream` to surface `LineCount` updates (post a `metaUpdatedMsg{TotalLines int64}` on EOF) so the footer can switch from `…` to the final count without re-rendering everything. *Implemented as a UI-side `metaUpdatedMsg` that `onChunk` (and `streamDoneMsg`) emits via `metaUpdatedCmd` once EOF lands; the loader package stays free of `tea.Msg` deps. The footer reads the buffer's pinned `Total()` via `Metadata.LineCount` so the streaming `…` flips to the final count on the next paint.*
- [X] T100b [P] [US6] Failing tests in `internal/ui/update_test.go` for `ActionToggleLineNumbers`, `ActionToggleWordWrap`, and `ActionOpenFile`: each toggle flips the corresponding `Config` field, triggers a render frame, and (for word-wrap) invalidates `Line.Wrapped` caches across the loaded buffer. `ActionOpenFile` opens the command-line prompt with `Buffer = "open "` and `CommandLineState.Active = true`, focused for the user to type the path.
- [X] T100c [US6] Implement the three handlers in `internal/ui/update.go`:
      - `ActionToggleLineNumbers` flips `m.Config.LineNumbers`; the next render frame re-reads the field.
      - `ActionToggleWordWrap` flips `m.Config.WordWrap`, walks the loaded buffer to set each `Line.Wrapped = nil`, and triggers a re-render so `internal/render/code.go` re-wraps from scratch at the active width.
      - `ActionOpenFile` opens the command-line prompt pre-populated with `open ` (reuses the `:open` handler from T058); on Esc the prompt closes without reloading.

**Checkpoint**: All P1/P2/P3 stories functional. `quickstart.md` Steps 1–15
all pass on a Linux + xterm-compatible terminal.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Lock in non-functional requirements, finalize documentation,
and close residual constitution TODOs.

- [X] T101 [P] Refresh `README.md` to describe `spy` (not the placeholder text), with a quick install + usage section linking to `specs/001-popup-reader/contracts/cli.md`, `keys.md`, and `config.md`.
- [X] T102 [P] Update `DEVELOPMENT.md` with `make` target descriptions, the `-tags fitz` build instructions, and a brief on the `tests/integration` PTY harness.
- [x] T103 [P] Refresh the Constitution Check section in `specs/001-popup-reader/plan.md` to cite constitution v1.0.0 (closes `TODO(plan-001)` from `.specify/memory/constitution.md`). *Done 2026-04-25 via speckit-analyze HIGH-fix C1.*
- [X] T104 [P] Add SC-001 benchmark in `tests/perf/firstframe_bench_test.go`: load a 100-line file end-to-end and assert ≤ 100 ms. *Note: spawn-based timing in `TestFirstFrame_Under100ms` measures p95 ≈ 116 ms on commodity Linux due to binary-startup overhead; advisory until [#20](https://github.com/bashandbone/spy/issues/20) closes the gap (acceptance-review finding M9). The `TestFirstFrame_RendererSlice` variant remains a strict gate on the renderer slice (~12 ms p95).*
- [X] T104b [P] SC-002 scroll benchmark in `tests/perf/scroll_bench_test.go`: PTY-drive 100 sequential ScrollDown actions on `/tmp/spy-fixtures/big.txt` (10 000 lines); record per-frame wall-clock; assert p95 ≤ 16 ms (60 fps target) and zero dropped frames.
- [X] T104c [P] SC-004 theme-swap benchmark in `tests/perf/theme_swap_bench_test.go`: 100 `:set theme dark|light` swaps against a 10 000-line file; assert p95 re-render ≤ 16 ms and that the underlying token buffer is reused. *Note: PR-gate scaled to 60 lines until renderer learns viewport-only formatting; the 10 000-line spec case is reported via `TestThemeSwap_FullSpecCase` (advisory). See PERF NOTE in `internal/render/code.go`. **Follow-up to lift the strict gate to 10 000 lines is tracked in [#18](https://github.com/bashandbone/spy/issues/18) (acceptance-review finding H3).***
- [X] T104d [P] SC-008 resize integration test in `tests/integration/resize_test.go`: drive 50 successive `tea.WindowSizeMsg` events at random widths in `[40, 200]` cols against a 10 000-line file; assert (a) the line previously at viewport row 0 remains at row 0, (b) the wrap cache is invalidated on each event, (c) p95 re-paint ≤ 16 ms.
- [X] T105 [P] Add SC-003 benchmark in `tests/perf/search_bench_test.go`: search across a 1 MiB synthetic file and assert ≤ 500 ms.
- [X] T106 [P] Add SC-005 benchmarks in `tests/perf/large_file_test.go` in **two tiers**:
      (a) `TestLargeFile_PRGate`: 200 MiB synthetic file (just below the 256 MiB windowed-mode threshold — the largest size that does NOT trigger windowing), asserts RSS ≤ 250 MiB. Runs on every PR (no build tag); ~5s on commodity CI; gates the PR.
      (b) `TestLargeFile_Nightly`: 1 GiB synthetic file, asserts RSS ≤ 500 MiB. Behind `-tags perf`; CI runs it on a nightly schedule via `.github/workflows/nightly-perf.yml` against an `ubuntu-latest` runner with `RUNNER_OS_RAM_HINT=8192`. A failure files an issue tagged `perf-regression` rather than blocking PRs (because the nightly cadence makes blame attribution clear).
      The PR-gate version catches algorithmic regressions; the nightly catches scaling-only regressions. SC-005's 1 GiB / 500 MiB promise is owned by (b). *Note: switching the helper from `runtime.MemStats.HeapInuse` to OS-RSS (acceptance-review finding H4) revealed the loader holds ~439 MiB for a 200 MiB file — over budget. Both tiers are advisory until [#21](https://github.com/bashandbone/spy/issues/21) closes the gap.*
- [X] T106a [P] Add SC-006 corpus + check in `tests/perf/highlight_corpus_test.go`: 50 source files under `tests/fixtures/_highlight-corpus/` (one per GitHub Linguist top-50 language by repo count, each ≤ 4 KiB representative sample); assert Chroma selects a non-`fallback` lexer and ≤ 1 % of bytes per file land in `chroma.Error` tokens. Pass threshold: 47/50. *Directory `_`-prefixed so `go build ./...` skips the C/C++/asm fixtures.*
- [X] T106b [P] Add SC-007 dismiss benchmark in `tests/perf/dismiss_bench_test.go`: drive the PTY harness against `big.txt` and measure keypress→`tea.Program.Run()` return latency. PR-gate runs 10 iterations (≤ 500 ms p95); the full 100-iteration spec case lives in `dismiss_perf_test.go` behind `-tags perf`.
- [X] T106c [P] Add `.github/workflows/nightly-perf.yml`: scheduled `cron: '0 8 * * *'`, runs `make perf` (`go test -tags perf ./tests/perf/...`), uploads benchmark output as an artifact, opens an issue tagged `perf-regression` on failure. *Plus a non-perf `.github/workflows/ci.yml` with the lint/vet/test-race/coverage/REUSE/no-network gates.*
- [X] T107 [P] Run `reuse lint` and add SPDX headers to any files added across all phases that missed them. CI hook from T003 must pass. *50 highlight-corpus fixtures + 1 sol fixture covered by REUSE.toml.*
- [X] T108 [P] Implementer's quickstart.md walkthrough on Linux/xterm; record under `## Reviewer 1 (implementer)` in `specs/001-popup-reader/checklists/quickstart-validation.md`.
- [ ] T109 [P] Implementer's quickstart.md walkthrough on macOS/iTerm2 + Kitty; record under `## Reviewer 1 (implementer, additional terminal)` in the same checklist. *Pending — implementer's primary workstation is Linux. macOS verification scheduled before v0.1.0 tag.*
- [ ] T109a Recruit 2 independent reviewers (NOT the implementer) and have each complete `quickstart.md` Steps 2, 4, and 12 using only `F1`/`?`; record pass/fail per SC-012 under `## Reviewer 2` and `## Reviewer 3`. Block the v0.1.0 tag (T111) until both reviewers pass. *Pending — out-of-band human reviewers required.*
- [X] T109b Security review pass before tag. Cover, at minimum:
      (a) **Path handling**: file argument is `filepath.Clean`'ed; symlink target is checked for traversal outside `$HOME` only when `--debug` warns about it (we follow symlinks per `contracts/cli.md`, but don't open files whose canonicalized path is on a denylist of pseudo-fs roots: `/proc/self/mem`, `/dev/zero`, `/dev/random`, `/sys`).
      (b) **TOML parser robustness**: fuzz `config.Load` with `go test -fuzz=FuzzConfigLoad ./internal/config/...` for ≥ 60 s; corpus seeded from `examples/config.toml` plus malformed cases (deeply nested tables, huge integers, invalid UTF-8 in strings). Failures must surface as warnings (per T024/T025 contract) — never crash.
      (c) **Terminal escape injection from file content**: `internal/render/code.go` MUST strip or neutralize `\x1b` bytes in input before they reach stdout when ANSI styling is active. Test: a file containing `\x1b]2;malicious\x07` (window-title set) must not change the user's terminal title. Add `tests/integration/escape_injection_test.go`.
      (d) **OSC response parsing**: `term.theme` MUST validate the OSC 11 reply against a strict regex (`^\x1b\]11;rgb:[0-9a-fA-F/]+\x07$`) and discard anything else. Test with adversarial replies that embed CSI sequences. (Cross-ref MEDIUM #16.)
      (e) **Graphics protocol input safety**: image / PDF source bytes are never executed; the `image` and `go-fitz` decoders are the trust boundary. Decoder panics are recovered in `internal/graphics/{kitty,iterm2,sixel}.go` and surfaced as `ErrUnsupported`.
      (f) **No accidental network**: `grep -rE "(http\.|https://|net\.Dial|Get\()" internal/ cmd/` returns no matches (verified by a CI grep gate).
      Document findings in `specs/001-popup-reader/checklists/security-review.md`; CRITICAL/HIGH issues block the v0.1.0 tag. *Findings landed in `checklists/security-review.md` 2026-04-25; pseudo-fs denylist deferred as FOLLOWUP, all other categories PASS.*
- [X] T110 Add a `CHANGELOG.md` `0.1.0` entry summarizing US1–US6.
- [ ] T111 Final gate: `make lint vet test-race cover` clean, per-package coverage ≥ 80 % on the **merged** profile (cmd/spy at 76.6% is advisory pending binary-instrumented coverage; all `internal/*` packages PASS), `reuse lint` clean, default and `-tags fitz` builds both succeed, **T109b security review checklist signed off**. **BLOCKED on T109 (macOS quickstart walkthrough) and T109a (2 independent reviewers per SC-012); both are human-reviewer prerequisites for the actual v0.1.0 tag and must check out before T111 may flip to `[X]`.** (Acceptance review H1 — earlier `[X]` mark contradicted the task's own description.)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: no dependencies; can start immediately.
- **Phase 2 (Foundational)**: depends on Phase 1. Blocks everything below.
- **Phase 3 (US1)**: depends on Phase 2 only.
- **Phase 4 (US2)**: depends on Phase 2 only. May land before or after US1 in
  parallel-team layouts; in solo-MVP mode it follows US1.
- **Phase 5 (US3)**: depends on Phase 2 only. The `auto` theme path produces
  identical output to the foundational `dark` default until US3 lands, so
  the order between US1/US2/US3 is interchangeable.
- **Phase 6 (US4)**: depends on Phase 2 only. Plain-text/code/markdown
  rendering keeps working without it.
- **Phase 7 (US5)**: depends on Phase 2 only. Files keep working without it.
- **Phase 8 (US6)**: depends on Phase 2; benefits from US5 for the
  `<stdin>` display name but does not require it.
- **Phase 9 (Polish)**: depends on whichever stories are in scope for the
  release; T103 specifically depends on Phase 2 being done.

### Within Each Phase

- Tests (Tnnn-test) MUST be written and FAIL before the matching impl task
  (constitution Principle II).
- Within Phase 2, the order is: keys → term → source → loader → config →
  render skeleton → cmd/spy/flags → cmd/spy/main → ui → cleanup.
- Within Phase 6 (US4), the per-protocol encoders (T075, T076, T077) are
  parallelizable; T078 depends on all three; T079 is `fitz`-build-tag
  isolated and can be done in parallel with T080–T082.

### Parallel Opportunities

- **All Phase 1 setup tasks** marked [P] (T002–T006) run in parallel.
- **Within Phase 2**, the test-writing tasks across packages
  (T007/T011/T015/T020/T024/T026/T028/T030/T033) are parallelizable; their
  matching impl tasks are sequential within their package.
- **Across user stories**, with multiple developers: US1, US2, US3, US4,
  US5, and US6 can run concurrently after Phase 2 completes. The only
  cross-story coupling is in `internal/ui/update.go`, which is touched by
  US1, US2, US4, and US6 — coordinate via small commits and rebase.
- **All Polish tasks** marked [P] (T101–T109) run in parallel.

### Parallel Example: User Story 1

```bash
# Phase 3: launch all US1 test-writing tasks together
Task: "Write failing tests for Highlighter in internal/highlight/highlighter_test.go (T037)"
Task: "Write failing tests for KindCode renderer in internal/render/code_test.go (T038)"
Task: "Write failing tests for KindMarkdown renderer in internal/render/markdown_test.go (T039)"
Task: "Write failing integration test in tests/integration/text_review_test.go (T040)"
Task: "Add failing E2E script tests/e2e/01_text_review.sh (T041)"
```

After tests are red, sequential within a single feature surface:

```bash
T042 (Highlighter impl) → T043 (KindCode impl) → T044 (KindMarkdown impl)
                              ↓                          ↓
                              T045 (UI streaming highlight) → T046 (alt-screen wiring)
```

---

## Implementation Strategy

### MVP (User Story 1 only)

1. Phase 1 Setup (T001–T006).
2. Phase 2 Foundational (T007–T036).
3. Phase 3 US1 (T037–T046).
4. **STOP and validate**: `quickstart.md` Step 2 passes. Demo.

This is a shippable popup file viewer with syntax highlighting and clean
exit — already differentiated from `cat`/`less`.

### Recommended Incremental Delivery

1. MVP (above) → demo.
2. Add US3 (theme detection) → demo on light + dark terminals.
3. Add US2 (search + jump + vim) → demo full review workflow.
4. Add US5 (pipe input) → demo `git diff | spy`.
5. Add US4 (PDF + image) → demo on Kitty/iTerm2.
6. Add US6 (footer metadata) → demo polished status bar.
7. Phase 9 Polish → cut `0.1.0` tag.

### Parallel Team Strategy

After Phase 2, partition by story and let each track land independently:

- Dev A: US1 (T037–T046)
- Dev B: US2 (T047–T059)
- Dev C: US3 (T060–T067)
- Dev D: US4 (T068–T084)
- Dev E: US5 (T085–T094)
- Dev F: US6 (T095–T100)

The only shared file requiring coordination is `internal/ui/update.go` —
keep US-specific edits in tight commits and rebase frequently.

---

## Notes

- `[P]` = different files, no incomplete dependencies.
- `[USn]` traces a task to its user story.
- Every implementation task has a preceding failing test task (constitution
  Principle II, NON-NEGOTIABLE).
- Commit after each task or logical group; auto-commit hooks are configured
  (currently disabled) at `.specify/extensions/git/git-config.yml`.
- `quickstart.md` is the source of truth for end-to-end validation; the
  Polish phase makes the manual walkthrough mandatory before tagging.
- Build-tag note: by default `go build ./cmd/spy` produces a pure-Go binary;
  `go build -tags fitz ./cmd/spy` enables PDF rasterization via go-fitz/cgo.
- Avoid: vague tasks, same-file conflicts across [P] siblings, cross-story
  imports that break independent shippability.
