#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# US1 E2E: open hello.go and confirm spy exits cleanly when stdout is
# piped to a non-TTY (degenerate-cat path). The full alt-screen
# verification ships once the PTY harness (T104) lands; for now the
# script asserts the documented degenerate-cat behaviour from
# contracts/cli.md "Stdout (non-TTY)".

set -eu

here="$(cd "$(dirname "$0")" && pwd)"
fixture="${here}/fixtures/hello.go"
binary="${SPY_BIN:-${here}/../../spy}"

if [[ ! -x "${binary}" ]]; then
    echo "spy binary not found at ${binary}; run 'make build' first" >&2
    exit 1
fi

if [[ ! -f "${fixture}" ]]; then
    echo "fixture not found at ${fixture}" >&2
    exit 1
fi

# Capture stdout; with stdout non-TTY the binary cats the file verbatim.
out="$("${binary}" --no-config "${fixture}" | cat)"
exit_code=$?

if [[ ${exit_code} -ne 0 ]]; then
    echo "expected exit 0; got ${exit_code}" >&2
    exit 1
fi

# Verify the file content is preserved verbatim.
if ! echo "${out}" | grep -q 'package main'; then
    echo "expected fixture content 'package main' in degenerate-cat output" >&2
    echo "got: ${out}" >&2
    exit 1
fi

if ! echo "${out}" | grep -q 'fmt.Println'; then
    echo "expected fixture content 'fmt.Println' in degenerate-cat output" >&2
    echo "got: ${out}" >&2
    exit 1
fi

# Confirm no ANSI escapes leaked into the non-TTY stream.
if echo "${out}" | grep -q $'\x1b\['; then
    echo "non-TTY output contained ANSI escape sequences" >&2
    exit 1
fi

echo "OK: 01_text_review"
