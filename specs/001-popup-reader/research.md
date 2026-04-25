<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Phase 0 Research: Popup Reader

**Feature**: 001-popup-reader
**Date**: 2026-04-25
**Inputs**: [spec.md](./spec.md), existing skeleton in `cmd/spy` and `internal/*`

This document resolves every NEEDS-CLARIFICATION-class unknown surfaced by the
Technical Context in [plan.md](./plan.md). Each section follows the
Decision / Rationale / Alternatives format.

---

## R1. Terminal capability detection

**Decision**: Use a layered detection strategy:

1. Honor explicit user override flags / env (`SPY_THEME`, `SPY_GRAPHICS=none|kitty|iterm2|sixel|auto`).
2. Detect interactive TTY via `golang.org/x/term.IsTerminal(int(os.Stdout.Fd()))`. If false, fall back to plain stdout (no alt-screen, no graphics).
3. For graphics protocols, probe in this order, accepting the first that succeeds:
   - **Kitty graphics**: `TERM=xterm-kitty` or `KITTY_WINDOW_ID` set, *or* successful query of `\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\` with response within 100ms.
   - **iTerm2 inline images**: `TERM_PROGRAM=iTerm.app` or `LC_TERMINAL=iTerm2`.
   - **WezTerm**: `TERM_PROGRAM=WezTerm`. WezTerm advertises both Kitty and iTerm2 protocols; prefer Kitty.
   - **Sixel**: send DA1 query `\x1b[c` and check response for `;4;` (sixel attribute) within 100ms.
4. Color depth: trust `COLORTERM=truecolor|24bit` for true color; `TERM` containing `256color` for 256; otherwise 8-color.
5. Dimensions: `term.GetSize(int(os.Stdout.Fd()))`; refresh on `tea.WindowSizeMsg`.

**Rationale**: Pure env-var probing is fast and avoids race conditions, but Kitty
under tmux/screen passthrough does not always set `KITTY_WINDOW_ID`; an active
device-attribute probe with a tight timeout is the documented contract. iTerm2
exports its own marker since 2018 and WezTerm explicitly mimics both. Falling
back to `TERM_PROGRAM` keeps detection deterministic on common shells.

**Alternatives considered**:
- *Always probe by query*: rejected — adds 100–300ms to startup on every launch
  and breaks `SC-001` (open under 100ms for a 100-line file).
- *Use `github.com/gdamore/tcell` capability map*: rejected — Bubble Tea owns
  the IO loop, mixing tcell would require rewriting the existing TUI shell.
- *Trust `TERM` alone*: rejected — `TERM=xterm-256color` is the universal lie.

---

## R2. Image rendering

**Decision**: Encode raw image bytes through three concrete renderers, selected
at runtime by R1's capability detection:

| Capability | Encoder | Library |
|------------|---------|---------|
| Kitty graphics | base64 + `\x1b_Ga=T,f=100;…\x1b\\` chunked at 4096B | hand-rolled (~80 LOC, see `internal/graphics/kitty.go`) |
| iTerm2 inline | `\x1b]1337;File=inline=1;preserveAspectRatio=1:<base64>\x07` | hand-rolled |
| Sixel | `golang.org/x/image` decode → `github.com/mattn/go-sixel` encode | external |
| Fallback | size + dimensions metadata block (current behavior) | n/a |

Source bytes come straight from disk for PNG/JPEG/GIF; for `image/*` types we
already pull dimensions via `image.DecodeConfig`, so we re-open the file when
rendering rather than caching the decoded `image.Image` in memory (large GIFs
otherwise blow the SC-005 500MB ceiling).

**Rationale**: Kitty and iTerm2 protocols are well-specified and each amount to
a few dozen lines of code; pulling in heavyweight wrappers (e.g., `chafa`) just
to re-implement the same thing buys nothing. `go-sixel` is the only mature
pure-Go sixel encoder. Falling back to a metadata block matches FR-013 (graceful
fallback) without misleading the user.

**Alternatives considered**:
- *`github.com/eliukblau/pixterm`*: rejected — emits ANSI half-block "image",
  which is genuinely worse than honest metadata in unsupported terminals and
  worse than native graphics in supported ones.
- *Shell out to `chafa` / `viu`*: rejected — adds an unmanaged binary
  dependency and breaks SC-005 piping semantics.
- *Embed `libvips` via cgo*: rejected — cgo dependency, complicates static
  builds, and we don't need transforms beyond "send what's on disk."

---

## R3. PDF preview

**Decision**: PDF support has two tiers:

1. **Default (always available)**: Use `pdfcpu` to extract page count, title,
   author, and the first page's text, displayed as plain text with the rest of
   the document accessible via page navigation (`]`/`[` or `:N`).
2. **Graphics-capable terminals**: Rasterize the current page to PNG using
   `github.com/gen2brain/go-fitz` (Go bindings for MuPDF) and feed the PNG
   through R2. MuPDF is already a transitive dep of `pdfcpu` for some
   operations but go-fitz vendors its own copy via cgo.

The renderer falls back to text mode automatically if go-fitz is not available
at build time (build tag `nofitz` for static / no-cgo builds). This satisfies
FR-011 ("graceful fallback") in two dimensions: terminal capability *and*
build configuration.

**Rationale**: `pdfcpu` is already in `go.mod` and pure-Go, so the text path is
free. MuPDF is the de-facto standard for high-fidelity PDF rasterization;
alternatives (poppler, mupdf-tools as subprocess) are heavier and not faster.
A build tag lets distributors ship a static binary without sacrificing the
common case.

**Alternatives considered**:
- *Pure-Go rasterization via `unipdf`*: rejected — AGPL-licensed; incompatible
  with project's MIT/Apache-2.0 dual license.
- *Always shell out to `pdftoppm`*: rejected — same dependency-management
  argument as R2's chafa rejection.
- *No graphics for PDF*: rejected — FR-011 explicitly requires preview/render
  in capable terminals.

---

## R4. Progressive loading with concurrency

**Decision**: Implement progressive loading with a `loader.Stream` goroutine
producing `tea.Msg` chunks, owned by a `bufio.Scanner` reading 64KiB at a time:

```text
loader.Open(source) → returns (firstChunk, streamCh, errCh)
firstChunk = enough lines to fill the initial viewport (≥ height*2)
streamCh   = subsequent chunks, posted as tea.Msg via tea.Cmd wrapper
```

The Bubble Tea model receives `chunkLoadedMsg{lines []Line, eof bool}` and
appends to its in-memory buffer (`[]Line` slice of pre-tokenized rows). A
`stopCh` is closed on quit/file-change to cancel the goroutine. Search and
jump-to-line operate against the in-memory buffer; if the user scrolls past
the loaded region we display a "loading…" indicator on the affected lines
until the goroutine catches up.

For files exceeding `MaxResidentBytes` (default 256MiB to stay under SC-005's
500MB working-set ceiling), the loader switches to a windowed mode: it keeps
the current viewport ± 4096 lines hot and discards the rest, re-reading from
the file (which it keeps `os.Open`-ed for the lifetime of the session).

**Backpressure**: `loader.Stream.Updates` is a *bounded* channel (capacity 4
chunks). When the highlighter or the UI consumer falls behind, the loader
goroutine blocks on send — preventing chunks (and the `Line.Raw` strings
they hold) from accumulating in memory beyond ~4 × `InitialChunkLines` ×
average-line-bytes (~256 KiB at defaults). The same applies to the
intermediate channel between `loader` and `highlight.HighlightStream`:
buffer 4. This is the single mechanism that keeps a fast on-disk source
from breaching the 500 MB ceiling while the user idles in the viewer.

**Rationale**: Goroutines + channels match the user's explicit Q1 answer
("goroutines exist for a reason"). Posting through `tea.Cmd` keeps the model
update path single-threaded — Bubble Tea's standard idiom. The chunked
approach lets us hit SC-001 (≤100ms for 100 lines) trivially while still
supporting SC-005 (1GB files under 500MB RAM) via the windowing mode.

**Alternatives considered**:
- *Read entire file synchronously*: rejected by Q1.
- *Memory-map the file with `golang.org/x/exp/mmap`*: rejected — works for
  on-disk files but not for stdin (FR-002), so we'd need two code paths;
  the chunked reader handles both uniformly.
- *Per-line goroutines*: rejected — channel overhead dominates.

---

## R5. Stdin / pipe handling

**Decision**:

1. At startup, check `term.IsTerminal(int(os.Stdin.Fd()))`. If false and no
   file argument was passed, treat stdin as the source.
2. Read stdin into the same `loader.Stream` pipeline; the underlying source
   is just an `io.Reader`, so the chunked loader works without modification.
3. Stdin content is held only in the in-memory ring buffer + sliding window;
   we never write a temp file. (FR-002, plus assumption "no disk writes for
   piped content".)
4. If stdin is a TTY *and* no file argument is given, print usage to stderr
   and exit non-zero — there's nothing to display.
5. Language inference for piped content: try the shebang on the first line,
   then the optional `--lang` / `-l` flag, then content sniffing via Chroma's
   `lexers.Analyse`. If all three fail, render as plain text.

**Rationale**: This mirrors how `bat` handles stdin (the user's stated
reference). Keeping stdin entirely in memory is acceptable because stdin
streams are bounded by `MaxResidentBytes` like any other source; a 2GB
`cat huge.bin | spy` will hit the windowed-mode threshold and switch to
"can't seek backwards in stdin" behavior — see R8.

**Alternatives considered**:
- *Buffer stdin to a temp file*: rejected by FR-002 / spec assumption.
- *Default to JSON when stdin is structured*: rejected — out of scope for v1.

---

## R6. Theme detection (auto / dark / light)

**Decision**: Detect terminal background once at startup using OSC 11:
`\x1b]11;?\x07`. The terminal responds with `\x1b]11;rgb:RRRR/GGGG/BBBB\x07`
within ~50ms in supporting terminals. Compute relative luminance; if < 0.5
treat as dark, else light.

If the query times out or the response is malformed, fall back to
`COLORFGBG` env var (set by rxvt-style terminals). If that is also missing,
default to **dark** — empirically the most common terminal default.

**Defensive parsing requirements** (security: a hostile or buggy terminal
could reply with bytes that the rest of the program then interprets as user
input). Implementation MUST:

1. Read at most 64 bytes of response, with a 50 ms total deadline (`SetReadDeadline`).
2. Validate the response against the strict regex
   `^\x1b\](?:11|10);rgb:[0-9a-fA-F]{1,4}/[0-9a-fA-F]{1,4}/[0-9a-fA-F]{1,4}(?:\x07|\x1b\\)$`.
3. On any of: timeout, length excess, regex mismatch, partial read — discard
   the response entirely, **do not** echo or buffer it for later, fall back
   to `COLORFGBG`. The OSC probe runs *before* alt-screen entry and *before*
   any keyboard input loop, so even discarded bytes have nowhere to leak.
4. Bytes between `\x1b]` and the terminator that include further `\x1b`
   bytes (a malformed reply) are treated as a parse failure; never
   re-injected into stdout.
5. The probe is bypassed entirely when stdout is not a TTY, when `NO_COLOR`
   is set, or when `SPY_THEME` provides an explicit value.

These rules are exercised by `internal/term/theme_test.go` (T060) which
includes adversarial reply fixtures (CSI-embedded, oversize, mid-stream
abort).

The user can always override with `--theme dark|light` or
`SPY_THEME=dark|light`. Theme switching at runtime (FR-004 acceptance #3)
is implemented by sending a new theme into the renderer and re-rendering;
the in-memory token buffer does not need to be re-tokenized — Chroma styles
are applied at render time.

**Rationale**: OSC 11 is the standard query, supported by xterm, kitty,
iTerm2, WezTerm, alacritty, foot, and most modern emulators. Fall-back to
env vars handles the long tail. The 50ms budget keeps us within SC-001.

**Alternatives considered**:
- *Always require explicit theme*: rejected — fails the user-facing promise
  of "adapts automatically" in spec acceptance #1/2.
- *Use `github.com/muesli/termenv`'s `HasDarkBackground`*: ✅ this is
  acceptable and is already a transitive dependency via lipgloss; we will
  use it as the *implementation* of the OSC 11 query and fall-back logic
  rather than re-rolling. Decision stands; library is the means.

---

## R7. Streaming syntax highlighting with Chroma

**Decision**: Tokenize lazily, render eagerly:

1. The loader produces raw `[]Line` (string per line, no styling).
2. A `highlighter` worker consumes lines from a channel, runs the configured
   Chroma lexer in **streaming** mode (`chroma.Coalesce(lexer.TokeniseStreaming(...))`),
   and emits `[]Token` per line.
3. The render path joins tokens into ANSI-styled strings using a
   `formatters.TTY256` or `formatters.TrueColour` formatter selected per R1.

For files larger than `HighlightCap` (default 5MiB), highlighting is disabled
and we render plain text — the cost-benefit of syntax-highlighting a 50MB log
file is poor and risks SC-002 (smooth scroll on 10k-line files). The downgrade
is **surfaced**, not silent: the status bar shows
`highlighting disabled (file > <cap>); set highlight_cap_bytes to override`
on the first paint after detection. The message clears on the next non-error
status update or after 5 s, whichever comes first. Users who want
highlighting on large files raise `highlight_cap_bytes` in their config or
pass `--highlight-cap=<bytes>` (added to `contracts/cli.md`).

**Rationale**: Chroma supports streaming via the iterator interface, but most
of its lexers are stateful per-line; running per line is safe for the vast
majority and an explicit cap protects worst cases. Pre-rendering ANSI strings
once and caching them avoids re-tokenization on scroll (which hits SC-002).

**Alternatives considered**:
- *`github.com/sourcegraph/syntect-server` over HTTP*: rejected — external
  service, adds latency, complicates packaging.
- *`tree-sitter` via cgo*: rejected — heavier dep tree, marginal quality
  improvement for terminal use, breaks pure-Go static builds.

---

## R8. Search and jump-to-line

**Decision**:

- **Search prompt**: `/` opens forward, `?` opens backward; both build on the
  same `SearchState` (query, direction, matches []Position, currentMatch int).
- **Match collection**: linear scan over the in-memory line buffer. For files
  in windowed mode, search only the resident window initially; emit a
  notification line "searching beyond loaded region…" while a background
  goroutine completes the scan.
- **Match navigation**: `n` next, `N` previous, wrap-around with a status-bar
  hint ("search wrapped").
- **Case sensitivity**: smart-case (case-insensitive when the query is
  all-lowercase, case-sensitive otherwise). Toggleable via `\c` / `\C`
  inside the prompt, mimicking vim/less.
- **Regex**: opt-in via `--regex` flag or `\v` prefix in the prompt; default
  is literal substring (faster, no surprise on `(` / `*` etc.).

- **Jump-to-line**: `:N<enter>` or `gg` / `G` (vim mode only). `:0` or `:$`
  resolves to the appropriate edge. Out-of-range jumps clamp to the last
  loaded line and emit a status-bar warning rather than failing.

**Rationale**: This is the same UX shipped by `less`, `bat --pager`, and
`nvim`; users have built muscle memory for it. Smart-case is the most
common compromise between strict and case-insensitive search.

**Alternatives considered**:
- *Fuzzy search (fzf-style)*: rejected — out of scope, adds significant
  ranking infrastructure, deferred to v2.
- *Always regex*: rejected — too many surprise-failures on plain text.

---

## R9. Viewport implementation

**Decision**: Use `github.com/charmbracelet/bubbles/viewport` for the
scrollable region. The current skeleton rolls its own viewport in
`internal/ui/model.go`; replacing it with the upstream component buys us:

- Smooth half-page / page scrolling, mouse wheel support.
- High-water-mark tracking we'd otherwise re-implement.
- Established support for resize (FR-014) and content replacement.

The image / PDF page view is a separate `tea.Model` swapped into the same
slot; the viewport is bypassed when graphics are emitted, since terminal
graphics protocols don't compose with cell-based scrolling.

**Rationale**: Reduces our maintenance surface; `bubbles/viewport` is the
canonical Bubble Tea component for exactly this use case.

**Alternatives considered**:
- *Custom viewport (current code)*: keeps total deps smaller but duplicates
  battle-tested code; rejected.

---

## R10. Signal handling and terminal restoration

**Decision**: Bubble Tea's `tea.Program.Run()` already installs SIGINT / SIGTERM
handlers, restores the terminal on panic, and exits the alt-screen on `tea.Quit`.
Two additional measures are needed, and **both must run from the same `defer`
chain in `main()` so they fire on panic, not just on graceful `tea.Quit`**:

1. Capture terminal state at startup with `term.Restore()` (returns a closure).
2. Capture a `cleanupGraphics func()` at startup that, when invoked, writes
   the protocol-specific "delete all images" sequence to stdout — Kitty
   (`\x1b_Ga=d,d=A;\x1b\\`) is the load-bearing case; iTerm2 and sixel
   self-clean and the closure is a no-op for them; `GraphicsNone` is also a
   no-op. The closure is idempotent.
3. In `main()` after capability detection but before any rendering:
   ```go
   restore := term.Restore()
   cleanupGraphics := graphics.CleanupFunc(caps.Graphics)
   defer restore()           // outer: terminal modes/cursor/echo
   defer cleanupGraphics()   // inner: graphics cleanup runs first (LIFO)
   ```
   Both fire on `tea.Quit`, on `os.Exit` from a signal, and — critically —
   on panic, including a cgo `go-fitz` panic mid-render that bypasses Bubble
   Tea's normal teardown. Re-emitting the cleanup escapes from `tea.Quit` is
   redundant and harmless once these defers are in place.

**Rationale**: FR-015 demands clean exit on signals; the stock Bubble Tea
behavior covers `tea.Quit` and the SIGINT/SIGTERM happy path, but a panic
inside a third-party renderer skips `tea.Quit` entirely. Routing graphics
cleanup through `defer` (not through `tea.Cmd`) is the only way to guarantee
it runs in every exit path. Ghost Kitty images persisting across sessions
under tmux is a real, observable failure mode users will report as a bug.

**Alternatives considered**:
- *Implement our own signal loop*: rejected — fights Bubble Tea's runtime.
- *Cleanup only on `tea.Quit`*: rejected — silently fails on panic; the very
  case where users notice and complain.

---

## R11. Configuration loading

**Decision**:

- Config sources, in precedence (high → low):
  1. CLI flags (`--theme`, `--vim`, `--regex`, `--no-color`, `--lang`, etc.)
  2. Env vars (`SPY_THEME`, `SPY_VIM`, `SPY_GRAPHICS`, `NO_COLOR`)
  3. Per-user config: `$XDG_CONFIG_HOME/spy/config.toml` (default
     `$HOME/.config/spy/config.toml`)
  4. Compiled defaults
- Format: TOML via `github.com/BurntSushi/toml` (small, no transitive deps,
  permissively licensed). Sample shipped at `examples/config.toml`.
- The loader is best-effort: missing config file is not an error;
  malformed config emits a single warning to stderr and uses defaults.

**Rationale**: TOML matches the existing `_typos.toml`/`REUSE.toml` style
already in the repo; XDG layering is the Linux norm and works on macOS.
Honoring `NO_COLOR` is required by the de-facto convention
(<https://no-color.org>).

**Alternatives considered**:
- *YAML*: rejected — heavier, ambiguous indentation surprises users.
- *JSON*: rejected — comments forbidden, painful to edit by hand.
- *No config file*: rejected — FR-004 acceptance #3 requires persistent
  override across launches.

---

## R12. Keybinding model (default vs vim)

**Decision**: A `KeyMap` struct with named bindings; two presets:

- **Default (arrow keys primary)**: ↑/↓/←/→, PgUp/PgDn, Home/End, `q`/Esc
  to quit, `/` to search, `:` for command (jump-to-line), `n`/`N` for next/prev
  match, `?` for help.
- **Vim (`--vim` or `vim_mode = true`)**: adds `h`/`j`/`k`/`l`, `gg`/`G`,
  `Ctrl-D`/`Ctrl-U`, `Ctrl-F`/`Ctrl-B`, `0`/`$`. Arrow keys remain functional;
  vim mode is additive, not exclusive.

Bindings are defined via `bubbles/key`'s `key.Binding` so help text is
auto-generated.

**Rationale**: Matches the explicit Q3 answer ("arrow keys with optional vim
mode"). Additive vim mode means power users get their bindings without
penalising keyboard-shy users.

**Alternatives considered**:
- *Mode-exclusive*: rejected — punishes users who mix idioms.

---

## R13. Constitutional template (project-level)

**Observation**: `.specify/memory/constitution.md` contains placeholder
template content (`[PROJECT_NAME]`, `[PRINCIPLE_1_NAME]`, etc.) and has not
been ratified for this project. The `/speckit-constitution` workflow has not
been run.

**Decision for this plan**: Proceed with a Constitution Check stub that
declares "no ratified constitution; using sensible Go defaults aligned with
[~/.claude/rules/golang/](~/.claude/rules/golang/) and the project's
existing `DEVELOPMENT.md`". Recommend the user run `/speckit-constitution`
before later features so future plans have real gates to evaluate against.

**Rationale**: Blocking on an empty template would prevent any feature
planning; we explicitly call out the gap so it's not silently hidden.

---

## Open issues deferred to /speckit-tasks or implementation

- **PDF text extraction quality**: pdfcpu's text extraction is layout-naive.
  If the rendered text proves unreadable for typical PDFs, consider switching
  the default text extractor to go-fitz's `Page.Text()`.
- **Mouse support**: scroll wheel comes for free with bubbles/viewport;
  click-to-position is deferred to v2.
- **Telemetry / observability**: deferred per Q4 clarification — no logging
  unless `--debug` adds it.
