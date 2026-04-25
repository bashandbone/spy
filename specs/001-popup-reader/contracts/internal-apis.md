<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Internal Package APIs

**Spec**: 001-popup-reader
**Audience**: implementers; not a public Go API (modules under `internal/`).

This contract pins the boundaries between internal packages so /speckit-tasks
can parallelize work across them without accidental cross-coupling.

## `internal/term`

```go
package term

type ColorDepth int
const (
    ColorMono ColorDepth = iota
    ColorANSI16
    ColorANSI256
    ColorTrueColor
)

type Graphics int
const (
    GraphicsNone Graphics = iota
    GraphicsKitty
    GraphicsITerm2
    GraphicsSixel
)

type Capabilities struct {
    IsTTY               bool
    Cols, Rows          int
    ColorDepth          ColorDepth
    Graphics            Graphics
    BackgroundLuminance float64 // NaN if unknown
    Program, Term       string
    InTmux              bool
}

// Detect probes the current process's terminal. Honors SPY_GRAPHICS,
// SPY_THEME, NO_COLOR, COLORTERM. Probes are time-bounded (≤ 100ms total).
func Detect(ctx context.Context) Capabilities

// Restore returns a function that puts the terminal back into the state
// captured at call time; safe to defer.
func Restore() func()
```

Concurrency: `Detect` is goroutine-safe; `Restore`'s returned closure is
idempotent.

## `internal/source`

```go
package source

type Kind int
const (
    KindUnknown Kind = iota
    KindCode
    KindMarkdown
    KindText
    KindPDF
    KindImage
    KindBinary
)

type Source interface {
    Kind() Kind
    DisplayName() string
    Open() (io.ReadCloser, error) // may be called more than once for FileSource
    Reopen() (io.ReadSeeker, error) // returns nil, ErrNotSeekable for stdin
    Metadata() Metadata
}

type Metadata struct {
    Path     string
    Size     int64
    LineCount int64
    PageCount int
    Modified time.Time
    Language string
    Encoding string
}

func FromArgs(args []string, stdin *os.File, hint string) (Source, error)
// errors fall into:
var (
    ErrNoInput        = errors.New("no input provided")
    ErrBinary         = errors.New("binary content")
    ErrUnsupported    = errors.New("unsupported format")
    ErrNotFound       = errors.New("file not found")
    ErrPermission     = errors.New("permission denied")
)
```

## `internal/loader`

```go
package loader

type Chunk struct {
    Lines     []source.Line
    StartLine int64
    EOF       bool
}

type Stream struct {
    First    Chunk             // populated synchronously before Stream is returned
    Updates  <-chan Chunk      // subsequent chunks; closed on EOF or error
    Errs     <-chan error      // single-value channel; closed when no more errors
}

// Open begins streaming. cfg.MaxResidentBytes triggers windowed mode.
func Open(ctx context.Context, src source.Source, cfg Config) (*Stream, error)

type Config struct {
    MaxResidentBytes int64
    WindowSize       int
    InitialChunkLines int
}
```

The first chunk is sized to ≥ `cfg.InitialChunkLines` (default = 2× viewport
height) so the first frame paints immediately. Cancellation via `ctx`
unblocks all goroutines.

## `internal/highlight`

```go
package highlight

type Highlighter struct { /* ... */ }

func New(theme *chroma.Style, depth term.ColorDepth, capBytes int64) *Highlighter

// Highlight runs Chroma against a single line; if the source has no known
// lexer, it returns the line unchanged with a single Token of type Text.
func (h *Highlighter) Highlight(lang string, line string) []source.Token

// HighlightStream consumes chunks from in and emits chunks with Tokens
// populated on out. Closes out when in is closed.
func (h *Highlighter) HighlightStream(ctx context.Context, in <-chan loader.Chunk, out chan<- loader.Chunk)
```

## `internal/graphics`

```go
package graphics

// Render encodes the given image into the active graphics protocol.
// Returns an empty string if proto == GraphicsNone.
func Render(proto term.Graphics, img image.Image, cols, rows int) (string, error)

// Cleanup emits any escape sequences needed to clear residual images
// (e.g., Kitty "delete all images"). Idempotent.
func Cleanup(proto term.Graphics) string

// PDFPage rasterizes page n (1-indexed) of an open PDF into an image.Image.
// Built only when the `fitz` build tag is present; the no-fitz stub returns
// (nil, ErrPDFGraphicsUnavailable).
func PDFPage(path string, n int, dpi float64) (image.Image, error)
```

## `internal/render`

```go
package render

type Renderer interface {
    Render(state ui.ViewerSession, viewport viewport.Model) string
}

func ForKind(k source.Kind, deps Dependencies) Renderer

type Dependencies struct {
    Theme        Theme
    Capabilities term.Capabilities
    Graphics     graphics.Renderer
    Highlighter  *highlight.Highlighter
}
```

## `internal/search`

```go
package search

type Match struct { Line int64; Start, End int }

func Compile(query string, regex bool, caseMode CaseMode) (Matcher, error)

type Matcher interface {
    Find(line string) []Match
}

type Index struct {
    Matches []Match
    Wrapped bool
}

func Scan(ctx context.Context, lines source.LineProvider, m Matcher, dir Direction, from int64) <-chan Match
```

## `internal/keys`

```go
package keys

type Action string
const (
    ActionQuit          Action = "quit"
    ActionScrollUp      Action = "scroll_up"
    /* ... full list per contracts/keys.md ... */
)

type KeyMap map[Action][]key.Binding

func Default() KeyMap
func WithVim(km KeyMap) KeyMap
func ApplyOverrides(km KeyMap, overrides map[string][]string) (KeyMap, []error)
```

## `internal/ui`

```go
package ui

type Model struct { /* ViewerSession + Bubble Tea machinery */ }

func NewModel(opts ModelOptions) Model

type ModelOptions struct {
    Source       source.Source
    Stream       *loader.Stream
    Capabilities term.Capabilities
    Config       *config.Config
    Theme        render.Theme
    KeyMap       keys.KeyMap
}

// Implements tea.Model: Init, Update, View.
```

## Stability boundary

These signatures are *the* boundary the implementation must respect. Internal
implementation details (channel sizes, goroutine layout inside a package,
private helpers) are free to change without coordination.
