// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package reader

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

type FileType int

const (
	FileTypeUnknown FileType = iota
	FileTypeCode
	FileTypeMarkdown
	FileTypeText
	FileTypePDF
	FileTypeImage
)

type Content struct {
	Type        FileType
	FilePath    string
	RawContent  string
	PageCount   int
	CurrentPage int
	Metadata    map[string]string
}

func DetectFileType(filePath string) FileType {
	if filePath == "" {
		return FileTypeUnknown
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	codeExts := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true,
		".jsx": true, ".tsx": true, ".rs": true, ".c": true,
		".cpp": true, ".h": true, ".java": true, ".cs": true,
		".rb": true, ".php": true, ".swift": true, ".kt": true,
		".scala": true, ".clj": true, ".lisp": true, ".sql": true,
		".bash": true, ".sh": true, ".zsh": true, ".lua": true,
		".json": true, ".yaml": true, ".yml": true, ".toml": true,
		".xml": true, ".html": true, ".css": true, ".scss": true,
	}

	if codeExts[ext] {
		return FileTypeCode
	}

	if ext == ".md" || ext == ".markdown" {
		return FileTypeMarkdown
	}

	imageExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".bmp": true, ".webp": true,
	}

	if imageExts[ext] {
		return FileTypeImage
	}

	if ext == ".pdf" {
		return FileTypePDF
	}

	return FileTypeText
}

func ReadFile(filePath string) (*Content, error) {
	if filePath == "" {
		return &Content{
			Type:       FileTypeUnknown,
			RawContent: "No file selected. Press 'o' to open a file.",
			Metadata:   make(map[string]string),
		}, nil
	}

	fileType := DetectFileType(filePath)
	content := &Content{
		Type:        fileType,
		FilePath:    filePath,
		Metadata:    make(map[string]string),
		CurrentPage: 1,
	}

	switch fileType {
	case FileTypePDF:
		return readPDF(filePath, content)
	case FileTypeImage:
		return readImage(filePath, content)
	default:
		return readTextFile(filePath, content)
	}
}

func readTextFile(filePath string, content *Content) (*Content, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content.RawContent = string(data)
	fi, _ := os.Stat(filePath)
	if fi != nil {
		content.Metadata["size"] = fmt.Sprintf("%d bytes", fi.Size())
		content.Metadata["modified"] = fi.ModTime().String()
	}

	return content, nil
}

func readPDF(filePath string, content *Content) (*Content, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	ctx, err := pdfcpu.Read(f, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}

	content.PageCount = ctx.PageCount
	content.Metadata["pages"] = fmt.Sprintf("%d", ctx.PageCount)
	content.Metadata["title"] = ctx.Title
	content.Metadata["author"] = ctx.Author
	content.RawContent = fmt.Sprintf("[PDF Document: %s]\nPages: %d\n\nAuthor: %s\n\nTitle: %s",
		filepath.Base(filePath), ctx.PageCount, ctx.Author, ctx.Title)

	return content, nil
}

func readImage(filePath string, content *Content) (*Content, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	content.Metadata["dimensions"] = fmt.Sprintf("%dx%d", config.Width, config.Height)
	fi, _ := os.Stat(filePath)
	if fi != nil {
		content.Metadata["size"] = fmt.Sprintf("%d bytes", fi.Size())
	}

	content.RawContent = fmt.Sprintf("[Image: %s]\nDimensions: %dx%d\nSize: %s\n\nUse arrow keys to navigate, 'q' to quit.",
		filepath.Base(filePath), config.Width, config.Height, content.Metadata["size"])

	return content, nil
}
