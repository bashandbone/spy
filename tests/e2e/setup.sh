#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# tests/e2e/setup.sh — materialize the /tmp/spy-fixtures layout that
# quickstart.md Section 0 documents, sourced from local fixtures only.
# This script is idempotent and never reaches the network.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.."
FIXTURES_TEXT="$REPO_ROOT/tests/e2e/fixtures"
FIXTURES_PDF="$REPO_ROOT/tests/fixtures/pdf"
FIXTURES_IMG="$REPO_ROOT/tests/fixtures/img"
FIXTURES_DST="${SPY_FIXTURES_DIR:-/tmp/spy-fixtures}"

mkdir -p "$FIXTURES_DST"

# Always-present text fixtures. big.txt is generated to keep the repo small.
if [[ -f "$FIXTURES_TEXT/hello.go" ]]; then
  cp "$FIXTURES_TEXT/hello.go" "$FIXTURES_DST/hello.go"
fi
if [[ ! -f "$FIXTURES_DST/big.txt" ]]; then
  seq 1 10000 > "$FIXTURES_DST/big.txt"
fi
: > "$FIXTURES_DST/empty.txt"

# Optional binary fixtures (PDF + image). Skipped silently when absent
# so the script stays usable even when the repo opts to ship without
# them. Tests that need these guard with `[[ -f ... ]]`.
for f in dummy.pdf multi-page.pdf; do
  if [[ -f "$FIXTURES_PDF/$f" ]]; then
    cp "$FIXTURES_PDF/$f" "$FIXTURES_DST/$f"
  fi
done
for f in small.png; do
  if [[ -f "$FIXTURES_IMG/$f" ]]; then
    cp "$FIXTURES_IMG/$f" "$FIXTURES_DST/$f"
  fi
done

echo "fixtures ready: $FIXTURES_DST"
