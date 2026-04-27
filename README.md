<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# spy

A keyboard-driven popup reader for the terminal — `bat` with a real
viewport. `spy` opens any file (or piped content) in the alt-screen,
applies syntax highlighting, and lets you scroll, search, and jump to a
line without leaving the shell. It exits cleanly on `q` and leaves the
underlying terminal exactly as it found it.

## Features

- Syntax highlighting for 100+ languages via [Chroma](https://github.com/alecthomas/chroma)
- Scroll, page, and `:N` jump-to-line; `/` forward search and `?` reverse search
- Auto theme detection (background luminance via OSC 11) with dark/light overrides
- PDF and image viewers — Kitty, iTerm2, and Sixel graphics protocols supported
  (PDF rasterization requires the `-tags fitz` build)
- Pipe input: `git diff | spy`, `cat hello.go | spy -l go`, `… | spy | cat`
- Soft-wrap and horizontal scrolling; line-numbers gutter
- Configurable via `$XDG_CONFIG_HOME/spy/config.toml`; everything overridable
  by CLI flags and environment variables
- Optional vim-style keybindings (`--vim` or `vim_mode=true`)

## Install

Pre-built binaries will follow the v0.1.0 tag. Until then, build from
source:

```bash
git clone https://github.com/knitli/spy.git
cd spy
make build         # default pure-Go build → bin/spy
make build-fitz    # add cgo PDF rasterization → bin/spy-fitz
```

`spy` requires Go 1.26 or later. The `-tags fitz` build additionally
needs a working C toolchain and `mupdf` headers; the default build has no
cgo dependency and produces a single static binary.

## Usage

```bash
spy hello.go                # open a file
spy README.md               # markdown — rendered via Glamour
spy --theme light invoice.pdf
spy --lang go -                # explicit stdin, hint Go lexer
git diff HEAD~ | spy        # pipe input — language auto-detected
echo content | spy | cat    # degenerate-cat: verbatim, exit 0
```

`spy --help` prints the full flag list. The most common ones:

| Flag | Effect |
|------|--------|
| `--theme=auto\|dark\|light\|<chroma-style>` | Override theme detection. |
| `--vim` | Additive vim keybindings. |
| `--lang=<chroma-lexer>` (`-l`) | Force a Chroma lexer (e.g., `go`, `python`). |
| `--graphics=auto\|none\|kitty\|iterm2\|sixel` | Override graphics protocol. |
| `--no-line-numbers` / `--no-wrap` / `--no-color` | Disable display features. |
| `--highlight-cap=<bytes>` | Skip syntax highlighting above this size (default 5 MiB). |
| `--config=<path>` / `--no-config` | Override or skip the config file. |

### Keybindings

The defaults work without learning anything new. Pressing `F1` or `?`
opens an in-app help overlay generated from the live key table — remaps
update the overlay automatically.

| Action | Default | Vim |
|--------|---------|-----|
| Scroll | `↑` `↓` `←` `→` | `k` `j` `h` `l` |
| Page | `PgUp` / `PgDn` / `Space` | `Ctrl-B` / `Ctrl-F` / `Ctrl-D` / `Ctrl-U` |
| Top / bottom | `Home` / `End` | `gg` / `G` |
| Search forward / back | `/` / `?` | — |
| Next / prev match | `n` / `N` | — |
| Command-line (`:N`, `:set theme dark`) | `:` | — |
| Quit | `q`, `Esc`, `Ctrl-C` | — |

The full key contract lives in
[specs/001-popup-reader/contracts/keys.md](specs/001-popup-reader/contracts/keys.md).

## Multiplexer integration

### tmux (automatic)

When spy is run inside a tmux session it automatically re-launches itself
via `tmux display-popup`, creating a full-screen floating overlay that covers
all panes. When you quit (`q`), the popup closes and you are returned to your
exact prior state. Requires **tmux 3.2 or later** (2020).

```bash
spy README.md      # inside tmux: opens a full-screen popup automatically
```

Pass `--no-popup` to skip this and run spy in the current pane instead:

```bash
spy --no-popup README.md
```

To disable it permanently, add an alias to your shell config:

```bash
alias spy='spy --no-popup'
```

### WezTerm

WezTerm does not have a `display-popup` equivalent, but you can get a similar
experience with a shell function that opens spy in a new window:

```bash
# Add to ~/.bashrc / ~/.zshrc
spypop() {
  wezterm cli spawn --new-window -- spy "$@"
}
```

For a tighter integration, add a WezTerm key binding that opens the current
selection in spy:

```lua
-- wezterm.lua
local wezterm = require 'wezterm'
local act = wezterm.action

config.keys = {
  {
    key = 'o',
    mods = 'CTRL|SHIFT',
    action = act.SpawnCommandInNewTab {
      args = { 'spy', wezterm.UNKNOWN_FILENAME }, -- replace with selection logic
    },
  },
}
```

### Kitty

Kitty's remote-control API supports true overlay windows. Enable it in
`kitty.conf`:

```ini
# kitty.conf
allow_remote_control yes
listen_on unix:/tmp/kitty
```

Then define a shell function or alias:

```bash
# Add to ~/.bashrc / ~/.zshrc
spypop() {
  kitty @ launch --type=overlay --copy-env spy "$@"
}
```

`--type=overlay` creates a floating window over the active window — the
closest Kitty analogue to tmux's `display-popup`.

## Configuration

Drop a TOML file at `$XDG_CONFIG_HOME/spy/config.toml` (falling back to
`$HOME/.config/spy/config.toml`). Every option is also available as a
flag and an environment variable; precedence is **flags > env > file >
compiled defaults**. See
[examples/config.toml](examples/config.toml) for a fully-annotated
template and
[specs/001-popup-reader/contracts/config.md](specs/001-popup-reader/contracts/config.md)
for the schema.

## Behavior contracts

The CLI surface, exit codes, stdin handling, and resolution table are
specified in
[specs/001-popup-reader/contracts/cli.md](specs/001-popup-reader/contracts/cli.md).
Internal package APIs and their guarantees are documented in
[specs/001-popup-reader/contracts/internal-apis.md](specs/001-popup-reader/contracts/internal-apis.md).

## Project structure

```
cmd/spy/          # entry point + flag parsing
internal/
  config/         # TOML loader, precedence merge
  graphics/       # Kitty / iTerm2 / Sixel encoders, PDF rasterization
  highlight/      # Chroma wrapper + per-session cap
  keys/           # key bindings (default + vim) via bubbles/key
  loader/         # streaming + windowed loaders
  render/         # code/markdown/pdf/image renderers, status bar
  search/         # forward/backward scan, wrap-around, regex/literal
  source/         # File/Stdin sources, content-type detection
  term/           # capability + theme detection
  ui/             # Bubble Tea model: update/view/commands
specs/001-popup-reader/  # spec, plan, research, contracts, tasks
tests/
  e2e/            # shell-driven non-TTY pipeline tests
  integration/    # PTY-driven end-to-end tests
```

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for build/test/coverage targets and
the PTY harness used by integration tests.

## License

Dual-licensed under [MIT](LICENSE-MIT) or
[Apache-2.0](LICENSE-Apache.20). The repository is REUSE 3.3 compliant
(every file carries SPDX headers); run `reuse lint` from the root to
verify.
