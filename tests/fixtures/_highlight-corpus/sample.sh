#!/usr/bin/env bash
# Bash sample
set -euo pipefail

count=0
input="${1:-input.txt}"

if [[ ! -f "$input" ]]; then
    echo "input file not found: $input" >&2
    exit 1
fi

while IFS= read -r line; do
    if [[ -n "$line" ]]; then
        count=$((count + ${#line}))
    fi
done < "$input"

printf 'total bytes: %d\n' "$count"
