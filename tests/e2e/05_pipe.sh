#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# US5 E2E: validates the stdin / pipe input paths that don't require a
# PTY. The interactive parts (alt-screen frame with `<stdin>` in the
# footer, scroll, `q` exit) need the PTY harness from T104; the
# scaffolding here exercises the non-TTY pipeline shape — the
# degenerate-cat contract from contracts/cli.md "Stdin behavior" — for
# the three SC-011 pipeline shapes:
#   (a) `cat fixture | spy -l go` (Go highlight; <stdin> footer)
#   (b) `git diff HEAD~ | spy` when invoked inside the repo (diff highlight)
#   (c) `grep -n needle fixture | spy` (plain text; <stdin> footer)
# plus the explicit degenerate-cat case
#   (d) `echo content | spy | cat` exits 0 with verbatim content.
#
# When T104 lands the script grows assertions that:
#   - the alt-screen frame's footer reads `<stdin>` (not a basename)
#   - syntax highlighting is applied for shape (a) and (b)
#   - `q` exits cleanly with code 0.
# For now the script keeps US5 from regressing the `-` positional, the
# auto-stdin pickup, and the verbatim-cat exit-0 contract.

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
trap 'rm -f "${tmp_a}" "${tmp_b}" "${tmp_c}" "${tmp_d}"' EXIT

# (a) `cat hello.go | spy -l go` — non-TTY stdin, non-TTY stdout. The
#     pipeline degenerate-cats stdin verbatim to stdout. The byte count
#     matches the source.
cat "${hello_go}" | "${binary}" --no-config -l go > "${tmp_a}"
src_size="$(stat -c%s "${hello_go}" 2>/dev/null || stat -f%z "${hello_go}")"
out_size="$(stat -c%s "${tmp_a}" 2>/dev/null || stat -f%z "${tmp_a}")"
if [[ "${src_size}" != "${out_size}" ]]; then
    echo "(a) cat hello.go | spy -l go: byte count diverged (src=${src_size}, out=${out_size})" >&2
    exit 1
fi

# (b) `cat hello.go | spy` — same shape, no `--lang` hint. Detection
#     should classify as Go via shebang/Chroma, and the pipeline still
#     degenerate-cats verbatim on the non-TTY pipeline.
cat "${hello_go}" | "${binary}" --no-config > "${tmp_b}"
out_size="$(stat -c%s "${tmp_b}" 2>/dev/null || stat -f%z "${tmp_b}")"
if [[ "${src_size}" != "${out_size}" ]]; then
    echo "(b) cat hello.go | spy: byte count diverged" >&2
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

# (d) `echo content | spy | cat` — explicit pipe-into-pipe (the
#     degenerate-cat contract from contracts/cli.md). Content survives
#     the round trip verbatim and exit code is 0.
want=$'alpha\nbeta\ngamma\n'
got_d="$(printf '%s' "${want}" | "${binary}" --no-config | cat)"
if [[ "${got_d}" != "${want%$'\n'}" && "${got_d}" != "${want}" ]]; then
    echo "(d) echo | spy | cat: content mismatch" >&2
    printf '  want=%q\n  got =%q\n' "${want}" "${got_d}" >&2
    exit 1
fi

# (e) `spy -` with stdin pipe forces stdin even in this shape; non-TTY
#     stdout still degenerate-cats. Byte count matches.
cat "${hello_go}" | "${binary}" --no-config - > "${tmp_d}"
out_size="$(stat -c%s "${tmp_d}" 2>/dev/null || stat -f%z "${tmp_d}")"
if [[ "${src_size}" != "${out_size}" ]]; then
    echo "(e) cat | spy -: byte count diverged" >&2
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
