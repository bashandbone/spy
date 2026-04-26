#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# US6 E2E (T097): exercise the status-bar (footer) contract from
# contracts/cli.md. The interactive scroll / resize assertions require
# the PTY harness that lands with T104; for now this script covers the
# parts observable without one — namely that the binary still degrades
# cleanly on non-TTY stdout (the alt-screen + footer paint path is
# entirely skipped) and that no status-bar bytes leak into the
# degenerate-cat output.
#
# Documented assertions for the post-T104 PTY mode (lift into runnable
# code once the harness arrives):
#   1. spawn `./spy <100-line file>` on a 100x24 PTY.
#   2. wait for the alt-screen frame; assert the footer line contains:
#        - the file basename
#        - "100 lines"
#        - "Line 1"
#   3. send PageDown twice; assert the footer's "Line N" advances above 1
#      and the new top-row line number matches.
#   4. resize the PTY to 60x24; assert the footer collapses to
#      "<basename> · L<N>" with no " | " separators.
#   5. resize back to 100x24; assert the wide format returns.
#   6. send `q`; assert exit 0.

set -eu

here="$(cd "$(dirname "$0")" && pwd)"
binary="${SPY_BIN:-${here}/../../spy}"
if [[ ! -x "${binary}" ]]; then
    binary="${here}/../../bin/spy"
fi
if [[ ! -x "${binary}" ]]; then
    echo "spy binary not found; run 'make build' first" >&2
    exit 1
fi

# Build a deterministic 100-line fixture in a temp dir; each line
# carries its own number so we can spot-check the footer's line counter
# in the future PTY-mode assertions.
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

fixture="${tmp}/footer.txt"
{
    for i in $(seq 1 100); do
        printf 'line %03d\n' "${i}"
    done
} > "${fixture}"

# --- Degenerate-cat path: stdout redirected to a file is non-TTY, so
#     spy must `cat` the file verbatim, exit 0, and emit no ANSI / no
#     footer bytes (the status bar lives only in the alt-screen). ---
tmp_out="${tmp}/cat.out"
set +e
"${binary}" --no-config "${fixture}" >"${tmp_out}"
exit_code=$?
set -e

if [[ ${exit_code} -ne 0 ]]; then
    echo "expected exit 0; got ${exit_code}" >&2
    cat "${tmp_out}" >&2 || true
    exit 1
fi

# Content preserved.
if ! grep -q '^line 001$' "${tmp_out}"; then
    echo "expected first line 'line 001' in degenerate-cat output" >&2
    head "${tmp_out}" >&2
    exit 1
fi
if ! grep -q '^line 100$' "${tmp_out}"; then
    echo "expected last line 'line 100' in degenerate-cat output" >&2
    tail "${tmp_out}" >&2
    exit 1
fi

# No ANSI escapes (status-bar styling must never leak to non-TTY).
if grep -q $'\x1b\[' "${tmp_out}"; then
    echo "non-TTY output contained ANSI escape sequences" >&2
    exit 1
fi

# No footer markers (the wide-mode separators or collapse mid-dot must
# never appear in non-TTY stdout).
if grep -qE '( \| | · L[0-9]+)' "${tmp_out}"; then
    echo "non-TTY output contained status-bar markers" >&2
    grep -nE '( \| | · L[0-9]+)' "${tmp_out}" >&2 || true
    exit 1
fi

# Line count matches what the footer would report (sanity check on the
# fixture itself; future PTY assertions use this as the expected total).
total="$(wc -l < "${tmp_out}" | tr -d '[:space:]')"
if [[ "${total}" != "100" ]]; then
    echo "expected 100-line fixture, got ${total}" >&2
    exit 1
fi

echo "OK: 06_footer (degenerate-cat path; PTY assertions deferred to T104)"
