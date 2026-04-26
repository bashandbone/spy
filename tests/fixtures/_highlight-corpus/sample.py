# Python sample
"""A small illustrative module."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Counter:
    value: int = 0

    def incremented(self, n: int = 1) -> "Counter":
        return Counter(self.value + n)


async def main(path: Path) -> int:
    text = path.read_text(encoding="utf-8")
    counter = Counter()
    for line in text.splitlines():
        if line:
            counter = counter.incremented(len(line))
    print(f"total bytes: {counter.value}")
    return 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main(Path("input.txt"))))
