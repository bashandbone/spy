# Feature Specification: Popup Reader - Focused Text/PDF/Image Viewer

**Feature Branch**: `001-popup-reader`  
**Created**: 2026-04-25  
**Status**: Draft  
**Input**: Design a focused review tool with popup window for viewing text in tmux panes with syntax highlighting, theming, and PDF/image support

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Quick Text Review in Terminal Panes (Priority: P1)

A developer using tmux with multiple panes needs to quickly review a file without leaving the terminal environment or disrupting their current layout. They want to "pop up" a focused view that displays the file contents with proper syntax highlighting, then dismiss it to return to their normal workflow.

**Why this priority**: This is the core use case - solving the problem of difficult text review in constrained terminal environments. It's the MVP that delivers immediate value.

**Independent Test**: Can be fully tested by opening a text file with spy in a terminal, verifying syntax highlighting displays correctly, and confirming the window can be dismissed without losing terminal state.

**Acceptance Scenarios**:

1. **Given** `hello.go` exists on disk, **When** user runs `spy hello.go`, **Then** the alt-screen launches within 100 ms (SC-001), `hello.go` content is visible with Go syntax highlighting applied via Chroma's `go` lexer, and the footer reads `hello.go | <N> lines | Line 1`.
2. **Given** the viewer is open, **When** user presses `↓` or `j`, **Then** the viewport scrolls one line down without visible flicker; `PgDn` / `Space` advances one viewport-height; `Home` / `End` jump to top/bottom.
3. **Given** the viewer is open, **When** user presses `q`, `Esc`, or `Ctrl-C`, **Then** the alt-screen exits, the previous shell prompt is visible unchanged (no residual escape sequences, cursor restored, modes restored), and the process exits with code 0 (or 130 for `Ctrl-C`) within the SC-007 budget.
4. **Given** content fills multiple screens, **When** user scrolls past the last loaded line, **Then** the footer shows `END` styled with `Theme.Footer` and further down-scroll is a no-op (no error, no wrap).

---

### User Story 2 - Syntax Highlighting and Code Navigation (Priority: P1)

A developer reviewing code in the popup needs proper syntax highlighting to quickly understand the code structure, and wants to navigate efficiently using common patterns (jump to line, search for text).

**Why this priority**: Syntax highlighting is essential for code review and is a core differentiator from simple cat/less viewers. Code navigation enables real review workflows.

**Independent Test**: Can be fully tested by opening a code file (e.g., Go, Python, JavaScript), verifying correct syntax coloring, testing line-number navigation, and confirming search functionality works.

**Acceptance Scenarios**:

1. **Given** a file with a recognized extension or shebang is displayed, **When** the renderer initializes, **Then** Chroma selects a non-`fallback` lexer (verified per the SC-006 corpus) and tokens are styled per the active `Theme.ChromaStyle`.
2. **Given** content is displayed, **When** user presses `:`, types `42`, presses `Enter`, **Then** the viewport jumps so line 42 is visible (centered if possible; clamped to last loaded line if 42 > total). Out-of-range jumps emit a status-bar warning (`line 99999 out of range; clamped to <last>`) rather than failing. `:0` aliases to line 1; `:$` aliases to the last line.
3. **Given** content is displayed, **When** user presses `/`, types a query, presses `Enter`, **Then** the first match at-or-below the current line is highlighted with `Theme.SearchActive`; all other matches in the visible range are highlighted with `Theme.SearchHit`; the footer shows `<query> · <current>/<total>` matches. `?` performs the same with backward direction.
4. **Given** N matches exist for the active search and the cursor is on match `i`, **When** user presses `n`, **Then** the viewport advances to match `i+1`; if `i == N`, it wraps to match 1 AND the status bar shows `search wrapped` until the next non-search keypress. `N` performs the reverse. With zero matches, the status bar shows `no matches for <query>` and `n`/`N` are no-ops.

---

### User Story 3 - Dark/Light Theme Support (Priority: P1)

A developer wants to use spy with their preferred terminal theme (dark or light mode) without jarring color mismatches. The viewer should adapt to the terminal's theme automatically or allow manual override.

**Why this priority**: Terminal theming is fundamental to the user experience. Poor color contrast or theme mismatches would make the tool unusable in many environments.

**Independent Test**: Can be fully tested by running spy with a light terminal theme, verifying readability, then switching to dark theme and confirming colors adapt appropriately.

**Acceptance Scenarios**:

1. **Given** the terminal responds to OSC 11 with a background color whose relative luminance is ≥ 0.5 (light), **When** spy launches with `theme = "auto"` (default), **Then** `Theme.ChromaStyle` resolves to the `github` Chroma style and `Theme.LineNumber` / `Theme.Footer` use the light palette.
2. **Given** the terminal background luminance is < 0.5 (dark) OR OSC 11 returns no response within 50 ms, **When** spy launches with `theme = "auto"`, **Then** `Theme.ChromaStyle` resolves to `monokai` and the dark palette is used.
3. **Given** any terminal, **When** spy is invoked with `--theme dark|light|<chroma-style>` OR `SPY_THEME=…` is set OR `theme = "…"` is in the config file, **Then** the explicit value wins over auto-detection (precedence: flag > env > config > auto). Setting `--theme github` with no terminal probe still works.

---

### User Story 4 - PDF and Image Support in Modern Terminals (Priority: P2)

**Primary actor**: developer triaging a bug report or reviewing a design artifact.
**Trigger**: a teammate has shared a screenshot, diagram, or PDF (e.g., a Slack drop, a `gh issue view` attachment, a research paper from a download folder), and the developer wants to confirm the visual content matches the description without leaving the terminal or context-switching to a GUI viewer.
**Goal**: see the image/PDF clearly enough to read embedded text and identify diagram structure (not just confirm a thumbnail is "the right colour"), or — when the terminal can't — get an honest, useful metadata fallback that names the file, dimensions, and size.

**Why this priority**: This is a differentiator feature that extends spy beyond text-only viewers. While text is the primary use case, supporting PDFs and images in capable terminals adds significant value without disrupting the text workflow.

**Independent Test**: Can be fully tested by opening a PDF in a capable terminal (Kitty or iTerm2), verifying the rendered image is large enough to read 12-pt embedded text, and confirming the metadata fallback in terminals without image support shows filename + dimensions + size.

**Acceptance Scenarios**:

1. **Given** a PDF is opened with `spy paper.pdf` in a Kitty/iTerm2/WezTerm session, **When** the file is identified as PDF, **Then** the current page rasterizes inline at the viewport width and rendered text in the page is readable at the user's normal terminal font size.
2. **Given** an image file is opened with `spy diagram.png` in a Kitty/iTerm2/WezTerm session, **When** the terminal supports inline rendering, **Then** the image displays inline preserving aspect ratio and the user can identify text labels in the image.
3. **Given** a terminal does not support image rendering (e.g., GNOME terminal), **When** a PDF or image is opened, **Then** a metadata block appears showing filename, dimensions (`W × H` for images, `N pages` for PDFs), file size, and a single-line note (`graphics not supported in this terminal`); for PDFs, page-1 text extraction is also displayed when available.

---

### User Story 5 - Pipe Input Support (Priority: P2)

A developer wants to pipe command output directly to spy without saving to a file, similar to how bat handles piped input. This enables workflows like `cat file.go | spy` or `git diff HEAD~ | spy`.

**Why this priority**: Piping is a Unix philosophy pattern that enables flexible workflows. It's important but secondary to opening files directly.

**Independent Test**: Can be fully tested by running `echo "test content" | spy`, verifying content displays correctly, and testing with command output (e.g., `git diff | spy`).

**Acceptance Scenarios**:

1. **Given** stdin is non-TTY and no `FILE` argument is provided (e.g., `git diff | spy`), **When** spy starts, **Then** stdin content streams into the viewer; the footer shows `<stdin> | … lines | Line 1` (line count shows `…` until EOF, then the final count).
2. **Given** piped content is displayed, **When** language detection runs, **Then** the order is: explicit `--lang` > shebang on first line (`#!/usr/bin/env python3` → `python`) > Chroma `lexers.Analyse` content sniffing > plain text fallback. Detection result appears in the footer when non-empty.
3. **Given** piped content has been displayed and the viewer is dismissed, **When** the process exits, **Then** no file is created under `$TMPDIR`, `/tmp`, `$XDG_CACHE_HOME`, or the current working directory; verified by `tests/integration/stdin_test.go` snapshotting the relevant directories pre/post.

---

### User Story 6 - File Metadata and Navigation Context (Priority: P3)

A developer using the tool wants to know which file they're viewing, how many lines it contains, and their current position in the file. This context helps during code review.

**Why this priority**: Nice-to-have feature that improves usability but isn't essential for core functionality. Can enhance the experience after MVP is solid.

**Independent Test**: Can be fully tested by opening a file and checking that the footer displays file name, line count, and current position (e.g., "file.go | 245 lines | Line 42").

**Acceptance Scenarios**:

1. **Given** a file is displayed, **When** the viewer is open, **Then** a footer shows the file name and total line count
2. **Given** user navigates through content, **When** they are at line N of M, **Then** the footer updates to show current line position

---

### Edge Cases

- How does the tool handle files with very long lines (>1000 characters)? Soft-wrap by default (`word_wrap = true`); `--no-wrap` enables horizontal scroll. Lines longer than 100 KiB are truncated at 100 KiB with a status-bar warning to bound per-line memory.
- What happens when terminal is resized while the viewer is open? (Already covered in FR-014)
- What happens when both a file argument and non-TTY stdin are provided? File wins; stdin is ignored. See `contracts/cli.md` resolution table for the full matrix.
- What happens with a 0-byte file or empty stdin? Viewer launches with a single styled `(empty)` line; not an error. See `contracts/cli.md` "Empty input".
- What happens when a file is deleted or its permissions change while the viewer is open? Existing buffer remains viewable; reload (`Ctrl-R`/`r`) surfaces the failure as a status-bar error and keeps the prior buffer.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept file path as command-line argument: `spy <file>`
- **FR-002**: System MUST accept piped input and display contents when invoked without file argument
- **FR-003**: System MUST apply syntax highlighting based on file extension or inferred language type
- **FR-004**: System MUST support dark mode and light mode themes that adapt to terminal capabilities
- **FR-005**: System MUST provide keyboard navigation via arrow keys (↑↓←→, page up/down) as primary; optional vim mode (hjkl, /, :) enabled via flag or config
- **FR-006**: System MUST support jumping to specific line numbers via ':' command followed by line number
- **FR-007**: System MUST support text search via '/' (forward) and '?' (backward) with 'n'/'N' navigation
- **FR-008**: System MUST allow dismissal of viewer via 'q' or 'ESC' key without altering terminal state
- **FR-009**: System MUST display file metadata in footer (file name, line count, current position) when viewing files
- **FR-010**: System MUST support detection and inline rendering of image files in compatible terminals (Kitty, iTerm2, WezTerm)
- **FR-011**: System MUST support PDF preview/rendering in compatible terminals with graceful fallback in unsupported terminals
- **FR-012**: System MUST handle large files gracefully via progressive loading (initial viewport paints immediately while remaining content streams in the background). Files exceeding 256 MiB MUST switch to windowed mode (a sliding hot region kept in RAM, with the rest re-read from disk on demand). Files up to 1 GiB MUST remain viewable, with resident memory ≤ 500 MB (cross-ref SC-005). For non-seekable sources (stdin) above 256 MiB, the viewer enters scroll-forward-only mode with a status-bar warning rather than failing.
- **FR-013**: System MUST output error messages to stderr (single-line, prefixed `spy: <reason>: <detail>`) and exit without launching the viewer if file is inaccessible, binary, or unsupported format. Exit codes follow `contracts/cli.md` (3 = I/O, 4 = unsupported, 2 = usage)
- **FR-014**: System MUST handle terminal resize events and reflow content appropriately
- **FR-015**: System MUST exit cleanly on signals (SIGINT, SIGTERM) without corrupting terminal state

### Key Entities

- **ViewerSession**: Represents an active spy viewing session
  - Current file or piped content
  - Current scroll position
  - Search state (active search query, current match position)
  - Theme preference (dark/light/auto)
  
- **FileMetadata**: Information about the file being viewed
  - File path or source (stdin, file)
  - File type/language for syntax highlighting
  - Line count
  - File size
  
- **TerminalCapabilities**: Terminal feature detection
  - Supports image rendering (Kitty protocol, iTerm2, etc.)
  - Color depth (8-bit, 256-color, true color)
  - Dimensions (rows, columns)

## Clarifications

### Session 2026-04-25

- **Q1: Loading & Streaming Strategy for Large Files** → **A: Progressive loading with concurrent goroutines (Option B)**
  - Large files load viewport first, remaining content streams in background
  - No blocking on initial display; user sees content immediately
  - Graceful scrolling within loaded region; degradation only if scrolling ahead of stream

- **Q2: Error Handling & Fallback UX** → **A: Errors to stderr, no viewer display (minimal Unix approach)**
  - If file cannot be accessed, is binary, or format unsupported: print error message to stderr (`spy: <reason>: <detail>`) and exit with the matching code from `contracts/cli.md`
  - Viewer only launches if there is displayable content
  - No error dialogs or fallback rendering; fail fast and cleanly
  - *Note*: original Q2 wording said "stdout"; corrected here to match Unix convention and `contracts/cli.md`. Stdout is reserved for the alt-screen TUI (TTY) or verbatim content (degenerate-cat mode).

- **Q3: Accessibility & Keybinding Scope** → **A: Arrow keys primary with optional vim mode (inverted approach)**
  - Default keybindings use arrow keys (↑↓←→) for maximum accessibility
  - Vim mode (hjkl, /, :) available as optional flag or config for power users
  - No Emacs keybindings or advanced remapping in v1

- **Q4: Minimum Terminal Dimensions & Small Window Behavior** → **A: Graceful degradation with 80×24 minimum (Option B)**
  - Minimum supported terminal size: 80 columns × 24 rows (standard terminal)
  - Below 80 columns or 8 rows: tool still functions but with reduced layout (single-column view, minimal footer)
  - No hard error; graceful behavior across all reasonable terminal sizes

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can open and view a 100-line text file with syntax highlighting in under 100ms from invocation
- **SC-002**: Scrolling a 10 000-line file (`/tmp/spy-fixtures/big.txt`) is smooth. Measured by `tests/perf/scroll_bench_test.go`: 100 sequential ScrollDown actions driven through the PTY harness, p95 per-frame wall-clock ≤ 16 ms (60 fps target) and zero dropped frames.
- **SC-003**: Text search returns results in under 500ms even in files larger than 1MB
- **SC-004**: Theme switching at runtime (`:set theme dark|light|<style>`) re-renders the visible viewport in ≤ 16 ms p95 without re-tokenizing the in-memory buffer (Chroma styles are applied at format time per research R7). Measured by `tests/perf/theme_swap_bench_test.go` averaging 100 swaps against a 10 000-line file.
- **SC-005**: The tool successfully handles file sizes up to 1GB without consuming more than 500MB of memory
- **SC-006**: For a fixed corpus of 50 representative source files (one per language from the GitHub Linguist top-50 by repository count, captured under `tests/fixtures/highlight-corpus/`), Chroma successfully selects a non-`fallback` lexer and produces tokenization where ≤ 1 % of bytes per file land in `chroma.Error` tokens. Pass threshold: ≥ 47/50 files (94 %). Measured by `tests/perf/highlight_corpus_test.go`.
- **SC-007**: From the user pressing `q`/`Esc`/`Ctrl-C` to `tea.Program.Run()` returning and the alt-screen having been exited (terminal back to main screen, cursor restored), elapsed wall-clock ≤ 500 ms at the 95th percentile across 100 invocations against `/tmp/spy-fixtures/big.txt` (10 000 lines). Measured by `tests/perf/dismiss_bench_test.go` driving the PTY harness.
- **SC-008**: After a `tea.WindowSizeMsg` arrives, (a) the line previously at viewport row 0 remains at row 0, (b) the wrap cache is invalidated, (c) the next frame paints in ≤ 16 ms p95. Measured by `tests/integration/resize_test.go` driving 50 successive resize events at random widths in `[40, 200]` cols.
- **SC-009**: For each fixture in `tests/fixtures/img/{small.png 32 KB, medium.jpg 5 MB, large.gif 49 MB}`, `spy <file>` in a Kitty-emulating PTY (a) emits a Kitty payload that round-trips through the harness's reference decoder back to a pixel-identical `image.Image`, (b) exits 0, (c) keeps resident memory ≤ 250 MB. Measured by `tests/integration/graphics_test.go` (extends T073).
- **SC-010**: For `dummy.pdf` (single-page, known text "Dummy PDF file") and `tests/fixtures/pdf/multi-page.pdf` (3 pages), the renderer (a) rasterizes page 1 via `graphics.PDFPage` under `-tags fitz` in a graphics-capable PTY, and (b) extracts page text via `pdfcpu` in a non-graphics PTY — the extracted text MUST contain the known sentinel string. Measured by `tests/integration/pdf_test.go`.
- **SC-011**: Three pipeline shapes pass end-to-end in `tests/e2e/05_pipe.sh`: (a) `cat tests/fixtures/hello.go | spy -l go` displays Go-highlighted content with `<stdin>` in the footer; (b) `git diff HEAD~ | spy` displays diff-highlighted content; (c) `grep -n needle tests/fixtures/hello.go | spy` displays plain text with `<stdin>` in the footer. All three exit 0 on `q`.
- **SC-012**: Three independent reviewers (not the implementer) complete the `quickstart.md` Steps 2, 4, and 12 (open file, search + jump-to-line, resize) using only the in-app help overlay (`F1`/`?`); each reviewer records pass/fail and notes blockers in `specs/001-popup-reader/checklists/quickstart-validation.md`. Pass threshold: 3/3 reviewers pass all three steps without escaping to external docs. *Note*: original SC-012 ("90 % of users without docs") would require a funded user study with N ≥ 20; deferred to a post-v0.1.0 success metric. The reviewer-panel heuristic above is the v0.1.0 gate.

## Assumptions

- **User Environment**: Users have access to a modern terminal emulator (xterm-compatible or better) with at least 256 colors support. Minimum supported dimensions are 80×24 (standard terminal); tool degrades gracefully below 80 columns or 8 rows. Extended image support (Kitty protocol, iTerm2) is optional but enables enhanced features.
- **File System**: Spy operates on local files only; remote file systems (NFS, SSH) are out of scope for v1 but could be added if performance is acceptable.
- **Scope - Text Focus**: The primary use case is text file review; PDF and image support is supplementary and can degrade gracefully.
- **Large File Handling**: Files larger than 1GB should be handled via streaming or pagination; attempting to load entire file into memory is prohibited.
- **Binary File Behavior**: Binary files will display a clear "unsupported format" message rather than attempting to render binary data.
- **Terminal State Preservation**: The tool will restore terminal state (colors, cursor position, echo mode) on exit in all cases, even on error or signal.
- **Configuration**: User preferences (theme, keybindings) can be configured via a simple config file (~/.spy/config) or environment variables; full config system design is out of scope for v1.
- **Stdin Handling**: Piped input streams into the viewer through the same chunked loader as files (per FR-002 and US5 acceptance #1); content is held only in memory subject to the FR-012 resident-memory bound, and is never persisted to disk. Non-seekable stdin above the windowed-mode threshold falls back to scroll-forward-only mode with a status-bar warning (per FR-012).
- **Large File Loading**: Large files (>100MB) use progressive/concurrent loading: initial viewport displays immediately while remaining content streams in background. No blocking waits for full file load.
- **Keyboard Shortcuts**: Default keybindings use arrow keys for navigation (accessible to all users). Optional vim mode (hjkl, /, :) can be enabled via `--vim` flag or config file. Emacs keybindings and full customization out of scope for v1.
