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
#
# Uses a temp-file capture (not a pipeline) so spy's exit status is
# preserved verbatim — a `cmd | cat` substitution would mask it
# unless `set -o pipefail` is on, and even then `set -e` aborts before
# `exit_code=$?` is reachable (Copilot review PR#8 #5).

set -eu

here="$(cd "$(dirname "$0")" && pwd)"
fixture="${here}/fixtures/hello.go"
binary="${SPY_BIN:-${here}/../../spy}"

if [[ ! -x "${binary}" ]]; then
    binary="${here}/../../bin/spy"
fi
if [[ ! -x "${binary}" ]]; then
    echo "spy binary not found; run 'make build' first" >&2
    exit 1
fi

if [[ ! -f "${fixture}" ]]; then
    echo "fixture not found at ${fixture}" >&2
    exit 1
fi

tmp_out="$(mktemp)"
trap 'rm -f "${tmp_out}"' EXIT

# Redirecting stdout to a regular file makes stdout non-TTY without a
# pipeline, so $? reflects spy's exit status directly.
set +e
"${binary}" --no-config "${fixture}" >"${tmp_out}"
exit_code=$?
set -e

if [[ ${exit_code} -ne 0 ]]; then
    echo "expected exit 0; got ${exit_code}" >&2
    cat "${tmp_out}" >&2 || true
    exit 1
fi

out="$(cat "${tmp_out}")"

# Verify the file content is preserved verbatim.
if ! grep -q 'package main' "${tmp_out}"; then
    echo "expected fixture content 'package main' in degenerate-cat output" >&2
    echo "got: ${out}" >&2
    exit 1
fi

if ! grep -q 'fmt.Println' "${tmp_out}"; then
    echo "expected fixture content 'fmt.Println' in degenerate-cat output" >&2
    echo "got: ${out}" >&2
    exit 1
fi

# Confirm no ANSI escapes leaked into the non-TTY stream.
if grep -q $'\x1b\[' "${tmp_out}"; then
    echo "non-TTY output contained ANSI escape sequences" >&2
    exit 1
fi

echo "OK: 01_text_review"
