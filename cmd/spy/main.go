// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/ui"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: spy [OPTIONS] [FILE]\n")
		fmt.Fprintf(os.Stderr, "A GUI file reader/viewer with syntax highlighting for code, PDFs, and images.\n\n")
		flag.PrintDefaults()
	}

	help := flag.Bool("help", false, "Show help message")
	version := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if *version {
		fmt.Println("spy version 0.1.0")
		os.Exit(0)
	}

	args := flag.Args()
	var filePath string
	if len(args) > 0 {
		filePath = args[0]
	}

	cfg := config.NewConfig()
	model := ui.NewModel(filePath, cfg)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running TUI: %v", err)
	}
}
