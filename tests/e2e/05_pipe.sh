#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# US5 E2E: validates the non-TTY stdin / pipe input paths — the
# verbatim degenerate-cat contract from contracts/cli.md "Stdin
# behavior" plus exit-code shape for piped input. The actual shapes
# tested below are:
#   (a) `cat hello.go | spy -l go`     — verbatim bytes through (Go --lang)
#   (b) `cat hello.go | spy`           — same, no --lang hint
#   (c) `grep -n func hello.go | spy`  — verbatim grep output
#   (d) `printf … | spy | cat`         — explicit pipe-into-pipe round-trip
#   (e) `cat hello.go | spy -`         — explicit `-` positional
#   (f) `spy < /dev/null`              — empty stdin pipe (exit 0)
# Shape (g) — TTY-stdin without a FILE — is impossible to drive from a
# shell harness; the equivalent unit test lives in cmd/spy
# (TestRun_StdinTTYWithoutFileExitsUsage). See the inline note below.
#
# SC-011 split (acceptance review M11): the interactive half of SC-011
# — the alt-screen footer literally rendering `<stdin>` rather than a
# basename, syntax highlighting for displayed content, and `q` exiting
# cleanly — is asserted by the PTY-driven integration tests at
# `tests/integration/stdin_test.go` and `tests/integration/footer_test.go`.
# Those tests are live (no `t.Skip`); this shell scaffolding deliberately
# covers only the non-TTY half — verbatim degenerate-cat and exit-code
# contracts — and explicitly does NOT claim to cover the alt-screen
# footer / highlighting / `q`-exit assertions.

# pipefail is critical here: shape (d) (`printf | spy | cat`) would
# otherwise mask a non-zero spy exit because `cat` succeeds (Copilot
# review PR#12 #4).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
fixtures="${SPY_FIXTURES_DIR:-${here}/fixtures}"
binary="${SPY_BIN:-${here}/../../spy}"

if [[ ! -x "${binary}" ]]; then
    binary="${here}/../../bin/spy"
fi
if [[ ! -x "${binary}" ]]; then
    echo "spy binary not found; run 'make build' first" >&2
    exit 1
fi

hello_go="${fixtures}/hello.go"
if [[ ! -f "${hello_go}" ]]; then
    echo "fixture not found: ${hello_go}" >&2
    exit 1
fi

tmp_a="$(mktemp)"
tmp_b="$(mktemp)"
tmp_c="$(mktemp)"
tmp_d="$(mktemp)"
tmp_d_want="$(mktemp)"
tmp_d_got="$(mktemp)"
trap 'rm -f "${tmp_a}" "${tmp_b}" "${tmp_c}" "${tmp_d}" "${tmp_d_want}" "${tmp_d_got}"' EXIT

# (a) `cat hello.go | spy -l go` — non-TTY stdin, non-TTY stdout. The
#     pipeline degenerate-cats stdin verbatim to stdout. Compare bytes
#     end-to-end via `cmp -s` so any in-flight transformation (newline
#     normalisation, ANSI injection) trips the assertion (Copilot
#     review PR#12 round-3 #9).
cat "${hello_go}" | "${binary}" --no-config -l go > "${tmp_a}"
if ! cmp -s "${hello_go}" "${tmp_a}"; then
    echo "(a) cat hello.go | spy -l go: content mismatch" >&2
    diff "${hello_go}" "${tmp_a}" >&2 || true
    exit 1
fi

# (b) `cat hello.go | spy` — same shape, no `--lang` hint. Detection
#     should classify as Go via shebang/Chroma, and the pipeline still
#     degenerate-cats verbatim on the non-TTY pipeline (Copilot
#     review PR#12 round-3 #10).
cat "${hello_go}" | "${binary}" --no-config > "${tmp_b}"
if ! cmp -s "${hello_go}" "${tmp_b}"; then
    echo "(b) cat hello.go | spy: content mismatch" >&2
    diff "${hello_go}" "${tmp_b}" >&2 || true
    exit 1
fi

# (c) `grep -n func hello.go | spy` — plain text content piped in,
#     non-TTY stdout. The grep output is verbatim-copied to stdout.
expected="$(grep -n func "${hello_go}")"
got="$(grep -n func "${hello_go}" | "${binary}" --no-config)"
if [[ "${expected}" != "${got}" ]]; then
    echo "(c) grep -n | spy: content mismatch" >&2
    diff <(printf '%s' "${expected}") <(printf '%s' "${got}") >&2 || true
    exit 1
fi

# (d) `printf | spy | cat` — explicit pipe-into-pipe (the
#     degenerate-cat contract from contracts/cli.md). Content survives
#     the round trip verbatim and exit code is 0. We compare via temp
#     files + `cmp -s` so trailing newlines aren't stripped by command
#     substitution (Copilot review PR#12 #7); pipefail (set above)
#     ensures a non-zero spy exit propagates through the trailing
#     `cat` and trips `set -e`.
printf 'alpha\nbeta\ngamma\n' > "${tmp_d_want}"
printf 'alpha\nbeta\ngamma\n' | "${binary}" --no-config | cat > "${tmp_d_got}"
if ! cmp -s "${tmp_d_want}" "${tmp_d_got}"; then
    echo "(d) printf | spy | cat: content mismatch" >&2
    diff "${tmp_d_want}" "${tmp_d_got}" >&2 || true
    exit 1
fi

# (e) `spy -` with stdin pipe forces stdin even in this shape; non-TTY
#     stdout still degenerate-cats. Compare bytes against the fixture
#     so the verbatim contract is actually pinned (Copilot review
#     PR#12 round-3 #11).
cat "${hello_go}" | "${binary}" --no-config - > "${tmp_d}"
if ! cmp -s "${hello_go}" "${tmp_d}"; then
    echo "(e) cat | spy -: content mismatch" >&2
    diff "${hello_go}" "${tmp_d}" >&2 || true
    exit 1
fi

# (f) `spy < /dev/null` (no FILE, empty stdin pipe) exits 0 with empty
#     output — empty pipe is valid input per contracts/cli.md
#     "Empty input".
"${binary}" --no-config < /dev/null > "${tmp_c}"
empty_size="$(stat -c%s "${tmp_c}" 2>/dev/null || stat -f%z "${tmp_c}")"
if [[ "${empty_size}" != "0" ]]; then
    echo "(f) spy </dev/null: expected empty output, got ${empty_size} bytes" >&2
    exit 1
fi

# (g) `spy` with stdin redirected from a TTY is impossible to test in
#     this shell-based harness (we'd need an embedded PTY). The
#     equivalent is verified at the unit-test layer in cmd/spy:
#     TestRun_StdinTTYWithoutFileExitsUsage feeds a nil stdin (the
#     "no input available" branch) and asserts exit 2.

echo "OK: 05_pipe"
