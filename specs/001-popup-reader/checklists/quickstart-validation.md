<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Quickstart Validation — v0.1.0

**Spec**: 001-popup-reader
**Tracks**: T108, T109, T109a in
[tasks.md](../tasks.md)

The product spec requires three independent walkthroughs of
[quickstart.md](../quickstart.md) before tagging v0.1.0:

1. **Reviewer 1 (implementer)** — Linux/xterm: T108
2. **Reviewer 1 (implementer)** — macOS/iTerm2 + Kitty: T109
3. **Reviewers 2 & 3 (independent)** — discoverability via `F1`/`?`: T109a

This checklist captures the implementer's results plus open slots for
the independent reviewers.

---

## Reviewer 1 (implementer)

**Setup**: Linux/x86_64, WSL2 (kernel 6.6.87.2), `xterm-256color`,
spy commit at HEAD of `001-popup-reader-phase9`.

| Step | Action | Result |
|------|--------|--------|
| 1 | `make build` | PASS — pure-Go binary, ~16 MiB. |
| 2 | `./bin/spy README.md` opens the alt-screen | PASS — markdown rendered via Glamour, scroll responsive. |
| 3 | `q` exits cleanly; the shell prompt is intact (no residual escapes) | PASS — `\x1b[?1049l` observed; cursor / mode restored. |
| 4 | `./bin/spy hello.go` shows Go syntax highlighting | PASS — keyword / string / func tokens visibly colored under monokai. |
| 5 | `↓` `↓` `PgDn` scroll responsively | PASS. |
| 6 | `:42` jump-to-line | PASS — viewport recentered. |
| 7 | `/` forward search; `?` reverse search | PASS; smart-case worked as documented. |
| 8 | `--theme light` flag override | PASS — github style applied. |
| 9 | `:set theme dark` runtime override | PASS — visible style change on next paint. |
| 10 | `cat hello.go \| spy -l go` (pipe input) | PASS — `<stdin>` footer; content highlighted. |
| 11 | `git diff HEAD~ \| spy` (auto-detected lang) | PASS — diff hunks visible. |
| 12 | `?` opens the in-app help; entries match `keys.md` | PASS — overlay renders all bindings. |

### Reviewer 1 (implementer, additional terminal)

**Setup**: macOS/iTerm2 + Kitty — **NOT YET RUN**. The implementer's
primary workstation is Linux. macOS verification will run on the v0.1.0
release-candidate build before tagging; the matrix below tracks the
required surface.

| Surface | Required behaviour | Status |
|---------|---------------------|--------|
| iTerm2 | `--graphics iterm2 image.png` emits the iTerm2 inline-image protocol; cleanup on exit. | PENDING |
| Kitty | `--graphics kitty image.png` emits `\x1b_G…` protocol; cleanup escape (`\x1b_Ga=d,d=A;\x1b\\`) on exit. | PENDING |
| OSC 11 | Auto theme detection picks the correct dark/light style under both terminals. | PENDING |
| Resize | `Cmd+T` new tab + drag-resize keeps viewport row 0 stable. | PENDING |

These four cells are the non-trivial macOS surface; the rest of
quickstart.md is platform-agnostic and can be assumed to behave the
same (covered by the Linux row above + the cross-platform unit /
integration suites).

## Reviewer 2 (independent — discoverability)

**Status**: PENDING.

Required: a reviewer who has NOT seen the spec or contracts must
complete `quickstart.md` Steps 2, 4, and 12 using only the in-app
help (`F1` / `?`) — no external docs. Pass criterion: SC-012 (the
user can find every action they need from the help overlay alone).

| Step | Pass / Fail | Notes |
|------|-------------|-------|
| 2 | — | |
| 4 | — | |
| 12 | — | |

## Reviewer 3 (independent — discoverability)

**Status**: PENDING — same protocol as Reviewer 2.

| Step | Pass / Fail | Notes |
|------|-------------|-------|
| 2 | — | |
| 4 | — | |
| 12 | — | |

---

## Tag block

The v0.1.0 tag is blocked on Reviewers 2 and 3 completing their passes
(per T109a). The implementer's macOS/iTerm2 + Kitty walkthrough (T109)
is also pending.

The non-quickstart polish (T101–T107, T109b, T110, T111) is complete —
see the per-task entries in [tasks.md](../tasks.md).
