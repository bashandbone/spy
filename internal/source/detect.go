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
//  3. Chroma `Analyse` over the read-ahead buffer.
//  4. Text/binary heuristic over the first 8 KiB.
//
// The reader is consumed up to 8 KiB; callers that need to feed the
// bytes downstream should wrap the result with [io.MultiReader] using
// the buffer returned via [DetectAndPeek] (see file.go).
func detectKind(r io.Reader, hint string) (Kind, string, error) {
	// 1. Extension hint.
	if hint != "" {
		if k, lex := classifyByName(hint); k != KindUnknown {
			return k, lex, nil
		}
	}

	// Read up to 8 KiB for content-based detection.
	const peekN = 8192
	buf := make([]byte, peekN)
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

	// 3. Chroma Analyse.
	if lex := lexers.Analyse(string(buf)); lex != nil {
		cfg := lex.Config()
		// Plaintext / fallback lexer is not "code" — degrade to KindText.
		if !isPlaintextLexer(cfg.Name) {
			return KindCode, cfg.Name, nil
		}
	}

	// 4. Text fallback.
	return KindText, "", nil
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

// classifyByMagic recognises the well-known magic-byte signatures that
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
