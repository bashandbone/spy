# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# VHS tape — records the spy demo GIF embedded in README.md.
#
# Prerequisites:
#   vhs    https://github.com/charmbracelet/vhs
#   spy    make build && export PATH=$PWD/bin:$PATH
#
# Usage:
#   vhs demo.tape

Output demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 680
Set Theme "Catppuccin Mocha"
Set Padding 12

# ---------------------------------------------------------------------------
# Scene 1 — open a Go source file (syntax highlighting + line numbers)
# ---------------------------------------------------------------------------
Type "spy internal/ui/model.go"
Enter
Sleep 800ms

# Scroll down a few pages to show the content
PageDown
Sleep 400ms
PageDown
Sleep 400ms

# ---------------------------------------------------------------------------
# Scene 2 — forward search
# ---------------------------------------------------------------------------
Type "/"
Sleep 200ms
Type "Update"
Enter
Sleep 500ms

# Next match
Type "n"
Sleep 300ms
Type "n"
Sleep 300ms

# ---------------------------------------------------------------------------
# Scene 3 — jump to a line number
# ---------------------------------------------------------------------------
Type ":"
Sleep 200ms
Type "1"
Enter
Sleep 400ms

# ---------------------------------------------------------------------------
# Scene 4 — help overlay
# ---------------------------------------------------------------------------
Type "?"
Sleep 1s
Type "?"
Sleep 300ms

# Quit
Type "q"
Sleep 300ms

# ---------------------------------------------------------------------------
# Scene 5 — open Markdown (Glamour rendering)
# ---------------------------------------------------------------------------
Type "spy README.md"
Enter
Sleep 800ms
PageDown
Sleep 500ms
PageDown
Sleep 500ms
Type "q"
Sleep 300ms

# ---------------------------------------------------------------------------
# Scene 6 — pipe input (git log)
# ---------------------------------------------------------------------------
Type "git log --oneline -20 | spy"
Enter
Sleep 800ms
Type "/"
Sleep 200ms
Type "feat"
Enter
Sleep 500ms
Type "q"
Sleep 300ms

# ---------------------------------------------------------------------------
# Scene 7 — forced dark theme on the config example
# ---------------------------------------------------------------------------
Type "spy --theme dark examples/config.toml"
Enter
Sleep 800ms
PageDown
Sleep 400ms
Type "q"
Sleep 500ms
