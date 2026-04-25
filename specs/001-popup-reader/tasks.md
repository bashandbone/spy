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

- [ ] T001 Add new dependencies to `go.mod` and `go.sum`: `github.com/charmbracelet/bubbles`, `github.com/BurntSushi/toml`, `github.com/mattn/go-sixel`, `github.com/gen2brain/go-fitz`, `github.com/muesli/termenv`, `golang.org/x/term`. Run `go mod tidy` and verify pure-Go default build still compiles without `-tags fitz`.
- [ ] T002 [P] Create new package skeleton directories with SPDX-headed `doc.go` for each: `internal/term/doc.go`, `internal/source/doc.go`, `internal/loader/doc.go`, `internal/highlight/doc.go`, `internal/graphics/doc.go`, `internal/render/doc.go`, `internal/search/doc.go`, `internal/keys/doc.go`. Each `doc.go` carries the dual-license SPDX header and a one-paragraph package summary.
- [ ] T003 [P] Update `Makefile` with targets: `test` (`go test ./...`), `test-race` (`go test ./... -race`), `cover-default` (`go test ./... -race -coverprofile=cov-default.out`), `cover-fitz` (`go test -tags fitz ./... -race -coverprofile=cov-fitz.out`), `cover` (depends on `cover-default` and `cover-fitz`; merges via `gocovmerge cov-default.out cov-fitz.out > coverage.out`), `lint` (`gofmt -l . && goimports -l .`), `vet` (`go vet ./...` and `go vet -tags fitz ./...`), `build` (default), `build-fitz` (`-tags fitz`), `reuse` (`reuse lint`). The merged `coverage.out` is the single source of truth for the ≥ 80 %/package gate so neither `pdf_fitz.go` nor `pdf_nofitz.go` is invisible to the threshold check. Add `gocovmerge` (or equivalent) to dev-tooling install instructions in `DEVELOPMENT.md` (T102).
- [ ] T004 [P] Create PTY harness skeleton in `tests/integration/pty.go` and `tests/integration/helpers.go` exposing `NewPTYProgram(t, args, env)` and golden-file diffing helpers; mark with SPDX header.
- [ ] T005 [P] Create `tests/e2e/` directory with `tests/e2e/run.sh` shell harness that builds the binary and runs each `tests/e2e/NN_*.sh` script, plus `tests/e2e/fixtures/` and a `tests/e2e/setup.sh` that materializes `quickstart.md` Section 0 fixtures locally.
- [ ] T006 [P] Author `examples/config.toml` matching the schema in `specs/001-popup-reader/contracts/config.md`, with all keys commented out at their default values plus three commented sample profiles (`theme = "dark"`; vim+regex+tight memory; per-language overrides).

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

- [ ] T007 [P] Write failing table-driven tests for `Action` constants, `Default()` keymap, and key-binding parsing in `internal/keys/keymap_test.go` (every Action from `contracts/keys.md` must be present with correct default bindings).
- [ ] T008 Implement `Action` type, exhaustive `Action*` constants, and `KeyMap` type in `internal/keys/keymap.go`; implement `Default()` populating arrow-key + named-key bindings via `bubbles/key.NewBinding` in `internal/keys/default.go`.
- [ ] T009 [P] Write failing tests for `ApplyOverrides(km, map[string][]string)` covering known actions, unknown actions (warn-not-fail), unrecognised key strings, and idempotence; in `internal/keys/keymap_test.go`.
- [ ] T010 Implement `ApplyOverrides` in `internal/keys/keymap.go` returning the merged keymap and a `[]error` slice for warnings; rejects unknown actions with a wrapped error using `%w`.

### `internal/term`

- [ ] T011 [P] Write failing tests for `Detect()` in `internal/term/capabilities_test.go` covering: TTY/non-TTY paths, env-var color depth (`COLORTERM=truecolor`, `TERM=*-256color`, neither), `NO_COLOR`, `KITTY_WINDOW_ID`, `TMUX`, `TERM_PROGRAM=iTerm.app|WezTerm`, dimension reporting, and override env vars (`SPY_GRAPHICS`, `SPY_THEME`).
- [ ] T012 Implement `Capabilities`, `ColorDepth`, `Graphics` types and `Detect(ctx context.Context)` in `internal/term/capabilities.go`. Theme luminance probe is deferred to US3 — the field is `math.NaN()` here. Honour all override env vars; total probe budget ≤ 100 ms.
- [ ] T013 [P] Write failing tests for `Restore()` in `internal/term/capabilities_test.go` (idempotence, no-op when stdout is not a TTY).
- [ ] T014 Implement `Restore()` returning a closure that calls `term.Restore` on the saved state in `internal/term/capabilities.go`; `defer`-safe and idempotent.

### `internal/source`

- [ ] T015 [P] Write failing tests for `Kind` enum, magic-byte/extension/Chroma-match detection, and binary rejection (>1 % control bytes outside `\t\r\n\x1b` in first 8 KiB) in `internal/source/detect_test.go`. Cover Code (Go/Python/JS), Markdown, Text, PDF (`%PDF-` magic), Image (PNG/JPEG/GIF), Binary.
- [ ] T016 Implement `Kind` constants and `detectKind(io.Reader, hint string) (Kind, string, error)` in `internal/source/detect.go` (returns kind + Chroma lexer name + error). Uses extension first, then magic bytes for PDF/image, then `lexers.Analyse`, then text/binary heuristic.
- [ ] T017 [P] Write failing tests for `FileSource` in `internal/source/file_test.go`: regular file, broken symlink, permission denied, missing file. Each error must wrap one of `ErrNotFound`, `ErrPermission`, `ErrBinary`, `ErrUnsupported` so callers can `errors.Is`.
- [ ] T018 Implement `Source` interface, `FileSource` struct, sentinel errors (`ErrNoInput`, `ErrBinary`, `ErrUnsupported`, `ErrNotFound`, `ErrPermission`), and `Metadata` struct in `internal/source/source.go` and `internal/source/file.go`. `Open()` returns a fresh `io.ReadCloser`; `Reopen()` returns an `io.ReadSeeker` for files.
- [ ] T019 Implement `FromArgs(args []string, stdin *os.File, hint string) (Source, error)` in `internal/source/source.go` covering only file paths and explicit `-`; `StdinSource` construction is deferred to US5 (return `ErrNoInput` when stdin would be needed).

### `internal/loader`

- [ ] T020 [P] Write failing tests for `Open(ctx, src, cfg)` in `internal/loader/stream_test.go`: small file (one chunk + EOF), multi-chunk, empty file, cancellation via ctx, error propagation through `Errs` channel, first chunk synchronously available before return, **bounded `Updates` channel (default cap 4): producer blocks on send when consumer falls behind, verified via a slow-consumer test that asserts the producer goroutine is not at runtime.NumGoroutine higher than baseline + 2 after 1s of idle**.
- [ ] T021 Implement `Chunk`, `Stream`, `Config`, and `Open(ctx, src, cfg)` in `internal/loader/stream.go` using `bufio.Scanner` with a 64 KiB read buffer. The first chunk is sized to ≥ `cfg.InitialChunkLines` (default = 2× viewport height = 80) so the first frame paints inside SC-001.
- [ ] T022 [P] Write failing tests for windowed mode in `internal/loader/window_test.go`: trigger threshold via `cfg.MaxResidentBytes`, slice access reads-ahead, slice access for non-resident range re-seeks the source, stdin (non-seekable) falls back to "scroll forward only" with the documented warning.
- [ ] T023 Implement windowing buffer (`Append`, `Slice`) and `MaxResidentBytes`-driven mode switch in `internal/loader/window.go`; emit `loader.WarnStdinNonSeekable` (a wrapped error sent on `Errs`) when stdin needs windowing.

### `internal/config`

- [ ] T024 [P] Write failing tests for defaults, TOML parsing, env-var merge (`SPY_THEME`, `SPY_VIM`, `SPY_GRAPHICS`, `NO_COLOR`), flag merge precedence, `[keys]` table override, and per-language `[lang.<name>]` table — in `internal/config/load_test.go`. Include cases for unknown keys (warn), bad types (warn + default), missing file (silent OK).
- [ ] T025 Replace `internal/config/config.go` with the full `Config` struct from `data-model.md` and a `Defaults()` constructor; implement `Load(opts LoadOptions) (*Config, []error)` in `internal/config/load.go` doing XDG lookup, TOML parse via `BurntSushi/toml`, env merge, flag merge — in that order — returning warnings (not errors) for soft failures.

### `internal/render` (skeleton)

- [ ] T026 [P] Write failing tests for `Renderer` interface dispatch via `ForKind` in `internal/render/renderer_test.go` covering all `source.Kind` values; the unsupported kinds return a no-op renderer that prints a warning frame.
- [ ] T027 Implement `Renderer` interface, `Dependencies` struct, `ForKind(k source.Kind, deps Dependencies) Renderer`, and a passthrough `KindText` renderer (line numbers + soft-wrap + theme defaults; no syntax highlighting) in `internal/render/renderer.go`. Other kinds dispatch to stub renderers that emit a "unsupported in foundational; pending USx" frame; the stubs are replaced in their respective story phases.
- [ ] T028 [P] Write failing tests for built-in dark/light `Theme` defaults in `internal/render/theme_test.go` (Chroma style resolution: `monokai` for dark, `github` for light; fallback when an unknown style name is requested).
- [ ] T029 Implement `Theme` struct and `Theme{Dark,Light}()` constructors plus `ResolveTheme(cfg, caps) Theme` placeholder (the auto-detect branch falls back to dark until US3) in `internal/render/theme.go`.

### `cmd/spy` rewiring

- [ ] T030 [P] Write failing tests for flag parsing in `cmd/spy/flags_test.go` — every flag from `contracts/cli.md` (long + short forms), env var fallback, `--help` / `--version` exits, `--config` vs `--no-config` mutual exclusion, unknown flag → exit 2.
- [ ] T031 Extract flag definitions into `cmd/spy/flags.go` exposing `ParseFlags([]string) (*ParsedFlags, error)`; pure (no side effects) so it's testable.
- [ ] T032 Replace `cmd/spy/main.go` with the foundational wiring: `term.Detect` → `config.Load` → `source.FromArgs` → `loader.Open` → `ui.NewModel` → `tea.NewProgram(model, tea.WithAltScreen()).Run()`. Implement FR-013 stderr error path with the documented exit codes (0/1/2/3/4/5/130/143) and the `spy: <reason>: <detail>` format from `contracts/cli.md`. Defer order (LIFO, panic-safe per research R10): `defer restore := term.Restore()` first, then `defer graphics.CleanupFunc(caps.Graphics)()` second so graphics cleanup runs *before* terminal restore. The graphics-cleanup defer is a no-op until US4 lands but the wiring is added here so panic-safety is correct from the foundational phase.

### `internal/ui` rewiring

- [ ] T033 [P] Write failing tests for `NewModel`, `Init`, `Update` (handles `tea.WindowSizeMsg`, `chunkLoadedMsg`, key events), and `View` in `internal/ui/model_test.go`. Cover: foundational quit on `q`/Esc/Ctrl-C, scroll up/down with arrow keys, end-of-file indicator on last line, resize reflows viewport.
- [ ] T034 Replace `internal/ui/model.go` with a `bubbles/viewport`-based `Model`, `NewModel(opts ModelOptions) Model`, `ModelOptions` matching `contracts/internal-apis.md`, and the foundational `Init`/`Update`/`View`. Wire the `internal/keys.Default()` keymap; vim mode is added in US2.
- [ ] T035 [P] Split UI into `internal/ui/update.go`, `internal/ui/view.go`, `internal/ui/help.go` (help is a stub overlay until later stories add bindings to surface).

### Cleanup of legacy skeleton

- [ ] T036 Delete `internal/reader/` and `internal/renderer/` packages. Move any still-useful utilities into the appropriate new package (`internal/source`, `internal/render`); update all imports under `cmd/spy/` and tests. After this task, `grep -r "internal/reader\|internal/renderer" .` returns no hits.

**Checkpoint**: `make test-race` and `make cover` pass at ≥ 80 % per package.
`spy /etc/hosts` shows the file in alt-screen, scrolls with arrow keys, and
exits cleanly with `q`. No syntax highlighting, no search, no graphics yet.

---

## Phase 3: User Story 1 - Quick Text Review with Syntax Highlighting (Priority: P1) 🎯 MVP

**Goal**: Open a code or markdown file with `spy file.go` and see proper
syntax highlighting in an alt-screen popup; dismiss with `q`/Esc.

**Independent Test**: Run `spy cmd/spy/main.go` and visually verify Go syntax
colours; run `spy README.md` and verify Glamour-rendered headings/lists. Run
the matching golden-file integration test.

### Tests for User Story 1 ⚠️ (write first, FAIL before implementation)

- [ ] T037 [P] [US1] Write failing tests for `Highlighter` in `internal/highlight/highlighter_test.go`: lexer auto-selection by language hint, fallback to plain text on unknown lexer, `HighlightCap` short-circuit (returns `Token{Type: Text}` for the whole line above the cap), streaming behaviour over a chunk channel.
- [ ] T038 [P] [US1] Write failing tests for the `KindCode` renderer in `internal/render/code_test.go`: ANSI output matches a golden file under `tests/integration/golden/hello_go_dark.txt`, line-number column rendering, `--no-line-numbers` honoured, soft-wrap behaviour.
- [ ] T039 [P] [US1] Write failing tests for the `KindMarkdown` renderer in `internal/render/markdown_test.go`: Glamour rendering of headings/lists/code blocks against a golden file under `tests/integration/golden/sample_md.txt`.
- [ ] T040 [P] [US1] Write a failing integration test in `tests/integration/text_review_test.go` driving the PTY harness against a small Go file: assert highlighted output, scroll-down behaviour, `q` exit.
- [ ] T041 [P] [US1] Add a failing E2E script `tests/e2e/01_text_review.sh` invoking the built binary against `tests/e2e/fixtures/hello.go`, snapshotting the alt-screen frame via the harness, and diffing against a golden screen.

### Implementation for User Story 1

- [ ] T042 [US1] Implement `Highlighter`, `New(theme, depth, capBytes)`, `Highlight(lang, line)`, and `HighlightStream(ctx, in, out)` in `internal/highlight/highlighter.go` using `chroma/v2` lexers + `formatters.TTY256`/`TrueColour` selected from `term.ColorDepth`. Honour `HighlightCap` from config (above-cap → no-op).
- [ ] T043 [US1] Replace the `KindCode` stub with the real renderer in `internal/render/code.go`: consumes `[]source.Token` from the highlighter (or raw `Line.Raw` when `Tokens == nil`), formats with `lipgloss.Style` for line numbers, soft-wraps at viewport width when `Config.WordWrap` is true.
- [ ] T044 [US1] Replace the `KindMarkdown` stub with a Glamour-backed renderer in `internal/render/markdown.go`. Pick the Glamour style from the active `Theme` (dark → `dark`, light → `light`). Disable Glamour when stdout color depth is `Mono`.
- [ ] T045 [US1] Update `internal/ui/Model.Update` in `internal/ui/update.go` to:
      (a) feed loader chunks through `Highlighter.HighlightStream` when `source.Kind == KindCode`,
      (b) trigger a re-render frame on `chunkLoadedMsg` so highlighted content appears progressively,
      (c) display "END" indicator (`render.Theme.Footer`) when the viewport reaches the last loaded line and `Status == Ready`.
- [ ] T046 [US1] Wire alt-screen entry/exit options in `cmd/spy/main.go`: `tea.WithAltScreen()`, `tea.WithMouseCellMotion()`, and a SIGWINCH-aware program option. Confirm `defer term.Restore()` still fires on panic.

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

- [ ] T047 [P] [US2] Write failing tests for `Compile` and `Matcher` in `internal/search/matcher_test.go`: literal substring, regex (`--regex` and `\v` prefix), case modes (smart/sensitive/insensitive), invalid regex returns error.
- [ ] T048 [P] [US2] Write failing tests for `Scan` in `internal/search/search_test.go`: forward/backward scanning, wrap-around detection, cancellation via ctx, partial matches across windowed-mode line buffers.
- [ ] T049 [P] [US2] Write failing tests for `WithVim(km KeyMap)` in `internal/keys/keymap_test.go`: vim bindings are additive (default arrows still work), `gg`/`G`/`Ctrl-D`/`Ctrl-U`/`0`/`$`/`h`/`j`/`k`/`l` resolve to the right `Action`.
- [ ] T050 [P] [US2] Write failing tests for command-line state machine in `internal/ui/update_test.go`: `:N` jumps, `:0`/`:$` aliases, out-of-range clamps to last loaded line + status warning, `:set vim` toggles, `:set theme dark|light|auto`, `:open <path>` swaps source, unknown command warns.
- [ ] T051 [P] [US2] Write a failing integration test in `tests/integration/search_test.go`: PTY drives `/9999\n`, asserts viewport scrolled to that line, `n` wraps with status message.
- [ ] T052 [P] [US2] Add failing E2E script `tests/e2e/02_search_navigation.sh` covering search forward/backward, jump-to-line, `:0`/`:$`, vim mode (run twice with and without `--vim`).

### Implementation for User Story 2

- [ ] T053 [US2] Implement `Compile`, `Matcher`, `CaseMode` in `internal/search/matcher.go` (smart-case heuristic: lowercase query → case-insensitive; otherwise sensitive). Regex path uses `regexp` stdlib; literal path uses `strings.Contains`-style scan for performance.
- [ ] T054 [US2] Implement `Scan(ctx, lines, m, dir, from)` returning a `<-chan Match` in `internal/search/search.go`. Honour cancellation; emit a synthetic `Wrapped` marker as the last channel value before close.
- [ ] T055 [US2] Implement `WithVim(km) KeyMap` in `internal/keys/vim.go` adding the additive bindings; tests T049 must pass.
- [ ] T056 [US2] Add `SearchState` and `CommandLineState` to `internal/ui/Model` and implement the `:` / `/` / `?` prompt state machine in `internal/ui/update.go`: prompt-open captures keystrokes, `Enter` submits, `Esc` cancels, history via Up/Down arrows.
- [ ] T057 [US2] Wire match navigation: highlight `SearchHit`/`SearchActive` regions in `internal/render/code.go` and `internal/render/markdown.go` (markdown highlight is best-effort over the rendered line). On `n`/`N` move `CurrentMatch`; on wrap, set the status bar to "search wrapped".
- [ ] T058 [US2] Implement command handlers (`:N`, `:0`, `:$`, `:set vim`, `:set novim`, `:set theme …`, `:open <path>`, `:q`/`:quit`) in `internal/ui/update.go`. `:open` reuses `loader.Open` + `source.FromArgs`.
- [ ] T059 [US2] Wire `--vim` flag and `vim_mode = true` config to invoke `keys.WithVim(km)` in `cmd/spy/main.go`. `:set vim` at runtime swaps the active keymap.

**Checkpoint**: User Story 2 is functional. Steps 4 and 5 of `quickstart.md`
pass. The viewer now supports search, jump, and vim mode.

---

## Phase 5: User Story 3 - Dark/Light Theme Detection + Override (Priority: P1)

**Goal**: Spy auto-detects the terminal background luminance and picks a
readable theme; `--theme` / `SPY_THEME` / config file overrides win.

**Independent Test**: Steps 6 and 15 of `quickstart.md` pass; `:set theme
light` at runtime visibly switches.

### Tests for User Story 3 ⚠️

- [ ] T060 [P] [US3] Write failing tests for OSC 11 query + `COLORFGBG` fallback in `internal/term/theme_test.go` (mock the response stream; assert luminance ≥ 0.5 → light, < 0.5 → dark, malformed → NaN, timeout → NaN).
- [ ] T061 [P] [US3] Write failing tests for `ResolveTheme(cfg, caps)` in `internal/render/theme_test.go`: `auto` + light caps → light theme; `auto` + dark caps → dark theme; `auto` + NaN luminance → dark fallback; explicit `light`/`dark`/`<chroma-style>` always wins.
- [ ] T062 [P] [US3] Write a failing integration test in `tests/integration/theme_test.go` using a fake PTY that replies to OSC 11 with a light-grey colour, asserting the rendered ANSI uses the `github` Chroma style.
- [ ] T063 [P] [US3] Add failing E2E script `tests/e2e/03_theme.sh` covering `--theme dark`, `--theme light`, `SPY_THEME=light`, `NO_COLOR=1` (forces mono).

### Implementation for User Story 3

- [ ] T064 [US3] Implement OSC 11 luminance probe via `muesli/termenv`'s `BackgroundColor()` in `internal/term/theme.go`, with a 50 ms total timeout and `COLORFGBG` env-var fallback.
- [ ] T065 [US3] Wire `BackgroundLuminance` into `term.Detect()` (replacing the `NaN` placeholder from T012). Honour `SPY_THEME` to short-circuit the probe entirely.
- [ ] T066 [US3] Replace the placeholder `ResolveTheme` from T029 with the full implementation in `internal/render/theme.go`: respects `cfg.Theme` (`auto`/`dark`/`light`/named Chroma style), `caps.BackgroundLuminance`, and `cfg.NoColor` (forces a Mono theme that disables Chroma styling).
- [ ] T067 [US3] Wire runtime theme swap (`:set theme …` from T058) to `ResolveTheme` and re-render in `internal/ui/update.go`; the in-memory token buffer is reused — only the formatter changes.

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

- [ ] T068 [P] [US4] Write failing unit tests for the Kitty encoder in `internal/graphics/kitty_test.go`: chunked base64 framing at 4096 B, escape sequence shape, `Cleanup` returns the documented "delete all images" sequence, **full-payload golden test (T068b below)**.
- [ ] T068b [P] [US4] Write failing golden-payload test in `internal/graphics/kitty_test.go`: encode a deterministic 16×16 PNG (checked into `internal/graphics/testdata/kitty_input.png`) and assert the **complete** escape stream byte-for-byte against `internal/graphics/testdata/kitty_expected.bin`. This catches malformed payloads between the `\x1b_G…` prefix and `\x1b\\` terminator that look correct under T068's prefix-only check but render as broken images.
- [ ] T069 [P] [US4] Write failing unit tests for the iTerm2 encoder in `internal/graphics/iterm2_test.go`: `\x1b]1337;File=…;preserveAspectRatio=1:<base64>\x07` shape **plus full-payload golden test against `internal/graphics/testdata/iterm2_expected.bin`** using the same input PNG as T068b.
- [ ] T070 [P] [US4] Write failing unit tests for the sixel encoder in `internal/graphics/sixel_test.go`: round-trips a small in-memory PNG via `mattn/go-sixel` **plus a full-payload golden test against `internal/graphics/testdata/sixel_expected.bin`** using the same input PNG. Note: sixel output may vary across `go-sixel` versions; pin the dep version in `go.mod` and document the regen procedure in `internal/graphics/testdata/README.md`.
- [ ] T071 [P] [US4] Write failing tests for the `KindImage` renderer in `internal/render/image_test.go`: capable path emits the right protocol bytes; non-capable path emits a deterministic metadata block (filename, dimensions, size, fallback notice).
- [ ] T072 [P] [US4] Write failing tests for the `KindPDF` renderer in `internal/render/pdf_test.go`: pdfcpu text extraction path returns page text; capable + `fitz` build tag triggers `graphics.PDFPage`; capable + `nofitz` build returns `ErrPDFGraphicsUnavailable` + falls back to text.
- [ ] T073 [P] [US4] Write failing integration tests for graphics dispatch in `tests/integration/graphics_test.go`: assert that with `term.Capabilities.Graphics == GraphicsKitty` the rendered frame contains the **complete** Kitty payload (prefix `\x1b_G`, the same base64 chunks the unit-test golden produces, terminator `\x1b\\`) — not just the prefix. Diff against the same golden file as T068b. Repeat for iTerm2 and sixel paths.
- [ ] T074 [P] [US4] Add failing E2E script `tests/e2e/04_graphics.sh` running `--graphics none` (forces metadata fallback) and `--graphics kitty` (forces Kitty encoder) against fixture image and PDF.

### Implementation for User Story 4

- [ ] T075 [US4] Implement the Kitty graphics encoder in `internal/graphics/kitty.go` (~80 LOC, chunked base64 framing) and the cleanup escape.
- [ ] T076 [US4] Implement the iTerm2 inline-image encoder in `internal/graphics/iterm2.go`.
- [ ] T077 [US4] Implement the sixel encoder wrapper around `mattn/go-sixel` in `internal/graphics/sixel.go` (uses `golang.org/x/image` for source decoding).
- [ ] T078 [US4] Implement `Render(proto, img, cols, rows)` and `Cleanup(proto)` dispatchers in `internal/graphics/graphics.go`.
- [ ] T079 [US4] Implement build-tag-gated `internal/graphics/pdf_fitz.go` (build tag `fitz`) using `gen2brain/go-fitz` to rasterize a single page to `image.Image`; and `internal/graphics/pdf_nofitz.go` (build tag `!fitz`) returning the documented sentinel `ErrPDFGraphicsUnavailable`.
- [ ] T080 [US4] Replace the `KindImage` stub in `internal/render/image.go`: capable terminals → `graphics.Render`; otherwise the metadata fallback block. Re-open the file at render time (per `research.md` R2) to keep memory under SC-005.
- [ ] T081 [US4] Replace the `KindPDF` stub in `internal/render/pdf.go`: pdfcpu page-text extraction by default; on graphics-capable terminals AND `fitz` build, rasterize the current page and render via `graphics.Render`. Track `Page` index in the renderer state.
- [ ] T082 [US4] Wire `]` / `[` page navigation when `source.Kind == KindPDF` and `:N` for page-jump (re-using the command-line state from T056) in `internal/ui/update.go`.
- [ ] T083 [US4] Wire panic-safe graphics cleanup in **two** places (per `research.md` R10):
      (a) `cmd/spy/main.go`: after capability detection, capture `cleanupGraphics := graphics.CleanupFunc(caps.Graphics)` and `defer cleanupGraphics()` *inside* the existing `defer term.Restore()` chain (LIFO ordering: graphics cleanup runs first, then terminal restore). This fires on `tea.Quit`, signals, AND panic — including cgo `go-fitz` panics that skip Bubble Tea's normal teardown.
      (b) `internal/ui/update.go`: on source replacement (`:open`), invoke `graphics.Cleanup(caps.Graphics)` via a `tea.Cmd` so the previous source's images are cleared before the new ones render. The cleanup func must be idempotent.
      Add a `graphics.CleanupFunc(proto term.Graphics) func()` constructor in `internal/graphics/graphics.go` returning a closure that writes the appropriate escape sequence to `os.Stdout` (no-op for `GraphicsNone`, `GraphicsITerm2`, `GraphicsSixel`).
- [ ] T084 [US4] Honour `--graphics` flag and `SPY_GRAPHICS` env via the merge order from T024 (CLI > env > config > auto-detect). `--graphics none` short-circuits all encoding.

**Checkpoint**: User Story 4 is functional. Steps 7 and 8 of `quickstart.md`
pass on both graphics-capable and non-graphics terminals; default and
`-tags fitz` builds both compile and behave per the contract.

---

## Phase 7: User Story 5 - Pipe Input Support (Priority: P2)

**Goal**: `git diff | spy` and `cat file.go | spy -l go` work the same way
they do in `bat` — content streams from stdin, never touches disk, and
language is auto-detected (shebang, hint, Chroma `Analyse`).

**Independent Test**: Step 3 of `quickstart.md` passes.

### Tests for User Story 5 ⚠️

- [ ] T085 [P] [US5] Write failing tests for `StdinSource` in `internal/source/stdin_test.go`: `Open()` returns the underlying reader, `Reopen()` returns `nil, ErrNotSeekable`, `DisplayName()` is `<stdin>`.
- [ ] T086 [P] [US5] Write failing tests for stdin language inference in `internal/source/detect_test.go`: shebang first line, `--lang` hint override, Chroma `Analyse` fallback, plain text when none match.
- [ ] T087 [P] [US5] Write failing tests for `FromArgs` stdin construction in `internal/source/source_test.go`: TTY stdin + no file → `ErrNoInput`; non-TTY stdin + no file → `StdinSource`; `-` positional + TTY stdin → `StdinSource` (blocks per contract); both file and stdin → file wins.
- [ ] T088 [P] [US5] Write a failing integration test in `tests/integration/stdin_test.go`: PTY pair, write a Go diff to stdin, assert highlighted-diff output and `<stdin>` in the footer.
- [ ] T089 [P] [US5] Add failing E2E script `tests/e2e/05_pipe.sh`: `git diff HEAD~ | spy` (when run inside a git repo), `echo content | spy -`, and the degenerate-cat mode (`echo content | spy | cat`) which must exit 0 and pass content through verbatim with no escape sequences.

### Implementation for User Story 5

- [ ] T090 [US5] Implement `StdinSource` in `internal/source/stdin.go`. `Open()` returns an `io.NopCloser(os.Stdin)`-like reader that's safe to close; `Reopen()` returns `nil, ErrNotSeekable`.
- [ ] T091 [US5] Update `FromArgs` (T019) to construct `StdinSource` when stdin is non-TTY and no file argument; honour the `-` positional explicitly.
- [ ] T092 [US5] Update `detectKind` (T016) to support stdin: read the first 8 KiB into a `bytes.Buffer`, detect, then prepend the buffered bytes back into the stream via a `MultiReader`.
- [ ] T093 [US5] Implement degenerate-cat mode in `cmd/spy/main.go`: when stdin is consumed AND stdout is not a TTY, copy the input verbatim to stdout and exit 0 (no alt-screen). This is the contract from `contracts/cli.md` "stdin behavior".
- [ ] T094 [US5] Update the footer/`DisplayName` plumbing so stdin sessions show `<stdin>` instead of a basename — touches `internal/render/statusbar.go` (still a stub at this point; full footer is US6).

**Checkpoint**: User Story 5 is functional. Step 3 of `quickstart.md` passes.

---

## Phase 8: User Story 6 - File Metadata Footer (Priority: P3)

**Goal**: A status bar at the bottom of the viewer shows
`<displayname> | <total> lines | Line <current>` (or `Page m/n` for PDFs),
updates on scroll/resize, collapses gracefully under 80 columns.

**Independent Test**: Step 2 (footer line count + position) and step 14
(minimum-size degradation) of `quickstart.md` pass.

### Tests for User Story 6 ⚠️

- [ ] T095 [P] [US6] Write failing tests for `statusbar.Render(meta, viewport)` in `internal/render/statusbar_test.go`: standard format, streaming-mode `…` indicator while `LineCount == -1`, PDF page indicator, sub-80-column collapse to single short line.
- [ ] T096 [P] [US6] Write a failing integration test in `tests/integration/footer_test.go` driving the PTY through scroll-down and asserting the line counter advances correctly.
- [ ] T097 [P] [US6] Add failing E2E script `tests/e2e/06_footer.sh` covering scrolling end-to-end, resize, and the sub-80-column degraded layout.

### Implementation for User Story 6

- [ ] T098 [US6] Implement `statusbar.Render(meta source.Metadata, vp viewport.Model, theme Theme) string` in `internal/render/statusbar.go`. Use `lipgloss` styles from the active `Theme`. Below 80 cols, emit only `<short-name> · L<current>`.
- [ ] T099 [US6] Wire the statusbar into `internal/ui/View` in `internal/ui/view.go` so it renders on every frame; subscribe to `tea.WindowSizeMsg` for width updates.
- [ ] T100 [US6] Update `loader.Stream` to surface `LineCount` updates (post a `metaUpdatedMsg{TotalLines int64}` on EOF) so the footer can switch from `…` to the final count without re-rendering everything.

**Checkpoint**: All P1/P2/P3 stories functional. `quickstart.md` Steps 1–15
all pass on a Linux + xterm-compatible terminal.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Lock in non-functional requirements, finalize documentation,
and close residual constitution TODOs.

- [ ] T101 [P] Refresh `README.md` to describe `spy` (not the placeholder text), with a quick install + usage section linking to `specs/001-popup-reader/contracts/cli.md`, `keys.md`, and `config.md`.
- [ ] T102 [P] Update `DEVELOPMENT.md` with `make` target descriptions, the `-tags fitz` build instructions, and a brief on the `tests/integration` PTY harness.
- [ ] T103 [P] Refresh the Constitution Check section in `specs/001-popup-reader/plan.md` to cite constitution v1.0.0 (closes `TODO(plan-001)` from `.specify/memory/constitution.md`).
- [ ] T104 [P] Add SC-001 benchmark in `tests/perf/firstframe_bench_test.go`: load a 100-line file end-to-end and assert ≤ 100 ms.
- [ ] T105 [P] Add SC-003 benchmark in `tests/perf/search_bench_test.go`: search across a 1 MiB synthetic file and assert ≤ 500 ms.
- [ ] T106 [P] Add SC-005 benchmarks in `tests/perf/large_file_test.go` in **two tiers**:
      (a) `TestLargeFile_PRGate`: 200 MiB synthetic file (just above the 256 MiB windowed-mode threshold's complement — the largest file that *doesn't* trigger windowing), asserts RSS ≤ 250 MiB. Runs on every PR (no build tag); ~5s on commodity CI; gates the PR.
      (b) `TestLargeFile_Nightly`: 1 GiB synthetic file, asserts RSS ≤ 500 MiB. Behind `-tags perf`; CI runs it on a nightly schedule via `.github/workflows/nightly-perf.yml` against an `ubuntu-latest` runner with `RUNNER_OS_RAM_HINT=8192`. A failure files an issue tagged `perf-regression` rather than blocking PRs (because the nightly cadence makes blame attribution clear).
      The PR-gate version catches algorithmic regressions; the nightly catches scaling-only regressions. SC-005's 1 GiB / 500 MiB promise is owned by (b).
- [ ] T106a [P] Add SC-006 corpus + check in `tests/perf/highlight_corpus_test.go`: 50 source files under `tests/fixtures/highlight-corpus/` (one per GitHub Linguist top-50 language by repo count, each ≤ 4 KiB representative sample); assert Chroma selects a non-`fallback` lexer and ≤ 1 % of bytes per file land in `chroma.Error` tokens. Pass threshold: 47/50.
- [ ] T106b [P] Add SC-007 dismiss benchmark in `tests/perf/dismiss_bench_test.go`: drive the PTY harness against `big.txt` 100 times, send `q`, measure wall-clock from keypress to `tea.Program.Run()` return; assert p95 ≤ 500 ms.
- [ ] T106c [P] Add `.github/workflows/nightly-perf.yml`: scheduled `cron: '0 8 * * *'`, runs `make perf` (`go test -tags perf ./tests/perf/...`), uploads benchmark output as an artifact, opens an issue tagged `perf-regression` on failure (`peter-evans/create-issue-from-file@v5` or equivalent). Also add `make perf` target to `Makefile` (T003 add-on).
- [ ] T107 [P] Run `reuse lint` and add SPDX headers to any files added across all phases that missed them. CI hook from T003 must pass.
- [ ] T108 [P] Manual quickstart.md walkthrough on Linux/xterm; record results in `specs/001-popup-reader/checklists/quickstart-validation.md`.
- [ ] T109 [P] Manual quickstart.md walkthrough on macOS/iTerm2 + Kitty; same checklist.
- [ ] T110 Add a `CHANGELOG.md` `0.1.0` entry summarizing US1–US6.
- [ ] T111 Final gate: `make lint vet test-race cover` clean, per-package coverage ≥ 80 % computed against the **merged** profile from `cover-default` + `cover-fitz` (so build-tag-gated files in `internal/graphics` are not silently excluded), `reuse lint` clean, default and `-tags fitz` builds both succeed.

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
