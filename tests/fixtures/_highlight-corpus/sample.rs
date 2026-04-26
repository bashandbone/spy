// Rust sample
use std::fs;
use std::io::{self};

#[derive(Default)]
struct Counter {
    value: usize,
}

impl Counter {
    fn increment(&mut self, n: usize) -> usize {
        self.value += n;
        self.value
    }
}

fn main() -> io::Result<()> {
    let text = fs::read_to_string("input.txt")?;
    let mut counter = Counter::default();
    for line in text.lines() {
        if !line.is_empty() {
            counter.increment(line.len());
        }
    }
    println!("total bytes: {}", counter.value);
    Ok(())
}
