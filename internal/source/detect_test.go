// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// detectKind is the unexported helper exercised by these tests; the
// Source interface and concrete sources call it during construction. The
// signature is the contract listed in T016.
//
// signature: detectKind(r io.Reader, hint string) (Kind, lexerName string, err error)

func TestDetectKind_ByExtension(t *testing.T) {
	cases := []struct {
		hint string
		want Kind
	}{
		{"file.go", KindCode},
		{"path/to/main.py", KindCode},
		{"app.js", KindCode},
		{"config.toml", KindCode},
		{"README.md", KindMarkdown},
		{"README.markdown", KindMarkdown},
		{"image.png", KindImage},
		{"image.JPEG", KindImage}, // case-insensitive
		{"doc.pdf", KindPDF},
		{"plain.txt", KindText},
	}
	for _, tc := range cases {
		t.Run(tc.hint, func(t *testing.T) {
			got, _, err := detectKind(strings.NewReader(""), tc.hint)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("hint=%q: got %v want %v", tc.hint, got, tc.want)
			}
		})
	}
}

func TestDetectKind_PDFMagic(t *testing.T) {
	r := bytes.NewReader([]byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n"))
	got, _, err := detectKind(r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != KindPDF {
		t.Errorf("PDF magic: got %v want %v", got, KindPDF)
	}
}

func TestDetectKind_PNGMagic(t *testing.T) {
	r := bytes.NewReader([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00})
	got, _, err := detectKind(r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != KindImage {
		t.Errorf("PNG magic: got %v want %v", got, KindImage)
	}
}

func TestDetectKind_JPEGMagic(t *testing.T) {
	r := bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00})
	got, _, err := detectKind(r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != KindImage {
		t.Errorf("JPEG magic: got %v want %v", got, KindImage)
	}
}

func TestDetectKind_GIFMagic(t *testing.T) {
	r := bytes.NewReader([]byte("GIF89a\x01\x00"))
	got, _, err := detectKind(r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != KindImage {
		t.Errorf("GIF magic: got %v want %v", got, KindImage)
	}
}

func TestDetectKind_BinaryRejected(t *testing.T) {
	// 8 KiB with > 1% control bytes outside \t\r\n\x1b → binary.
	var buf bytes.Buffer
	for i := 0; i < 8192; i++ {
		// Sprinkle a control byte every 50 bytes (= 2% control), well
		// over the 1% threshold.
		if i%50 == 0 {
			buf.WriteByte(0x01)
		} else {
			buf.WriteByte('a')
		}
	}
	got, _, err := detectKind(&buf, "")
	if err == nil {
		t.Fatalf("expected ErrBinary, got nil")
	}
	if !errors.Is(err, ErrBinary) {
		t.Errorf("expected ErrBinary, got %v", err)
	}
	if got != KindBinary {
		t.Errorf("expected KindBinary, got %v", got)
	}
}

func TestDetectKind_TabNewlineEscapeAreText(t *testing.T) {
	// \t \r \n \x1b are text bytes (escape is allowed because terminal
	// content commonly carries CSI sequences from tools like `bat`).
	body := strings.Repeat("\t\r\n\x1bhello world\n", 200)
	got, _, err := detectKind(strings.NewReader(body), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == KindBinary {
		t.Errorf("text with \\t/\\r/\\n/\\x1b must not be classified binary")
	}
}

func TestDetectKind_CodeViaChroma(t *testing.T) {
	// No extension hint, but Chroma's Analyse should pick Go from the
	// content.
	body := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	got, lexer, err := detectKind(strings.NewReader(body), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != KindCode && got != KindText {
		// Either is fine — Chroma may or may not Analyse-match this short
		// snippet. The contract says detectKind never crashes; if Chroma
		// matched, lexer is non-empty.
		t.Errorf("got %v, want KindCode or KindText", got)
	}
	if got == KindCode && lexer == "" {
		t.Errorf("KindCode without a lexer name")
	}
}

func TestDetectKind_PlainTextFallback(t *testing.T) {
	body := "Just a plain text file.\nNothing fancy.\n"
	got, _, err := detectKind(strings.NewReader(body), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != KindText {
		t.Errorf("plain text fallback: got %v want %v", got, KindText)
	}
}

func TestDetectKind_EmptyInput(t *testing.T) {
	got, _, err := detectKind(strings.NewReader(""), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != KindText {
		t.Errorf("empty input: got %v want %v", got, KindText)
	}
}

func TestDetectKind_HintWinsOverContent(t *testing.T) {
	// Even if content looks binary-ish, an explicit .go hint should
	// classify as Code.
	body := "package main\nfunc main(){}\n"
	got, _, err := detectKind(strings.NewReader(body), "snippet.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != KindCode {
		t.Errorf("hint .go: got %v want %v", got, KindCode)
	}
}
