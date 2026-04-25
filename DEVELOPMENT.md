# Development Guide for spy

## Quick Start

### Build the project

```bash
make build
# or
go build -o bin/spy ./cmd/spy
```

### Run the application

```bash
# Start with welcome screen
make run

# Or open a specific file
./bin/spy path/to/file.go
```

### Development workflow

```bash
# Format code
make fmt

# Run linter
make vet

# Run tests (when added)
make test

# Build and run
make dev
```

## Project Architecture

### Package Structure

- **cmd/spy** - Entry point with CLI flag parsing
- **internal/config** - Configuration management
- **internal/reader** - File detection and reading logic
  - Handles code, markdown, text, PDF, and image files
  - Type detection based on file extension
- **internal/renderer** - Content rendering and styling
  - Syntax highlighting integration
  - Markdown rendering
  - Terminal styling with Lip Gloss
- **internal/ui** - Bubble Tea TUI framework integration
  - Model definition
  - Keyboard navigation
  - Viewport management

### Data Flow

```
main.go
  ↓
ui.Model (Bubble Tea)
  ├─ Input: KeyMsg, WindowSizeMsg
  ├─ Update() → handle navigation, file loading
  ├─ View() → render content
  └─ Dependencies:
      ├─ reader.ReadFile() → load file content
      ├─ renderer.Render() → format content
      └─ config.Config → user settings
```

## Syntax Highlighting Status

Currently, code is rendered as plain text with line wrapping. Future enhancements:

1. Integrate Chroma v2 for syntax highlighting
2. Add support for multiple color themes
3. Implement line numbers for code files
4. Add language-specific features (e.g., bracket matching)

**Note on Syntect**: The user requested Syntect integration. Syntect is a Rust library with no Go bindings. The recommended approach:
- Use Chroma for Go-native syntax highlighting
- Or create a Rust FFI wrapper (advanced approach)
- For now, Chroma provides excellent syntax highlighting support

## Keyboard Navigation

The TUI supports:
- **vim keys**: hjkl for navigation
- **arrow keys**: standard navigation
- **page keys**: PgUp/PgDn for paging
- **home/end**: jump to beginning/end
- **o**: open file
- **q**: quit
- **?**: help

## Testing

Add tests to `internal/*/name_test.go` files:

```bash
go test -v ./... -race
```

Current test coverage: 0% (TDD approach recommended for new features)

## Dependencies

Key dependencies and their usage:

- **Bubble Tea** - Terminal UI framework
- **Lip Gloss** - Terminal styling and layout
- **Glamour** - Markdown rendering
- **Chroma** - Syntax highlighting
- **pdfcpu** - PDF file reading

Check `go.mod` for complete dependency list.

## Common Development Tasks

### Adding a new file type

1. Update `reader.FileType` enum
2. Add detection in `DetectFileType()`
3. Add reader function (e.g., `readXML()`)
4. Add case in `ReadFile()`
5. Add renderer in `renderer.go`

### Improving syntax highlighting

1. Review Chroma v2 API documentation
2. Implement proper tokenization in `renderCode()`
3. Test with various language files
4. Consider caching formatted output

### Adding configuration file support

1. Create `config.go` that reads from `$HOME/.config/spy/config.toml`
2. Parse TOML into `Config` struct
3. Merge with defaults
4. Use throughout application

## Performance Notes

- The TUI uses full screen buffering (efficient)
- File content is loaded entirely into memory (suitable for small-medium files)
- For very large files (>100MB), consider streaming/chunking

## Debugging

Enable pprof profiling:

```bash
import _ "net/http/pprof"
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Visit http://localhost:6060/debug/pprof

## Next Steps

1. **Enhanced Syntax Highlighting** - Proper Chroma integration
2. **Search** - Ctrl+F to find in file
3. **Configuration** - config file support
4. **Line Numbers** - Display line numbers for code
5. **Better Image Viewer** - ASCII art rendering for images
6. **Plugin System** - Extensible renderer architecture

## References

- [Bubble Tea Documentation](https://github.com/charmbracelet/bubbletea)
- [Lip Gloss Styling Guide](https://github.com/charmbracelet/lipgloss)
- [Glamour Markdown](https://github.com/charmbracelet/glamour)
- [Chroma Syntax Highlighting](https://github.com/alecthomas/chroma)
