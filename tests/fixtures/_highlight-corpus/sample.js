// JavaScript sample
import { readFile } from "node:fs/promises";

class Counter {
  #value = 0;
  increment(n = 1) {
    this.#value += n;
    return this.#value;
  }
  get value() {
    return this.#value;
  }
}

async function main() {
  const data = await readFile("input.txt", "utf8");
  const lines = data.split("\n").filter((l) => l.length > 0);
  const counter = new Counter();
  for (const line of lines) {
    counter.increment(line.length);
  }
  console.log(`total bytes: ${counter.value}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
