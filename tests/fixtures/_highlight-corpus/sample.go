// Go sample
package main

import (
	"bufio"
	"fmt"
	"os"
)

type Counter struct {
	value int
}

func (c *Counter) Increment(n int) int {
	c.value += n
	return c.value
}

func main() {
	f, err := os.Open("input.txt")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	c := &Counter{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if line != "" {
			c.Increment(len(line))
		}
	}
	if err := s.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("total bytes: %d\n", c.value)
}
