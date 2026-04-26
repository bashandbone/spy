// TypeScript sample
import { readFile } from "node:fs/promises";

interface Incrementable {
  increment(n?: number): number;
  readonly value: number;
}

class Counter implements Incrementable {
  private _value = 0;
  increment(n: number = 1): number {
    this._value += n;
    return this._value;
  }
  get value(): number {
    return this._value;
  }
}

async function main(): Promise<void> {
  const data: string = await readFile("input.txt", "utf8");
  const lines: string[] = data.split("\n").filter((l) => l.length > 0);
  const counter: Incrementable = new Counter();
  for (const line of lines) {
    counter.increment(line.length);
  }
  console.log(`total bytes: ${counter.value}`);
}

main();
