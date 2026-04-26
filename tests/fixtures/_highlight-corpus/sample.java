// Java sample
package com.example.demo;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;

public final class Counter {
    private int value;

    public Counter() {
        this.value = 0;
    }

    public int increment(int n) {
        this.value += n;
        return this.value;
    }

    public int value() {
        return this.value;
    }

    public static void main(String[] args) throws Exception {
        List<String> lines = Files.readAllLines(Path.of("input.txt"));
        var counter = new Counter();
        for (String line : lines) {
            if (!line.isEmpty()) {
                counter.increment(line.length());
            }
        }
        System.out.printf("total bytes: %d%n", counter.value());
    }
}
