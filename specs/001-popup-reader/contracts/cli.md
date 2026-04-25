<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# CLI Contract

**Binary**: `spy`
**Spec**: 001-popup-reader

## Synopsis

```text
spy [OPTIONS] [FILE]
spy [OPTIONS] -      # explicit stdin
... | spy [OPTIONS]  # implicit stdin when stdin is not a TTY
```

## Positional argument

| Token | Meaning |
|-------|---------|
| `FILE` | Path to a regular file. Symlinks are followed. Mutually exclusive with stdin. |
| `-` | Force read from stdin even if `os.Stdin` is a TTY (will block until EOF). |
| *(none)* | If stdin is not a TTY, read stdin. Otherwise print usage to stderr and exit 2. |

## Flags

| Flag | Short | Type / values | Default | Description |
|------|-------|---------------|---------|-------------|
| `--help` | `-h` | bool | false | Print usage on stdout, exit 0. |
| `--version` | `-V` | bool | false | Print `spy <version>` on stdout, exit 0. |
| `--theme` | | `auto`\|`dark`\|`light`\|`<chroma-style>` | `auto` | Override theme detection. |
| `--vim` | | bool | false | Enable additive vim keybindings. |
| `--lang` | `-l` | string | `""` | Force a Chroma lexer name (e.g., `go`, `python`). |
| `--regex` | | bool | false | Treat search queries as regex by default. |
| `--no-color` | | bool | false | Disable ANSI color (alias for `NO_COLOR=1`). |
| `--graphics` | | `auto`\|`none`\|`kitty`\|`iterm2`\|`sixel` | `auto` | Override graphics protocol detection. |
| `--no-line-numbers` | | bool | false | Hide line numbers. |
| `--no-wrap` | | bool | false | Disable soft-wrap; use horizontal scrolling instead. |
| `--config` | | path | `$XDG_CONFIG_HOME/spy/config.toml` | Override config file path. |
| `--no-config` | | bool | false | Skip loading any config file. |
| `--debug` | | path | `""` | Write a debug log to the given file (no logging when unset). |

Flag parsing uses Go's `flag` package conventions (`--flag=value` or
`--flag value`); both single- and double-dash forms accepted for long flags.

## Environment variables

| Variable | Type | Effect |
|----------|------|--------|
| `SPY_THEME` | `auto`\|`dark`\|`light`\|`<style>` | Same as `--theme`; flag wins. |
| `SPY_VIM` | `0`\|`1`\|`true`\|`false` | Same as `--vim`. |
| `SPY_GRAPHICS` | matches `--graphics` | Same as `--graphics`. |
| `NO_COLOR` | any non-empty | Disables color (no-color.org convention). |
| `XDG_CONFIG_HOME` | path | Config file lookup base; falls back to `$HOME/.config`. |
| `COLORTERM` | `truecolor`\|`24bit` | Used during capability detection. |
| `TERM`, `TERM_PROGRAM`, `LC_TERMINAL` | string | Used during capability detection. |
| `KITTY_WINDOW_ID`, `TMUX` | string (presence) | Used during capability detection. |

## Stdin behavior

- Stdin is consumed only when no `FILE` is given and `os.Stdin` is not a TTY,
  or when `-` is passed explicitly.
- Stdin content is never persisted to disk.
- If stdin is consumed, the alt-screen is still entered on `os.Stdout` (which
  must be a TTY); if stdout is also not a TTY, `spy` exits with the input
  copied verbatim to stdout (degenerate "cat" behavior) and exit code 0.

### Resolution table when both a file argument and non-TTY stdin are present

`spy` resolves source ambiguity deterministically:

| `FILE` arg | `-` arg | stdin is TTY | stdout is TTY | Source used | Stdin handling |
|------------|---------|--------------|---------------|-------------|----------------|
| present    | no      | yes          | yes           | FILE        | ignored |
| present    | no      | no           | yes           | FILE        | ignored (drained at exit, never read) |
| present    | yes     | —            | yes           | usage error | exit 2 (`-` and FILE are mutually exclusive) |
| absent     | no      | no           | yes           | stdin       | streamed |
| absent     | no      | yes          | yes           | none        | exit 2 (usage printed) |
| absent     | yes     | —            | yes           | stdin       | blocks on stdin until EOF/Ctrl-D |
| absent     | no      | no           | no            | stdin       | degenerate cat to stdout, exit 0 |

When a file argument is present, stdin is **never** read, even if it is
non-TTY. This avoids a class of accidents where a piped command's output
silently overrides an explicit file argument.

### Empty input

A 0-byte file or 0-byte stdin produces an alt-screen viewer showing the
single line `(empty)` styled with `Theme.Footer`, footer reading
`<displayname> | 0 lines | Line 0`, exit 0 on dismiss. Not an error.

## Stdout / stderr / exit codes

| Stream | Use |
|--------|-----|
| stdout (TTY) | Alt-screen TUI frames |
| stdout (non-TTY) | Verbatim file content (degenerate mode) |
| stderr | All error messages, debug warnings, deprecation notes |

Exit codes:

| Code | Meaning |
|------|---------|
| `0` | Success (viewer dismissed normally, or degenerate mode wrote content). |
| `1` | Generic error not covered below. |
| `2` | Usage error: bad flag, missing argument when stdin is a TTY, etc. |
| `3` | I/O error: file not found, permission denied, broken symlink. |
| `4` | Unsupported content: binary file, malformed PDF, decode failure. |
| `5` | Terminal initialization error: no TTY when one is required. |
| `130` | Interrupted by SIGINT. |
| `143` | Terminated by SIGTERM. |

## Error message format (stderr)

Single line, prefixed with the program name (FR-013 requires errors visible to
shell users):

```text
spy: <short reason>: <detail>
```

Examples:

```text
spy: cannot open: ./missing.txt: no such file or directory
spy: unsupported format: ./image.heic: HEIC images are not supported
spy: binary file: ./bin/spy: refusing to render binary content
```

## Usage output (`--help`)

Stable across patch releases; backward-incompatible only with major version
bumps. Exact format produced by Go's `flag.Usage`. Example shape:

```text
Usage: spy [OPTIONS] [FILE]
A focused popup viewer for text, code, PDFs, and images.

Options:
  -h, --help              show this help and exit
  -V, --version           show version and exit
      --theme=<value>     dark|light|auto|<chroma-style>  (default: auto)
      --vim               enable vim keybindings
  -l, --lang=<name>       force language for highlighting
      --regex             treat searches as regex
      --no-color          disable color (alias for NO_COLOR=1)
      --graphics=<value>  auto|none|kitty|iterm2|sixel    (default: auto)
      --no-line-numbers   hide line numbers
      --no-wrap           disable soft wrap
      --config=<path>     config file path
      --no-config         do not load any config file
      --debug=<path>      write debug log to path

Examples:
  spy README.md
  cat main.go | spy -l go
  git diff HEAD~ | spy
```

## Compatibility notes

- Flag parsing is deliberately conservative: `bat`-style short combined flags
  (e.g., `-pP`) are not supported.
- The contract is the *interface*, not the implementation. Internal exit codes
  may add finer detail (e.g., `4-pdf`, `4-binary`) but the documented codes
  remain stable.
