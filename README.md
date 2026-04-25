# spy - GUI File Viewer

A beautiful, keyboard-driven file viewer for the terminal. Think "GUI `bat`" with syntax highlighting, PDF support, and image preview capabilities.

Built with:
- **Bubble Tea** - TUI framework
- **Lip Gloss** - Terminal styling
- **Chroma** - Syntax highlighting
- **Glamour** - Markdown rendering
- **pdfcpu** - PDF handling

## Features

- 🎨 **Syntax Highlighting** - Beautiful code highlighting for 100+ languages
- 📄 **PDF Support** - View PDF metadata and extract text
- 🖼️ **Image Support** - Preview images with dimension info
- 📝 **Markdown Rendering** - Beautifully formatted markdown
- ⌨️ **Vim-style Navigation** - hjkl keys, Home/End, Page Up/Down
- 🎯 **Status Bar** - File info and navigation hints
- 🌙 **Themable** - Configurable syntax highlighting themes

## Installation

```bash
go install github.com/knitli/spy/cmd/spy@latest
```

Or build from source:

```bash
git clone https://github.com/knitli/spy.git
cd spy
go build -o bin/spy ./cmd/spy
./bin/spy [FILE]
```

## Usage

```bash
spy                    # Start with welcome screen
spy path/to/file.go   # Open a file
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `q`, `Ctrl+C` | Quit |
| `o` | Open file dialog |
| `↑`/`↓` or `k`/`j` | Scroll up/down |
| `Home` | Jump to start |
| `End` | Jump to end |
| `PgUp`/`PgDn` | Page up/down |
| `?` | Toggle help |

## Project Structure

```
spy/
├── cmd/
│   └── spy/
│       └── main.go          # Entry point
├── internal/
│   ├── config/
│   │   └── config.go        # Configuration management
│   ├── reader/
│   │   └── reader.go        # File reading and type detection
│   ├── renderer/
│   │   └── renderer.go      # Content rendering and styling
│   └── ui/
│       └── model.go         # Bubble Tea model and UI logic
├── go.mod
├── go.sum
└── README.md
```

## Configuration

Configuration is managed in `internal/config/config.go`. Future versions will support config files.

Current settings:
- **Theme**: monokai (Chroma syntax highlighting theme)
- **Line Numbers**: enabled
- **Word Wrap**: enabled
- **Tab Width**: 4 spaces
- **Status Bar**: enabled

## Supported File Types

- **Code**: Go, Python, Rust, JavaScript, TypeScript, Java, C, C++, C#, Ruby, PHP, Swift, Kotlin, Scala, Lisp, Clojure, SQL, Bash, Lua, JSON, YAML, TOML, XML, HTML, CSS, SCSS
- **Markup**: Markdown
- **Documents**: PDF
- **Images**: PNG, JPG, GIF, BMP, WebP
- **Text**: Any plain text file

## Development

### Building

```bash
go build -o bin/spy ./cmd/spy
```

### Running

```bash
./bin/spy path/to/file
```

### Dependencies

- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/charmbracelet/lipgloss` - Terminal styling
- `github.com/charmbracelet/glamour` - Markdown rendering
- `github.com/alecthomas/chroma/v2` - Syntax highlighting
- `github.com/pdfcpu/pdfcpu` - PDF handling

## Roadmap

- [ ] Configuration file support
- [ ] Search within file (Ctrl+F)
- [ ] Line navigation (Ctrl+G, go to line)
- [ ] Drag and drop file opening
- [ ] Theme switcher
- [ ] Wider color support
- [ ] PDF text extraction
- [ ] Image viewer improvements
- [ ] Plugin system

## License

MIT

## Author

Created with ❤️ for developers who love beautiful CLIs
