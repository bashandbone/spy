<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Quickstart: Popup Reader Validation

**Spec**: 001-popup-reader
**Audience**: implementer / reviewer verifying the feature end-to-end.

This walk-through executes every functional requirement at least once. Run it
against the `001-popup-reader` branch after `/speckit-implement` finishes.
Each step lists which `FR-` / `SC-` / acceptance scenario it covers.

## 0. Prerequisites

- Go 1.26.2 (matches `go.mod`).
- A modern terminal (xterm-compatible, ≥ 80×24).
- For graphics tests (steps 6–7): Kitty, iTerm2, or WezTerm (or a terminal
  with sixel support such as `foot`).
- A scratch directory with sample assets. The repo ships a setup script
  (T005) that materializes everything below from local fixtures under
  `tests/e2e/fixtures/`, so the walkthrough does not require network
  access:

```bash
bash tests/e2e/setup.sh   # populates /tmp/spy-fixtures from local fixtures
```

If that script is unavailable (e.g., before T005 lands), the equivalent
manual setup is:

```bash
mkdir -p /tmp/spy-fixtures
echo 'package main\nfunc main(){println("hi")}' > /tmp/spy-fixtures/hello.go
seq 1 10000 > /tmp/spy-fixtures/big.txt
# Network fallback for PDF/image fixtures (use only if local copies are absent):
curl -sL https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf \
  -o /tmp/spy-fixtures/dummy.pdf
curl -sL https://www.w3.org/Graphics/PNG/iso_8859-1.png \
  -o /tmp/spy-fixtures/iso.png
```

## 1. Build

```bash
go build -o bin/spy ./cmd/spy
./bin/spy --version
```

Expected: `spy <version>` printed on stdout, exit 0.
Covers: build sanity.

## 2. Open a code file

```bash
./bin/spy /tmp/spy-fixtures/hello.go
```

Expected:
- Alt-screen launches in under 100ms.
- `hello.go` content visible with Go syntax highlighting.
- Footer shows `hello.go | 3 lines | Line 1`.
- `↓` / `j` (vim mode is off by default but vim keys still scroll, see below)
  scrolls; `Esc` or `q` exits cleanly back to your prior shell prompt with no
  visible artifacts.

Covers: FR-001, FR-003, FR-008, FR-009, SC-001, SC-007, US1.

## 3. Pipe input

```bash
git diff HEAD~ | ./bin/spy
```

Expected:
- Alt-screen displays the diff with diff-style highlighting.
- Footer reads `<stdin> | … lines | Line 1`.
- No new files appear under `/tmp/`.
- `Esc` exits and the shell shows your previous prompt unchanged.

Covers: FR-002, US5, SC-011, no-disk-write assumption.

## 4. Search and jump-to-line

With `/tmp/spy-fixtures/big.txt` open:

```bash
./bin/spy /tmp/spy-fixtures/big.txt
```

In the viewer:

1. Press `/`, type `9999`, press `Enter`. The viewport jumps to the line
   containing `9999` and that match is highlighted.
2. Press `n`. There are no further matches — status bar shows
   `search wrapped` and the same match remains highlighted.
3. Press `Esc` to clear search.
4. Press `:`, type `1`, press `Enter`. The viewport jumps to line 1.
5. Press `:`, type `$`, press `Enter`. The viewport jumps to the last line
   and the footer shows the END indicator.

Covers: FR-006, FR-007, US1 acceptance #4, US2 acceptance #2/#3/#4, SC-003.

## 5. Vim mode

```bash
./bin/spy --vim /tmp/spy-fixtures/big.txt
```

In the viewer:

- `j` / `k` scroll one line; `Ctrl-D` / `Ctrl-U` half-page; `gg` to top, `G`
  to bottom.
- `/`, `n`, `N` work as in step 4.

Covers: FR-005 (default vs vim), Q3 clarification.

Then verify default mode preserves arrow keys:

```bash
./bin/spy /tmp/spy-fixtures/big.txt
```

`↑` / `↓` / `PgUp` / `PgDn` / `Home` / `End` should all work.

## 6. Theme detection and override

Run on a light terminal:

```bash
./bin/spy /tmp/spy-fixtures/hello.go
```

Expected: highlighting uses the light theme; the footer/status bar is
readable.

Run with override:

```bash
./bin/spy --theme dark /tmp/spy-fixtures/hello.go
```

Expected: dark theme used regardless of terminal background. Setting
`SPY_THEME=light` and re-running with no flag picks up the env override.

Covers: FR-004, US3 acceptance #1/#2/#3, SC-004.

## 7. Image rendering

In a Kitty / iTerm2 / WezTerm window:

```bash
./bin/spy /tmp/spy-fixtures/iso.png
```

Expected: the PNG renders inline. `Esc` exits.

In a non-graphics terminal (e.g., GNOME terminal):

```bash
./bin/spy /tmp/spy-fixtures/iso.png
```

Expected: a metadata block ("file name, dimensions, size, fallback message")
displays. No corrupt characters or escape garbage.

Covers: FR-010, US4 acceptance #2/#3, SC-009.

## 8. PDF preview

In a graphics-capable terminal:

```bash
./bin/spy /tmp/spy-fixtures/dummy.pdf
```

Expected:
- First page rasterizes inline.
- `]` advances to next page; `[` goes back.
- Footer shows `dummy.pdf | Page 1/N`.

In a non-graphics terminal:

Expected: text extraction of page 1 displays with footer indicator;
fallback message visible if the page is graphics-only.

Covers: FR-011, US4 acceptance #1, SC-010.

## 9. Large file handling

```bash
yes "the quick brown fox" | head -c 200M > /tmp/spy-fixtures/large.txt
./bin/spy /tmp/spy-fixtures/large.txt
```

Expected:
- Initial viewport paints in under 100ms.
- Scrolling near the start is smooth.
- Status bar shows `streaming…` indicator briefly when scrolling far ahead,
  then clears.
- Resident memory (check `RES` in `top` / `htop`) stays under 500 MB.

Covers: FR-012, Q1 clarification, SC-005.

## 10. Error path: file not found

```bash
./bin/spy /tmp/does-not-exist.txt; echo "exit=$?"
```

Expected:
- Single line on stderr: `spy: cannot open: /tmp/does-not-exist.txt: no such file or directory`.
- No alt-screen launched (terminal state unchanged).
- Exit code `3`.

Covers: FR-013, Q2 clarification.

## 11. Error path: binary file

```bash
./bin/spy ./bin/spy; echo "exit=$?"
```

Expected: stderr `spy: binary file: ./bin/spy: refusing to render binary content`,
exit code `4`, no alt-screen.

Covers: FR-013, binary-file assumption.

## 12. Resize handling

Open a long file (`big.txt`), then resize the terminal window. Viewport
should reflow without losing scroll position; line numbers should remain
aligned; status bar should update its width.

Covers: FR-014, SC-008, US1 edge case.

## 13. Signal handling

While the viewer is open, press `Ctrl-C`. The terminal should return
cleanly: cursor visible, normal screen restored, prompt redrawn.

Run again, send SIGTERM from another shell:

```bash
pkill -TERM spy
```

Same expected outcome.

Covers: FR-015.

## 14. Minimum-size degradation

Resize the terminal to 60×20 (below the documented minimum) before
launching. Open a code file. Expected:

- No crash.
- Footer collapses to a single short line; help bar hidden.
- Body still readable; horizontal scroll works.

Covers: Q4 clarification, "graceful degradation" assumption.

## 14b. Empty input + file/stdin precedence

```bash
: > /tmp/spy-fixtures/empty.txt
./bin/spy /tmp/spy-fixtures/empty.txt
```

Expected: viewer launches; body shows `(empty)`; footer
`empty.txt | 0 lines | Line 0`; `q` dismisses with exit 0.

```bash
echo "this should be ignored" | ./bin/spy /tmp/spy-fixtures/hello.go
```

Expected: viewer shows `hello.go` content (NOT the piped text). The piped
input is silently dropped.

Covers: `contracts/cli.md` resolution table, empty-input edge case.

## 15. Configuration file override

```bash
mkdir -p ~/.config/spy
cat > ~/.config/spy/config.toml <<'TOML'
theme    = "dark"
vim_mode = true
TOML
./bin/spy /tmp/spy-fixtures/hello.go
```

Expected: launches with vim bindings active and dark theme regardless of
terminal background. Remove the file when done.

Covers: configuration assumption, contracts/config.md.

---

## Done criteria

- [ ] All 15 steps pass on the implementer's machine.
- [ ] Steps 7 and 8 pass on at least one Kitty / iTerm2 / WezTerm install
      *and* at least one non-graphics terminal.
- [ ] No leftover files under `/tmp/spy-fixtures/` retained in the
      repository (these are scratch only).
