# Nim sample
import std/[os, strutils]

type
  Counter = ref object
    value: int

proc increment(self: Counter, n: int = 1): int =
  self.value += n
  result = self.value

proc countBytes(path: string): int =
  let counter = Counter(value: 0)
  for line in lines(path):
    if line.len > 0:
      discard counter.increment(line.len)
  result = counter.value

when isMainModule:
  let path = if paramCount() >= 1: paramStr(1) else: "input.txt"
  let total = countBytes(path)
  echo "total bytes: ", total
