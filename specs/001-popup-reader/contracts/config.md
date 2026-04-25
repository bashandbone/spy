<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Config File Contract

**Spec**: 001-popup-reader (Assumptions: configuration via simple file)
**Format**: TOML
**Default location**: `$XDG_CONFIG_HOME/spy/config.toml` (falls back to `$HOME/.config/spy/config.toml`)

The config file is optional. CLI flags > env vars > config file > compiled defaults.

## Schema

```toml
# All keys shown with their default values.

# Theme: "auto" detects from terminal background luminance.
theme = "auto"                      # "auto" | "dark" | "light" | "<chroma-style>"

# Enable additive vim keybindings.
vim_mode = false

# Search defaults.
regex_default = false               # treat search queries as regex by default
case_mode = "smart"                 # "smart" | "sensitive" | "insensitive"

# Display.
word_wrap = true
line_numbers = true
tab_width = 4

# Memory and performance limits.
max_resident_bytes = 268435456      # 256 MiB; switch to windowed mode above this
window_size = 8192                  # lines kept hot in windowed mode
highlight_cap_bytes = 5242880       # 5 MiB; disable syntax highlighting above this

# Graphics.
graphics = "auto"                   # "auto" | "none" | "kitty" | "iterm2" | "sixel"

# Minimum supported terminal size before degraded layout (FR clarifications Q4).
min_cols = 80
min_rows = 24

# Custom key bindings (optional). Each value is a list of key strings.
# Keys reference: see contracts/keys.md.
[keys]
# quit             = ["q", "esc", "ctrl+c"]
# search_forward   = ["/"]
# go_to_line       = [":"]

# Per-language overrides (optional). Key is a Chroma lexer name (lowercased).
[lang.go]
# theme = "github"

[lang.markdown]
# word_wrap = true
```

## Validation

- Unknown top-level keys: warning to stderr (or `--debug` log if set), key ignored.
- Invalid value type or out-of-range integer: warning, default used.
- `[keys]` table accepts only known action names; unknown actions ignored with warning.
- `[lang.<name>]` tables accept the same keys as the top level except `[keys]`.
- Empty file is valid; equivalent to no config file.

## Examples

### Minimal: prefer dark theme always

```toml
theme = "dark"
```

### Power-user: vim + regex + tighter memory

```toml
vim_mode = true
regex_default = true
max_resident_bytes = 67108864       # 64 MiB
```

### Per-language tuning

```toml
[lang.json]
word_wrap = false                   # JSON is easier with horizontal scroll
line_numbers = true

[lang.markdown]
theme = "github"
```

## Discovery rules

1. If `--config <path>` is provided, load only that file (failure is a hard error, exit 2).
2. Else, if `--no-config` is set, use compiled defaults.
3. Else, look up `$XDG_CONFIG_HOME/spy/config.toml`, falling back to
   `$HOME/.config/spy/config.toml`. Missing file is silently OK.
4. After loading, env vars override matching fields; flags override env vars.

## Stability

The set of accepted keys is part of the public contract; new keys may be added
in minor releases, existing keys may not be renamed or have semantics changed
without a major version bump.
