# Markdown sample

A keyboard-driven popup reader for the terminal — _`bat` with a real viewport._

## Features

- **Syntax highlighting** for 100+ languages via [Chroma](https://github.com/alecthomas/chroma)
- **Theme detection** via OSC 11 background-color queries
- **Pipe input**: `git diff | spy`, `cat hello.go | spy -l go`
- **PDF and images** with the Kitty, iTerm2, and Sixel protocols

## Quick start

```bash
git clone https://github.com/knitli/spy.git
cd spy
make build
./bin/spy README.md
```

## Configuration

Drop a TOML file at `$XDG_CONFIG_HOME/spy/config.toml`:

```toml
theme = "auto"
vim_mode = false
highlight_cap_bytes = 5242880
```

## Keybindings

| Action | Default | Vim |
|--------|---------|-----|
| Scroll | `↑` `↓` | `k` `j` |
| Page   | `PgDn`  | `Ctrl-F` |
| Quit   | `q`     | —    |

> **Note**: pressing `?` opens the in-app help.
