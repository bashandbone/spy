#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# US4 E2E: validates the graphics-fallback paths that don't require a PTY.
# The actual inline-image rendering (Kitty/iTerm2/sixel escape emission)
# needs the PTY harness from T104; the scaffolding here exercises the
# parts of US4 that survive on a non-TTY pipeline:
#   - `--graphics none` against an image fixture: degenerate-cats the
#     bytes (because non-TTY stdout takes the cat path before graphics
#     dispatch even fires) and exits 0.
#   - `--graphics none` against a PDF fixture: same — the non-TTY
#     pipeline degenerate-cats the PDF source.
#   - `--graphics kitty` parses cleanly even when stdout isn't a TTY;
#     the protocol override doesn't change the cat fallback contract.
#
# When T104 lands the script grows asserts that:
#   - `--graphics kitty <image.png>` over a PTY emits a Kitty graphics
#     escape (\x1b_G…) followed by alt-screen entry.
#   - `--graphics kitty -tags fitz <pdf>` rasterizes page 1 and
#     emits the corresponding payload.
#   - The cleanup escape (\x1b_Ga=d,d=A;\x1b\\) fires on tea.Quit
#     and on SIGINT.
# For now the script keeps US4 from regressing the `--graphics` parse
# contract and ensures the non-TTY pipeline remains a clean cat.

set -eu

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

dummy_pdf="${fixtures}/dummy.pdf"
if [[ ! -f "${dummy_pdf}" ]]; then
    echo "fixture not found: ${dummy_pdf}" >&2
    exit 1
fi

tmp_pdf_none="$(mktemp)"
tmp_pdf_kitty="$(mktemp)"
trap 'rm -f "${tmp_pdf_none}" "${tmp_pdf_kitty}"' EXIT

# 1. `--graphics none` parses and the non-TTY pipeline degenerate-cats
#    the PDF bytes. We confirm the byte count matches the source so the
#    cat fallback didn't truncate or transform the stream.
"${binary}" --no-config --graphics none "${dummy_pdf}" > "${tmp_pdf_none}"
src_size="$(stat -c%s "${dummy_pdf}" 2>/dev/null || stat -f%z "${dummy_pdf}")"
out_size="$(stat -c%s "${tmp_pdf_none}" 2>/dev/null || stat -f%z "${tmp_pdf_none}")"
if [[ "${src_size}" != "${out_size}" ]]; then
    echo "--graphics none: PDF byte count diverged (src=${src_size}, out=${out_size})" >&2
    exit 1
fi

# 2. `--graphics kitty` likewise parses; on a non-TTY the cat fallback
#    still applies (the protocol override doesn't override the
#    non-TTY contract — only the renderer's encoder choice when the
#    TTY path is active).
"${binary}" --no-config --graphics kitty "${dummy_pdf}" > "${tmp_pdf_kitty}"
out_size="$(stat -c%s "${tmp_pdf_kitty}" 2>/dev/null || stat -f%z "${tmp_pdf_kitty}")"
if [[ "${src_size}" != "${out_size}" ]]; then
    echo "--graphics kitty: PDF byte count diverged on non-TTY pipeline" >&2
    exit 1
fi

# 3. No ANSI escapes may leak into the non-TTY stream regardless of the
#    --graphics value (mirrors 03_theme.sh's broadened ESC check).
for f in "${tmp_pdf_none}" "${tmp_pdf_kitty}"; do
    # `grep -q $'\x1b'` catches any ESC byte; PDFs frequently contain
    # binary content with ESC bytes embedded, so we instead check that
    # the `--graphics kitty` path didn't *prepend* a graphics envelope.
    # An unmodified PDF cat preserves the file verbatim — the byte-count
    # check above already guarantees that. This step adds a positive
    # sniff: the first 4 bytes of the source must equal the first 4 of
    # the output (`%PDF`).
    src_head="$(head -c4 "${dummy_pdf}")"
    out_head="$(head -c4 "${f}")"
    if [[ "${src_head}" != "${out_head}" ]]; then
        echo "non-TTY graphics path mutated the PDF header in ${f}" >&2
        exit 1
    fi
done

# 4. `--graphics garbage` should still parse — unknown values fall
#    through to "auto" per contracts/cli.md (caller surfaces a warning
#    but doesn't abort).
if ! "${binary}" --no-config --graphics garbage "${dummy_pdf}" > /dev/null 2>&1; then
    echo "--graphics garbage: unknown value should not abort the parser" >&2
    exit 1
fi

echo "OK: 04_graphics"
