<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Data Model: Popup Reader

**Feature**: 001-popup-reader
**Date**: 2026-04-25

This document captures the runtime data model. Types are described
language-agnostically; concrete Go declarations live in the linked package
columns and are produced during /speckit-implement.

## Package map (Go)

| Type group | Package |
|------------|---------|
| Capabilities, theme detection | `internal/term` |
| Chunked reader, windowed buffer | `internal/loader` |
| Source detection, metadata, **Line, Token, LineProvider** | `internal/source` |
| Highlighter (Chroma wrapper) | `internal/highlight` |
| Image / PDF rasterization, graphics protocols | `internal/graphics` |
| Renderer (text, code, image, PDF), **RenderContext, Status, Theme** | `internal/render` |
| Bubble Tea model + viewport | `internal/ui` |
| Config | `internal/config` |
| Search state, matcher | `internal/search` |
| Key bindings | `internal/keys` |

**Type-home rationale**: `Token` lives in `source` (not `highlight`) because
`source.Line.Tokens` is a field; placing Token in `highlight` would force
`source → highlight` and break the acyclic DAG. The Token type is agnostic
to its producer — `highlight` populates it via Chroma, but a future
non-Chroma colorizer could populate the same shape. `Status` and
`RenderContext` live in `render` for the dual reason (avoids
`render → ui` cycle); see `contracts/internal-apis.md`.

The existing skeleton's `internal/reader` and `internal/renderer` are renamed
for clarity (`source` and `render`) and split into the additional packages
listed above as functionality is added; this is a /speckit-tasks concern.

---

## Entities

### ViewerSession

Top-level state held by the Bubble Tea model. There is exactly one per
process invocation.

| Field | Type | Notes |
|-------|------|-------|
| `Source` | `Source` | The thing we're displaying — file or stdin. |
| `Buffer` | `*LineBuffer` | Resident lines + windowing state. |
| `Renderer` | `Renderer` | Strategy chosen by source kind + capabilities. |
| `Viewport` | `bubbles/viewport.Model` | Scroll position, content, dimensions. |
| `Capabilities` | `TerminalCapabilities` | Frozen at startup; refreshed on resize for size only. |
| `Theme` | `Theme` | `dark` / `light`; auto-detected unless overridden. |
| `KeyMap` | `KeyMap` | Active bindings (default ± vim). |
| `Search` | `SearchState` | Inactive when `Query == ""`. |
| `CommandLine` | `CommandLineState` | `:` / `/` / `?` prompt buffer. |
| `Config` | `*Config` | Snapshot of resolved config (CLI > env > file > defaults). |
| `Status` | `render.Status` | Idle / Loading / Streaming / Error. Defined in `internal/render` so renderers can consume it without an `internal/ui` import. |
| `LastError` | `error` | Set when `Status == Error`; rendered in status bar. |
| `Quitting` | `bool` | Set true by `tea.Quit`; final frame uses it. |

**Renderer integration**: `ui.Model.View` builds a `render.RenderContext`
(see `contracts/internal-apis.md`) from the fields above on every frame and
calls `Renderer.Render(ctx)`. The renderer never imports `internal/ui`.

**Lifecycle**:

```text
NEW → READY → STREAMING ⇄ READY → QUITTING
              ↘            ↗
                ERROR  ─────
```

Transitions:

- `NEW → READY`: initial chunk received from the loader; first frame paints.
- `READY → STREAMING`: indicator shown when user scrolls past loaded region.
- `STREAMING → READY`: loader catches up; indicator clears.
- `_ → ERROR`: loader emits unrecoverable error (rare — most errors short-circuit before the viewer launches per FR-013).
- `_ → QUITTING`: q / Esc / SIGINT.

---

### Source

Tagged union (Go: interface + concrete types) describing where bytes come
from.

| Variant | Fields |
|---------|--------|
| `FileSource` | `Path string`, `Size int64`, `ModTime time.Time`, `Detected FileKind` |
| `StdinSource` | `LangHint string` (from `--lang` or `\#!shebang` sniff), `Detected FileKind` |

Validation:

- `FileSource.Path` must resolve to a regular file readable by the process.
  Symlinks are followed; broken symlinks raise FR-013 stdout error.
- Binary detection: first 8KiB scanned for null bytes; if > 1% of bytes are
  control characters outside `\t\r\n\x1b`, treated as binary → FR-013 error.
- `StdinSource` is only constructed when `os.Stdin` is not a TTY.

---

### FileKind

Enumeration; drives renderer selection.

| Value | Detection trigger |
|-------|-------------------|
| `Code` | Extension in known set, or Chroma `lexers.Match` succeeds |
| `Markdown` | Extension `.md`, `.markdown`, `.mdx` |
| `Text` | All other UTF-8/ASCII content |
| `PDF` | Extension `.pdf`, magic bytes `%PDF-` |
| `Image` | Extension in `.png/.jpg/.jpeg/.gif/.bmp/.webp` |
| `Binary` | Any other byte content (rejected → FR-013) |
| `Unknown` | Pre-detection state only |

---

### FileMetadata

Returned alongside content; rendered in the footer per FR-009.

| Field | Type | Notes |
|-------|------|-------|
| `DisplayName` | `string` | `basename(Path)` for files, `<stdin>` for pipes. |
| `Kind` | `FileKind` | |
| `Language` | `string` | Chroma lexer name (e.g., `Go`, `Python`) or `""`. |
| `Size` | `int64` | Bytes; `-1` if unknown (stdin streaming). |
| `LineCount` | `int64` | `-1` if still streaming. |
| `Modified` | `time.Time` | Zero value for stdin. |
| `PageCount` | `int` | Non-zero for PDFs. |
| `Encoding` | `string` | `utf-8` / `latin-1` / `unknown`. |

---

### LineBuffer

Resident-line storage with optional windowing.

| Field | Type | Notes |
|-------|------|-------|
| `Lines` | `[]Line` | Hot region. |
| `BaseLineNumber` | `int64` | First line number stored in `Lines`. |
| `Total` | `int64` | Total line count once known; `-1` while streaming. |
| `Windowed` | `bool` | True when input exceeds `MaxResidentBytes`. |
| `WindowSize` | `int` | Lines kept hot (default 8192). |
| `Source` | `io.ReadSeeker` | Set when `Windowed`; nil for stdin. |
| `Append(lines []Line)` | method | Used by loader chunks. |
| `Slice(start, end int64) []Line` | method | Renderer access; may trigger a paging read in windowed mode. |

Invariants:

- `len(Lines) ≤ WindowSize` whenever `Windowed`.
- `BaseLineNumber + len(Lines) ≤ Total` once `Total ≥ 0`.

---

### Line

| Field | Type | Notes |
|-------|------|-------|
| `Number` | `int64` | 1-indexed for display. |
| `Raw` | `string` | Pre-tokenized text without trailing `\n`. |
| `Tokens` | `[]Token` | Nil until highlighter has run; `Renderer` falls back to `Raw`. |
| `Wrapped` | `[]string` | Cached wrap if `WordWrap == true`; invalidated on resize. |

---

### Token

| Field | Type | Notes |
|-------|------|-------|
| `Type` | `chroma.TokenType` | Reused from Chroma. |
| `Value` | `string` | Substring of `Raw`. |

---

### TerminalCapabilities

Snapshot taken at startup; the `Cols`/`Rows` fields update on resize.

| Field | Type | Notes |
|-------|------|-------|
| `IsTTY` | `bool` | Stdout is a terminal. |
| `Cols`, `Rows` | `int` | Terminal dimensions. |
| `ColorDepth` | `ColorDepth` | `Mono`, `ANSI16`, `ANSI256`, `TrueColor`. |
| `Graphics` | `GraphicsProtocol` | `None`, `Kitty`, `ITerm2`, `Sixel`. |
| `BackgroundLuminance` | `float64` | 0..1; ≥ 0.5 → light. NaN if unknown. |
| `Program` | `string` | `TERM_PROGRAM` value. |
| `Term` | `string` | `TERM` value. |
| `InTmux` | `bool` | True if `TMUX` env set. |

---

### Theme

| Field | Type | Notes |
|-------|------|-------|
| `Name` | `string` | One of `dark`, `light`, or a Chroma style name. |
| `ChromaStyle` | `*chroma.Style` | Resolved style; `dark` → `monokai` default, `light` → `github` default. |
| `Statusbar` | `lipgloss.Style` | |
| `Footer` | `lipgloss.Style` | |
| `LineNumber` | `lipgloss.Style` | |
| `SearchHit` | `lipgloss.Style` | |
| `SearchActive` | `lipgloss.Style` | Currently selected match. |
| `Error` | `lipgloss.Style` | |

---

### Config

Resolved configuration. Defaults shown; CLI/env/file override in that order.

| Field | Type | Default |
|-------|------|---------|
| `Theme` | `string` | `auto` |
| `Vim` | `bool` | `false` |
| `Regex` | `bool` | `false` |
| `WordWrap` | `bool` | `true` |
| `LineNumbers` | `bool` | `true` |
| `TabWidth` | `int` | `4` |
| `MaxResidentBytes` | `int64` | `268435456` (256 MiB) |
| `WindowSize` | `int` | `8192` |
| `HighlightCap` | `int64` | `5242880` (5 MiB) |
| `Graphics` | `string` | `auto` |
| `LangHint` | `string` | `""` |
| `NoColor` | `bool` | `false` (auto-true if `NO_COLOR` set) |
| `MinCols`, `MinRows` | `int` | `80`, `24` (degraded layout below) |

---

### KeyMap

Subset of bindings (full list in [contracts/keys.md](./contracts/keys.md)):

| Action | Default | Vim addition |
|--------|---------|--------------|
| Quit | `q`, `Esc`, `Ctrl-C` | — |
| ScrollUp / Down | `↑` / `↓` | `k` / `j` |
| ScrollLeft / Right | `←` / `→` | `h` / `l` |
| PageUp / Down | `PgUp` / `PgDn` | `Ctrl-B` / `Ctrl-F` |
| HalfPageUp / Down | — | `Ctrl-U` / `Ctrl-D` |
| Home / End | `Home` / `End` | `gg` / `G`, `0` / `$` |
| SearchForward | `/` | — |
| SearchBackward | `?` | — |
| NextMatch / PrevMatch | `n` / `N` | — |
| GoToLine | `:` | — |
| ToggleHelp | `F1` | `?` (when not in search) |
| OpenFile | `o` | — |

---

### SearchState

| Field | Type | Notes |
|-------|------|-------|
| `Query` | `string` | Empty when inactive. |
| `Direction` | `enum{Forward, Backward}` | |
| `Regex` | `bool` | Per-search override of config. |
| `CaseMode` | `enum{Smart, Sensitive, Insensitive}` | |
| `Matches` | `[]Match` | Computed on-demand or eagerly for small files. |
| `CurrentMatch` | `int` | -1 if no match selected. |
| `Wrapped` | `bool` | Latest navigation wrapped around. |
| `Pending` | `bool` | True while a background scan is running. |

`Match` is `{Line int64, Start, End int}`.

---

### CommandLineState

| Field | Type | Notes |
|-------|------|-------|
| `Active` | `bool` | True while `:` / `/` / `?` prompt is open. |
| `Prefix` | `rune` | One of `:`, `/`, `?`. |
| `Buffer` | `string` | Current input. |
| `History` | `[]string` | Per-session, oldest-first. |
| `HistoryCursor` | `int` | -1 = current input; ≥0 = from history. |

---

## Cross-cutting validation rules

| Rule | Source spec | Enforcement |
|------|-------------|-------------|
| Reject binary input with stdout error | FR-013 | `loader.Open` returns sentinel before viewer launch |
| Reject inaccessible files with stdout error | FR-013 | `os.Stat` failure surfaces stderr message and `os.Exit(2)` |
| Restore terminal on signal | FR-015 | `defer term.Restore` in `main`; Bubble Tea's signal handlers |
| Resize handling | FR-014 | `tea.WindowSizeMsg` → viewport `SetWidth/Height` and wrapped-line cache invalidated |
| Minimum terminal `80 × 24` | Q4 / Assumptions | Below threshold: hide footer, single-column status, no graphics; never error |
| Stdin not retained on disk | Assumption | `LineBuffer.Source` is nil for stdin; windowing falls back to "scroll forward only" with a warning |
