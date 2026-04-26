<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Keybinding Contract

**Spec**: 001-popup-reader (FR-005, FR-006, FR-007, FR-008)

Two presets:

- **Default** — arrow keys + named keys; usable without learning new bindings.
- **Vim** — additive (`--vim` or `vim_mode=true`); default bindings remain active.

Bindings render in the help overlay (`F1` / `?`) using `bubbles/key` so any
remap automatically updates the help text.

## Navigation

| Action | Default | Vim addition |
|--------|---------|--------------|
| Scroll up one line | `↑` | `k` |
| Scroll down one line | `↓` | `j` |
| Scroll left one column | `←` | `h` |
| Scroll right one column | `→` | `l` |
| Page up | `PgUp` | `Ctrl-B` |
| Page down | `PgDn` / `Space` | `Ctrl-F` |
| Half page up | — | `Ctrl-U` |
| Half page down | — | `Ctrl-D` |
| Go to top | `Home` | `gg` |
| Go to bottom | `End` | `G` |
| Beginning of line | `Home` (when on line) | `0` |
| End of line | `End` (when on line) | `$` |
| Next page (PDF) | `]` | — |
| Previous page (PDF) | `[` | — |

## Search

| Action | Default | Vim addition |
|--------|---------|--------------|
| Open forward search | `/` | — |
| Open backward search | `?` | — |
| Submit search | `Enter` | — |
| Cancel search | `Esc` | — |
| Next match | `n` | — |
| Previous match | `N` | — |
| Toggle case sensitivity (in prompt) | `\c` / `\C` | — |
| Force regex (in prompt) | `\v` | — |
| Force literal (in prompt) | `\V` | — |

## Commands

| Action | Default | Vim addition |
|--------|---------|--------------|
| Open command line | `:` | — |
| Submit command | `Enter` | — |
| Cancel command | `Esc` | — |
| Recall previous command | `↑` (in prompt) | — |
| Recall next command | `↓` (in prompt) | — |

Commands recognized at the `:` prompt:

| Command | Effect |
|---------|--------|
| `:N` (integer) | Jump to line `N`; clamps to last loaded line. |
| `:0` | Jump to line 1. |
| `:$` | Jump to last line. |
| `:set vim` / `:set novim` | Toggle vim mode for this session. |
| `:set theme dark|light|auto` | Switch theme. |
| `:open <path>` | Replace current source with a new file. |
| `:q` / `:quit` | Quit (alias of `q` / `Esc`). |

Unknown commands surface a status-bar warning; they never crash the viewer.

## Application

| Action | Default | Vim addition |
|--------|---------|--------------|
| Quit | `q` / `Esc` / `Ctrl-C` | `ZZ`, `:q` |
| Toggle help | `F1` | — |
| Open file dialog | `o` | — |
| Reload current source | `Ctrl-R` / `r` | — |
| Toggle line numbers | `Ctrl-L` | — |
| Toggle word wrap | `Ctrl-W` | — |

In vim, `?` opens a backward search; in spy we keep that meaning so
`?foo<Enter>` works the way muscle memory expects. `?` is therefore
*not* a vim help-toggle binding — `F1` remains the only help toggle in
both default and vim modes (Copilot review PR#9 round-2 #5: an earlier
draft of this contract listed `?` as a vim addition for help, which
conflicted with `ActionSearchBackward`; the implementation has always
followed the search-backward semantics).

## Conflict-resolution rules

- Inside a `/` / `?` search prompt: `?` is a literal character, not the help
  toggle.
- Inside a `:` command prompt: `:` is a literal character.
- `Esc` always closes any active overlay before quitting; pressing `Esc` with
  no overlay open quits.

## Override

A user may remap any binding via the config file:

```toml
[keys]
quit          = ["q", "esc", "ctrl+c"]
search_forward = ["/"]
go_to_line    = [":"]
```

Unrecognized keys log a warning to `--debug` and are silently dropped.

## Help overlay

`F1` (or `?` outside a prompt) toggles a centered Lip Gloss-styled overlay
listing the active bindings, generated from the `KeyMap`. The overlay
respects the active theme. `Esc` or any unbound key closes it.
