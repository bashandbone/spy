#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# US3 E2E: validates the theme-resolution paths that don't require a PTY.
# The interactive theme behavior (visible color swap on `:set theme …`,
# OSC 11 luminance probe driving the auto-detect branch) needs the PTY
# harness from T104; the scaffolding here exercises the parts of US3
# that survive on a non-TTY pipeline:
#   - --theme dark / --theme light / --theme auto all parse cleanly
#   - SPY_THEME=light / SPY_THEME=dark env override parses cleanly
#   - NO_COLOR=1 forces Mono (no ANSI escapes in the output stream)
#   - Content is preserved verbatim across every theme variant
#
# When T104 lands the script grows asserts that:
#   - `--theme light` actually emits the github Chroma style escapes
#   - `:set theme dark` re-renders without re-tokenization
#   - The OSC 11 PTY responder (T062) drives the auto branch correctly
# For now the script keeps US3 from regressing the parse path and the
# NO_COLOR contract without depending on infrastructure that hasn't
# shipped.

set -eu

here="$(cd "$(dirname "$0")" && pwd)"
fixtures="${SPY_FIXTURES_DIR:-/tmp/spy-fixtures}"
binary="${SPY_BIN:-${here}/../../spy}"

if [[ ! -x "${binary}" ]]; then
    binary="${here}/../../bin/spy"
fi
if [[ ! -x "${binary}" ]]; then
    echo "spy binary not found; run 'make build' first" >&2
    exit 1
fi

hello="${fixtures}/hello.go"
if [[ ! -f "${hello}" ]]; then
    hello="${here}/fixtures/hello.go"
fi
if [[ ! -f "${hello}" ]]; then
    echo "fixture not found: ${hello}" >&2
    exit 1
fi

tmp_dark="$(mktemp)"
tmp_light="$(mktemp)"
tmp_auto="$(mktemp)"
tmp_envlight="$(mktemp)"
tmp_nocolor="$(mktemp)"
trap 'rm -f "${tmp_dark}" "${tmp_light}" "${tmp_auto}" "${tmp_envlight}" "${tmp_nocolor}"' EXIT

# 1. Each --theme value parses and produces identical degenerate-cat
#    output (theming only affects rendering, which doesn't run on a
#    non-TTY pipeline).
"${binary}" --no-config --theme dark  "${hello}" > "${tmp_dark}"
"${binary}" --no-config --theme light "${hello}" > "${tmp_light}"
"${binary}" --no-config --theme auto  "${hello}" > "${tmp_auto}"

if ! diff -q "${tmp_dark}" "${tmp_light}" >/dev/null; then
    echo "--theme dark vs light: degenerate-cat output diverged" >&2
    diff "${tmp_dark}" "${tmp_light}" | head -n 20 >&2 || true
    exit 1
fi

if ! diff -q "${tmp_dark}" "${tmp_auto}" >/dev/null; then
    echo "--theme dark vs auto: degenerate-cat output diverged" >&2
    diff "${tmp_dark}" "${tmp_auto}" | head -n 20 >&2 || true
    exit 1
fi

# 2. Content preservation: the Go source must reach stdout intact.
if ! grep -q 'package main' "${tmp_dark}"; then
    echo "--theme dark: 'package main' missing from output" >&2
    exit 1
fi

# 3. SPY_THEME env override parses cleanly and behaves identically to
#    --theme on the degenerate-cat path.
SPY_THEME=light "${binary}" --no-config "${hello}" > "${tmp_envlight}"
if ! diff -q "${tmp_envlight}" "${tmp_light}" >/dev/null; then
    echo "SPY_THEME=light vs --theme light: output diverged" >&2
    diff "${tmp_envlight}" "${tmp_light}" | head -n 20 >&2 || true
    exit 1
fi

# 4. NO_COLOR=1 must produce no escape sequences anywhere in the stream.
#    We grep for any ESC (\x1b) byte rather than just CSI (\x1b[) so a
#    stray OSC, DCS, or bare ESC also fails the assertion (Copilot
#    review PR#10 round-3 #3).
NO_COLOR=1 "${binary}" --no-config "${hello}" > "${tmp_nocolor}"
if grep -q $'\x1b' "${tmp_nocolor}"; then
    echo "NO_COLOR=1: escape sequences leaked into non-TTY output" >&2
    exit 1
fi

# 5. No escape sequences may leak into any non-TTY stream regardless
#    of theme (same broadened ESC check as step 4).
for f in "${tmp_dark}" "${tmp_light}" "${tmp_auto}" "${tmp_envlight}"; do
    if grep -q $'\x1b' "${f}"; then
        echo "non-TTY escape leak in ${f}" >&2
        exit 1
    fi
done

echo "OK: 03_theme"
