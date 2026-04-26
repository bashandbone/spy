// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// memprof is a developer utility that generates a 200 MiB synthetic
// text file, streams it through the loader pipeline, and reports
// OS-level RSS deltas alongside Go heap stats. Run it to reproduce
// or validate SC-005 memory budgets without the overhead of the full
// test suite.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"unsafe"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
)

// writeSyntheticFile creates a temporary file of targetBytes filled
// with lineBytes-wide lines. Returns the file path and the directory
// that should be removed by the caller when done.
func writeSyntheticFile(targetBytes, lineBytes int) (path string, dir string, err error) {
	dir, err = os.MkdirTemp("", "spy-perf")
	if err != nil {
		return "", "", fmt.Errorf("MkdirTemp: %w", err)
	}
	path = filepath.Join(dir, "synthetic.txt")
	f, err := os.Create(path)
	if err != nil {
		return "", dir, fmt.Errorf("create %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, cerr)
		}
	}()
	line := make([]byte, lineBytes)
	for i := 0; i < lineBytes-1; i++ {
		line[i] = byte('a' + (i % 26))
	}
	line[lineBytes-1] = '\n'
	written := 0
	for written < targetBytes {
		n, werr := f.Write(line)
		if werr != nil {
			return "", dir, fmt.Errorf("write: %w", werr)
		}
		if n == 0 {
			return "", dir, fmt.Errorf("write returned 0 bytes")
		}
		written += n
	}
	return path, dir, nil
}

// readRSS returns the OS-reported resident set size on Linux by reading
// VmRSS from /proc/self/status. Returns (0, error) on any failure so
// callers can detect an invalid reading rather than silently using 0.
func readRSS() (int64, error) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, fmt.Errorf("open /proc/self/status: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 6 && line[:6] == "VmRSS:" {
			var kb int64
			if n, serr := fmt.Sscanf(line[6:], "%d", &kb); serr != nil || n != 1 {
				return 0, fmt.Errorf("parse VmRSS line %q: %w", line, serr)
			}
			return kb * 1024, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan /proc/self/status: %w", err)
	}
	return 0, fmt.Errorf("VmRSS not found in /proc/self/status (unsupported platform?)")
}

func main() {
	const sizeBytes = 200 * 1024 * 1024
	const lineBytes = 256

	path, dir, err := writeSyntheticFile(sizeBytes, lineBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "writeSyntheticFile: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	debug.FreeOSMemory()
	rssBefore, err := readRSS()
	if err != nil {
		fmt.Fprintf(os.Stderr, "readRSS (before): %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src, err := source.FromArgs([]string{path}, nil, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "source.FromArgs: %v\n", err)
		os.Exit(1)
	}
	stream, err := loader.Open(ctx, src, loader.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "loader.Open: %v\n", err)
		os.Exit(1)
	}
	for range stream.Updates {
	}
	hadLoaderErr := false
	for loaderErr := range stream.Errs {
		hadLoaderErr = true
		fmt.Fprintf(os.Stderr, "loader error: %v\n", loaderErr)
	}
	if hadLoaderErr {
		fmt.Fprintln(os.Stderr, "aborting memory report: load completed with errors")
		os.Exit(1)
	}

	debug.FreeOSMemory()
	rssAfter, err := readRSS()
	if err != nil {
		fmt.Fprintf(os.Stderr, "readRSS (after): %v\n", err)
		os.Exit(1)
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	buf := stream.Buffer
	fmt.Printf("Total lines in buffer: %d\n", buf.Total())
	lines := buf.Slice(0, 10)
	if len(lines) > 0 {
		fmt.Printf("Line size struct: %d bytes\n", unsafe.Sizeof(lines[0]))
		fmt.Printf("Sample raw len: %d\n", len(lines[0].Raw))
	}

	delta := rssAfter - rssBefore
	fmt.Printf("RSS before: %.1f MiB\n", float64(rssBefore)/1024/1024)
	fmt.Printf("RSS after:  %.1f MiB\n", float64(rssAfter)/1024/1024)
	fmt.Printf("Delta:      %.1f MiB (%.1fx file size)\n", float64(delta)/1024/1024, float64(delta)/float64(sizeBytes))
	fmt.Printf("HeapInuse:  %.1f MiB\n", float64(ms.HeapInuse)/1024/1024)
	fmt.Printf("HeapAlloc:  %.1f MiB\n", float64(ms.HeapAlloc)/1024/1024)
	fmt.Printf("HeapSys:    %.1f MiB\n", float64(ms.HeapSys)/1024/1024)
	fmt.Printf("HeapReleased: %.1f MiB\n", float64(ms.HeapReleased)/1024/1024)
	fmt.Printf("Sys:        %.1f MiB\n", float64(ms.Sys)/1024/1024)

	heapFile, err := os.CreateTemp("", "heap-*.pprof")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create heap profile: %v\n", err)
		os.Exit(1)
	}
	heapProfilePath := heapFile.Name()
	if werr := pprof.WriteHeapProfile(heapFile); werr != nil {
		heapFile.Close()
		os.Remove(heapProfilePath)
		fmt.Fprintf(os.Stderr, "write heap profile: %v\n", werr)
		os.Exit(1)
	}
	if cerr := heapFile.Close(); cerr != nil {
		os.Remove(heapProfilePath)
		fmt.Fprintf(os.Stderr, "close heap profile: %v\n", cerr)
		os.Exit(1)
	}
	fmt.Printf("Heap profile written to %s\n", heapProfilePath)

	runtime.KeepAlive(stream)
}
