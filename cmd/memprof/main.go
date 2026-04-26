// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"unsafe"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
)

func writeSyntheticFile(targetBytes, lineBytes int) string {
	dir, _ := os.MkdirTemp("", "spy-perf")
	path := filepath.Join(dir, "synthetic.txt")
	f, _ := os.Create(path)
	defer f.Close()
	line := make([]byte, lineBytes)
	for i := 0; i < lineBytes-1; i++ {
		line[i] = byte('a' + (i % 26))
	}
	line[lineBytes-1] = '\n'
	written := 0
	for written < targetBytes {
		n, _ := f.Write(line)
		written += n
	}
	return path
}

func readRSS() int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 6 && line[:6] == "VmRSS:" {
			var kb int64
			fmt.Sscanf(line[6:], "%d", &kb)
			return kb * 1024
		}
	}
	return 0
}

func main() {
	const sizeBytes = 200 * 1024 * 1024
	const lineBytes = 256
	path := writeSyntheticFile(sizeBytes, lineBytes)
	defer os.RemoveAll(filepath.Dir(path))

	runtime.GC()
	rssBefore := readRSS()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src, _ := source.FromArgs([]string{path}, nil, "")
	stream, err := loader.Open(ctx, src, loader.Config{})
	if err != nil {
		fmt.Printf("Open error: %v\n", err)
		return
	}
	for range stream.Updates {
	}
	for range stream.Errs {
	}

	runtime.GC()
	runtime.GC()

	buf := stream.Buffer

	fmt.Printf("Total lines in buffer: %d\n", buf.Total())
	lines := buf.Slice(0, 10)
	if len(lines) > 0 {
		fmt.Printf("Line size struct: %d bytes\n", unsafe.Sizeof(lines[0]))
		fmt.Printf("Sample raw len: %d\n", len(lines[0].Raw))
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	rssAfter := readRSS()
	delta := rssAfter - rssBefore

	fmt.Printf("RSS before: %.1f MiB\n", float64(rssBefore)/1024/1024)
	fmt.Printf("RSS after:  %.1f MiB\n", float64(rssAfter)/1024/1024)
	fmt.Printf("Delta:      %.1f MiB (%.1fx file size)\n", float64(delta)/1024/1024, float64(delta)/float64(sizeBytes))
	fmt.Printf("HeapInuse:  %.1f MiB\n", float64(ms.HeapInuse)/1024/1024)
	fmt.Printf("HeapAlloc:  %.1f MiB\n", float64(ms.HeapAlloc)/1024/1024)
	fmt.Printf("HeapSys:    %.1f MiB\n", float64(ms.HeapSys)/1024/1024)
	fmt.Printf("HeapReleased: %.1f MiB\n", float64(ms.HeapReleased)/1024/1024)
	fmt.Printf("Sys:        %.1f MiB\n", float64(ms.Sys)/1024/1024)

	heapFile, _ := os.Create("/tmp/heap.pprof")
	defer heapFile.Close()
	pprof.WriteHeapProfile(heapFile)
	fmt.Println("Heap profile written to /tmp/heap.pprof")
}
