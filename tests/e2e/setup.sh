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
FIXTURES_SRC="$REPO_ROOT/tests/e2e/fixtures"
FIXTURES_DST="${SPY_FIXTURES_DIR:-/tmp/spy-fixtures}"

mkdir -p "$FIXTURES_DST"

# Always-present text fixtures. big.txt is generated to keep the repo small.
if [[ -f "$FIXTURES_SRC/hello.go" ]]; then
  cp "$FIXTURES_SRC/hello.go" "$FIXTURES_DST/hello.go"
fi
if [[ ! -f "$FIXTURES_DST/big.txt" ]]; then
  seq 1 10000 > "$FIXTURES_DST/big.txt"
fi
: > "$FIXTURES_DST/empty.txt"

# Optional binary fixtures. Skipped silently when absent so the script
# stays usable even when the repo opts to ship without them. Tests that
# need these guard with `[[ -f ... ]]`.
for f in dummy.pdf multi-page.pdf iso.png; do
  if [[ -f "$FIXTURES_SRC/$f" ]]; then
    cp "$FIXTURES_SRC/$f" "$FIXTURES_DST/$f"
  fi
done

echo "fixtures ready: $FIXTURES_DST"
