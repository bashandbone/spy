#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Adam Poulemanos
#
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# tests/e2e/run.sh — build the binary and run every tests/e2e/NN_*.sh script.
# Each script is responsible for its own assertions and exits non-zero on
# failure. Fixtures live under tests/e2e/fixtures/; setup.sh materializes
# the same /tmp/spy-fixtures layout that quickstart.md documents so the
# scripts can reuse the canonical paths.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.."
cd "$REPO_ROOT"

# Build the default (pure-Go) binary; `-tags fitz` builds are exercised by
# specific scripts that opt in.
mkdir -p bin
go build -o bin/spy ./cmd/spy

# Materialize /tmp/spy-fixtures from the repo's local fixtures.
bash tests/e2e/setup.sh

# Run every NN_*.sh script in lexical order. A script can be skipped by
# exporting SPY_E2E_SKIP as a whitespace-delimited list of script names
# (without the .sh suffix), e.g. SPY_E2E_SKIP="01_text_review 04_graphics".
# Match is whole-token — substrings do not skip unrelated scripts.
should_skip() {
  local name="$1"
  local tok
  for tok in ${SPY_E2E_SKIP:-}; do
    if [[ "$tok" == "$name" ]]; then
      return 0
    fi
  done
  return 1
}

shopt -s nullglob
status=0
for script in tests/e2e/[0-9][0-9]_*.sh; do
  name="$(basename "$script" .sh)"
  if should_skip "$name"; then
    echo "skip: $name"
    continue
  fi
  echo "==> $name"
  if ! bash "$script"; then
    echo "FAIL: $name"
    status=1
  fi
done
exit "$status"
