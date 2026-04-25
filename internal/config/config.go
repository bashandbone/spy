// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package config

type Config struct {
	Theme         string
	LineNumbers   bool
	WordWrap      bool
	TabWidth      int
	ShowStatusBar bool
	MaxLineWidth  int
}

func NewConfig() *Config {
	return &Config{
		Theme:         "monokai",
		LineNumbers:   true,
		WordWrap:      true,
		TabWidth:      4,
		ShowStatusBar: true,
		MaxLineWidth:  120,
	}
}
