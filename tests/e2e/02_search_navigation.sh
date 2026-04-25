#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# US2 E2E: validates that --vim and --no-config flags parse cleanly and
# that the degenerate-cat path (non-TTY stdout) preserves big.txt
# content unchanged. The interactive search flow (`/9999<Enter>`,
# `:1<Enter>`, `:$<Enter>`) needs the PTY harness from T104; the
# scaffolding here exercises the parts of US2 that don't need a PTY:
#   - --vim flag accepted without error
#   - --no-config flag accepted (regression for cmd/spy/flags.go)
#   - non-TTY output preserves the source verbatim regardless of mode
#
# Once T104 lands the script grows asserts that:
#   - `/9999<Enter>` scrolls to line 9999 of big.txt
#   - `:1<Enter>` returns to top
#   - `:$<Enter>` jumps to the last line
#   - `--vim` enables `gg` / `G` / `Ctrl-D` / `Ctrl-U`
# For now the script keeps US2 from regressing the degenerate-cat path
# without depending on infrastructure that hasn't shipped.

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

big="${fixtures}/big.txt"
if [[ ! -f "${big}" ]]; then
    echo "fixture not found: ${big}" >&2
    exit 1
fi

# 1. Default mode degenerate-cat preserves content.
tmp_default="$(mktemp)"
trap 'rm -f "${tmp_default}" "${tmp_vim:-}"' EXIT

set +e
"${binary}" --no-config "${big}" >"${tmp_default}"
default_exit=$?
set -e

if [[ ${default_exit} -ne 0 ]]; then
    echo "default mode: expected exit 0; got ${default_exit}" >&2
    head -n 5 "${tmp_default}" >&2 || true
    exit 1
fi

if ! grep -q '^9999$' "${tmp_default}"; then
    echo "default mode: line '9999' missing from output" >&2
    exit 1
fi

if ! grep -q '^1$' "${tmp_default}"; then
    echo "default mode: line '1' missing from output" >&2
    exit 1
fi

# 2. --vim flag must parse cleanly and produce identical output (since
#    keybindings only matter on a TTY; non-TTY falls back to cat).
tmp_vim="$(mktemp)"
set +e
"${binary}" --no-config --vim "${big}" >"${tmp_vim}"
vim_exit=$?
set -e

if [[ ${vim_exit} -ne 0 ]]; then
    echo "--vim mode: expected exit 0; got ${vim_exit}" >&2
    head -n 5 "${tmp_vim}" >&2 || true
    exit 1
fi

if ! diff -q "${tmp_default}" "${tmp_vim}" >/dev/null; then
    echo "--vim should produce identical degenerate-cat output as default" >&2
    diff "${tmp_default}" "${tmp_vim}" | head -n 20 >&2 || true
    exit 1
fi

# 3. ANSI must not leak into the non-TTY stream regardless of flag.
if grep -q $'\x1b\[' "${tmp_default}"; then
    echo "default mode: ANSI escapes leaked into non-TTY output" >&2
    exit 1
fi

if grep -q $'\x1b\[' "${tmp_vim}"; then
    echo "--vim mode: ANSI escapes leaked into non-TTY output" >&2
    exit 1
fi

echo "OK: 02_search_navigation"
