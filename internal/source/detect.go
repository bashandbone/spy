// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
)

// detectKind decides the [Kind] (and, for code, the Chroma lexer name)
// for a byte stream. Order, per T016:
//  1. Extension hint (markdown/image/pdf/code).
//  2. Magic bytes (PDF / image).
//  3. Chroma `Analyze` over the read-ahead buffer.
//  4. Text/binary heuristic over the first 8 KiB.
//
// The reader is consumed up to 8 KiB during detection. Callers that
// need to read the content again must account for those consumed
// bytes; [FileSource] does this by reopening the file after detection,
// so no peek buffer needs to be replayed here.
func detectKind(r io.Reader, hint string) (Kind, string, error) {
	// 1. Extension hint.
	if hint != "" {
		if k, lex := classifyByName(hint); k != KindUnknown {
			return k, lex, nil
		}
	}

	// Read up to [detectionPeekBytes] for content-based detection.
	buf := make([]byte, detectionPeekBytes)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return KindUnknown, "", err
	}
	buf = buf[:n]

	// 2. Magic bytes.
	if k := classifyByMagic(buf); k != KindUnknown {
		return k, "", nil
	}

	// Empty input → text fallback.
	if len(buf) == 0 {
		return KindText, "", nil
	}

	// 4-pre. Binary heuristic before code analysis: Chroma sometimes
	// produces nonsense lexer matches on binary content.
	if isBinary(buf) {
		return KindBinary, "", fmt.Errorf("%w: %d control bytes in first %d", ErrBinary, controlByteCount(buf), len(buf))
	}

	// 3a. Shebang (US5 / T092). Stdin and unhinted files may carry an
	// interpreter declaration on line 1 — Chroma's `Analyze` doesn't
	// score short snippets reliably, so we look for the shebang
	// explicitly. The interpreter name is mapped to a Chroma lexer; an
	// unknown interpreter falls through to the rest of the pipeline.
	// Try the full interpreter name first (e.g. "python3" — chroma
	// recognizes it as an alias of the Python lexer) and only fall
	// back to the trimmed form on miss (Copilot review PR#12 #2).
	if interp := shebangInterpreter(buf); interp != "" {
		if lex := lexers.Get(interp); lex != nil && !isPlaintextLexer(lex.Config().Name) {
			return KindCode, lex.Config().Name, nil
		}
		if trimmed := trimVersionSuffix(interp); trimmed != "" && trimmed != interp {
			if lex := lexers.Get(trimmed); lex != nil && !isPlaintextLexer(lex.Config().Name) {
				return KindCode, lex.Config().Name, nil
			}
		}
	}

	// 3b. Chroma Analyze.
	if lex := lexers.Analyze(string(buf)); lex != nil {
		cfg := lex.Config()
		// Plaintext / fallback lexer is not "code" — degrade to KindText.
		if !isPlaintextLexer(cfg.Name) {
			return KindCode, cfg.Name, nil
		}
	}

	// 4. Text fallback.
	return KindText, "", nil
}

// shebangInterpreter inspects the first line of `buf` for a `#!` line
// and returns the interpreter basename (verbatim, including any
// trailing version digits) that a Chroma lexer name lookup will
// recognize. Returns "" when the buffer has no shebang or the line is
// empty after stripping the magic + flags.
//
// Forms handled:
//
//	#!/usr/bin/env python      → "python"
//	#!/usr/bin/env -S python3  → "python3"
//	#!/usr/local/bin/python3.11 → "python3.11"
//	#!/bin/bash                → "bash"
//	#!/usr/bin/perl -w         → "perl"
//
// The version suffix is preserved here so chroma's `lexers.Get` can
// match aliases like "python3" directly. The caller is responsible for
// the try-full-then-trim fallback via [trimVersionSuffix] (the
// detectKind shebang branch does this).
func shebangInterpreter(buf []byte) string {
	if len(buf) < 2 || buf[0] != '#' || buf[1] != '!' {
		return ""
	}
	end := bytes.IndexByte(buf, '\n')
	if end < 0 {
		end = len(buf)
	}
	line := strings.TrimSpace(string(buf[2:end]))
	if line == "" {
		return ""
	}
	// Tokenise on whitespace; drop `env` / `-S` flags so
	// `/usr/bin/env -S python3` collapses to "python3".
	fields := strings.Fields(line)
	for i, f := range fields {
		base := filepath.Base(f)
		switch base {
		case "env":
			continue
		}
		if strings.HasPrefix(f, "-") && i > 0 {
			continue
		}
		// Preserve the verbatim interpreter name so the caller can
		// try `lexers.Get("python3")` first; falling back to the
		// trimmed form only on miss is the documented contract.
		if name := strings.ToLower(base); name != "" {
			return name
		}
	}
	return ""
}

// trimVersionSuffix returns the language portion of an interpreter name
// like "python3.11" → "python", "ruby2.7" → "ruby", "node18" → "node".
// If the name has no digits the input is returned unchanged.
func trimVersionSuffix(name string) string {
	for i, r := range name {
		if r >= '0' && r <= '9' {
			if i == 0 {
				return name
			}
			return name[:i]
		}
	}
	return name
}

// classifyByName picks a Kind from a filename / hint. The hint may be a
// path, a basename, a bare extension, or a language name (e.g. "go",
// "py"). Used by both `--lang` overrides and FileSource construction.
func classifyByName(hint string) (Kind, string) {
	name := strings.ToLower(strings.TrimSpace(hint))
	if name == "" {
		return KindUnknown, ""
	}
	// Language hint without a leading dot.
	if !strings.Contains(name, ".") && !strings.Contains(name, "/") {
		if lex := lexers.Get(name); lex != nil && !isPlaintextLexer(lex.Config().Name) {
			return KindCode, lex.Config().Name
		}
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" && strings.HasPrefix(name, ".") {
		ext = name
	}
	switch ext {
	case ".md", ".markdown":
		return KindMarkdown, ""
	case ".pdf":
		return KindPDF, ""
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
		return KindImage, ""
	case ".txt", ".log":
		return KindText, ""
	case "":
		return KindUnknown, ""
	}
	// Code by extension via Chroma.
	if lex := lexers.Match(name); lex != nil && !isPlaintextLexer(lex.Config().Name) {
		return KindCode, lex.Config().Name
	}
	return KindUnknown, ""
}

// classifyByMagic recognizes the well-known magic-byte signatures that
// dominate the PDF/image space. Returns KindUnknown when no signature
// matches — the caller falls through to Chroma / heuristic detection.
func classifyByMagic(buf []byte) Kind {
	switch {
	case bytes.HasPrefix(buf, []byte("%PDF-")):
		return KindPDF
	case bytes.HasPrefix(buf, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return KindImage // PNG
	case bytes.HasPrefix(buf, []byte{0xFF, 0xD8, 0xFF}):
		return KindImage // JPEG
	case bytes.HasPrefix(buf, []byte("GIF87a")), bytes.HasPrefix(buf, []byte("GIF89a")):
		return KindImage // GIF
	case bytes.HasPrefix(buf, []byte{'B', 'M'}) && len(buf) >= 14:
		return KindImage // BMP
	case bytes.HasPrefix(buf, []byte("RIFF")) && len(buf) >= 12 && bytes.Equal(buf[8:12], []byte("WEBP")):
		return KindImage // WebP
	}
	return KindUnknown
}

// controlByteCount counts bytes outside \t \r \n \x1b in the binary
// detection window. \x1b (ESC) is allowed because terminal output
// commonly carries CSI sequences (e.g., piped through `bat`).
func controlByteCount(buf []byte) int {
	n := 0
	for _, b := range buf {
		if b < 0x20 && b != '\t' && b != '\r' && b != '\n' && b != 0x1b {
			n++
		}
	}
	return n
}

// isBinary reports whether more than 1% of the inspected bytes are
// non-printable controls outside the standard text whitespace set.
func isBinary(buf []byte) bool {
	if len(buf) == 0 {
		return false
	}
	ctl := controlByteCount(buf)
	// > 1% control bytes → binary. Multiplied form avoids floating point.
	return ctl*100 > len(buf)
}

// isPlaintextLexer reports whether the supplied Chroma lexer is the
// "fallback" / plaintext lexer rather than a real language.
func isPlaintextLexer(name string) bool {
	n := strings.ToLower(name)
	return n == "" || n == "plaintext" || n == "text" || n == "fallback"
}
