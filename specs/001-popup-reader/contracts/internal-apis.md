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
    // Open returns a reader for the source bytes. FileSource may be opened
    // multiple times (each call yields a fresh os.File). StdinSource may be
    // opened only once; subsequent calls return (nil, ErrAlreadyConsumed).
    Open() (io.ReadCloser, error)
    // Reopen returns a seekable reader for windowed-mode re-reads.
    // FileSource returns a fresh *os.File. StdinSource returns
    // (nil, ErrNotSeekable).
    Reopen() (io.ReadSeeker, error)
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
    ErrNotSeekable    = errors.New("source does not support seeking")
    ErrAlreadyConsumed = errors.New("source already consumed")
)

// LineProvider is the read-side interface the search and renderer packages
// consume. Implemented by *loader.LineBuffer (the in-memory hot region with
// optional windowing). Defining the interface in `source` keeps consumers
// independent of the loader's concrete buffer type.
type LineProvider interface {
    // Slice returns the lines in [start, end). May trigger a paging read
    // in windowed mode. Returns whatever is currently resident if either
    // bound exceeds the loaded range; callers must check len(returned).
    Slice(start, end int64) []Line

    // Total returns the total line count once known, or -1 while streaming.
    Total() int64
}

// Line and Token are defined here so all consumers (highlight, render,
// search) depend on `source` rather than each other. Token lives in
// `source` (not `highlight`) because a Line carries its tokens as a field;
// the alternative (Token in highlight) would force `source → highlight`
// and break the DAG. The Token type is intentionally agnostic to where
// the styling came from — `highlight` populates it via Chroma, but a
// future plain-text colorizer could produce the same shape. See
// data-model.md for field semantics.
type Line struct {
    Number  int64
    Raw     string
    Tokens  []Token   // nil until highlighter has run
    Wrapped []string  // cached wrap; invalidated on resize
}

type Token struct {
    Type  chroma.TokenType  // reused from chroma; semantic styling key
    Value string             // substring of Line.Raw
}
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
    Updates  <-chan Chunk      // bounded buffer (cap = Config.UpdatesBuffer, default 4); closed on EOF or error
    Errs     <-chan error      // stream of warnings and/or fatal errors; may yield multiple values; closed after Updates when no more errors remain
    Buffer   *LineBuffer       // resident lines + windowing state; lives in internal/loader (see data-model.md package map). (Acceptance review M13.)
}

// Open begins streaming. cfg.MaxResidentBytes triggers windowed mode.
// The Updates channel is bounded; producer blocks when full so the buffer
// never holds more than UpdatesBuffer chunks worth of Line.Raw bytes.
// See research.md R4 (Backpressure).
func Open(ctx context.Context, src source.Source, cfg Config) (*Stream, error)

type Config struct {
    MaxResidentBytes int64
    WindowSize       int
    InitialChunkLines int
    UpdatesBuffer     int  // chunk channel capacity; default 4
    MaxLineBytes      int64 // per-line truncation cap; default 102400 (100 KiB)
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

// SetCap adjusts HighlightCap at runtime; emits WarnHighlightDisabled on
// Warns if the new cap is below the current source size.
func (h *Highlighter) SetCap(bytes int64)

// Warns is a side channel for user-visible advisories (currently:
// "highlighting disabled (file > <cap>)"). Stays open for the lifetime
// of the highlighter / session — consumers (internal/ui) surface entries
// in the status bar with a 5 s auto-clear and drain non-blockingly each
// Update tick. Buffer 1; producer drops on full to avoid backpressure.
type Warning struct {
    Kind WarnKind
    Cap  int64
}
type WarnKind int
const (
    WarnHighlightDisabled WarnKind = iota
)
func (h *Highlighter) Warns() <-chan Warning

// Highlight runs Chroma against a single line; if the source has no known
// lexer, it returns the line unchanged with a single Token of type Text.
// Token lives in `source` (not `highlight`) so source.Line.Tokens has a
// home without forcing source → highlight; see internal-apis.md `source`
// section.
func (h *Highlighter) Highlight(lang string, line string) []source.Token

// HighlightStream consumes chunks from in and emits chunks with Tokens
// populated on out. Closes out when in is closed.
func (h *Highlighter) HighlightStream(ctx context.Context, in <-chan loader.Chunk, out chan<- loader.Chunk)
```

## `internal/graphics`

```go
package graphics

// Renderer is the per-protocol encoder selected at startup based on
// term.Capabilities. render.Dependencies.Graphics holds the chosen instance
// so render/image.go and render/pdf.go don't switch on protocol per frame.
// GraphicsNone returns a no-op Renderer whose Render produces "".
type Renderer interface {
    Render(img image.Image, cols, rows int) (string, error)
    Cleanup() string
}

// RendererFor returns the Renderer matching proto. Idempotent; returns the
// no-op Renderer for GraphicsNone.
func RendererFor(proto term.Graphics) Renderer

// Render encodes the given image into the active graphics protocol.
// Returns an empty string if proto == GraphicsNone. Convenience wrapper
// around RendererFor(proto).Render(...) for call sites that already have
// the protocol value.
func Render(proto term.Graphics, img image.Image, cols, rows int) (string, error)

// Cleanup emits any escape sequences needed to clear residual images
// (e.g., Kitty "delete all images"). Idempotent. Returns the bytes to write;
// the caller decides where (used for in-session cleanup via tea.Cmd, e.g.,
// on `:open` replacing the current source).
func Cleanup(proto term.Graphics) string

// CleanupFunc returns a closure that writes the cleanup sequence directly
// to os.Stdout. Safe to use as `defer cleanupGraphics()` in main(); fires on
// tea.Quit, signals, AND panic — the only path that survives a cgo go-fitz
// panic. No-op for GraphicsNone, GraphicsITerm2, GraphicsSixel. Idempotent.
func CleanupFunc(proto term.Graphics) func()

```

PDF rasterization lives in `internal/render` (not `internal/graphics`) and is
unexported: `rasterizePDFPage(src source.Source, page int) (image.Image, error)`
in `internal/render/pdf_fitz.go`, gated by the `fitz` build tag. The no-fitz
stub in `internal/render/pdf_nofitz.go` returns `ErrPDFGraphicsUnavailable`.
`page` is 0-indexed; there is no DPI parameter (the resolution is fixed by
go-fitz's default rasterizer). The renderer in `internal/render/pdf.go` is the
sole caller. (Acceptance review H6 — replaces the earlier fictional
`graphics.PDFPage(path, n, dpi)` signature.)

## `internal/render`

```go
package render

// RenderContext carries the per-frame state a Renderer needs.
// It is populated by `internal/ui` on every Update tick and passed into
// Render(). Defining it here (rather than reaching into ui.ViewerSession)
// keeps the dependency direction acyclic: ui → render, never render → ui.
type RenderContext struct {
    Buffer       *loader.LineBuffer  // resident lines + windowing state (lives in internal/loader; see data-model.md package map)
    Viewport     viewport.Model      // scroll position + dimensions
    Theme        Theme
    Capabilities term.Capabilities
    Search       search.State        // query, matches, current selection
    Status       Status              // Idle | Loading | Streaming | Error
    LastError    error               // populated when Status == Error
    Page         int                 // 1-indexed; non-zero only for KindPDF
}

type Status int
const (
    StatusIdle Status = iota
    StatusLoading
    StatusStreaming
    StatusError
)

type Renderer interface {
    Render(ctx RenderContext) string
    // RowToLine maps a 0-based visual row offset (relative to the rendered
    // frame's first row) to the source line number visible at that row. Used
    // by the status bar to keep the reported line number consistent with the
    // rendered gutter once word-wrap inflates one source line into multiple
    // visual rows. Returns 0 when the buffer is empty or the row is out of
    // range; the caller treats 0 as the "Line 0" footer sentinel for empty
    // input. (Acceptance review M13.)
    RowToLine(ctx RenderContext, visualRow int) int64
}

func ForKind(k source.Kind, deps Dependencies) Renderer

type Dependencies struct {
    Theme        Theme
    Capabilities term.Capabilities
    Graphics     graphics.Renderer
    Highlighter  *highlight.Highlighter

    // LineNumbers and WordWrap mirror the active config for the current
    // session; renderers branch on them per frame so toggles
    // (Ctrl-L / Ctrl-W) take effect on the next tick.
    // (Acceptance review M13 — fields below were undocumented.)
    LineNumbers bool
    WordWrap    bool

    // Language is the Chroma lexer name picked at source detection time.
    // Empty for non-code kinds; populated by `internal/ui` from
    // source.Metadata.Language before constructing the renderer.
    Language string

    // Source is the active source.Source for the session. The image
    // renderer re-opens it at render time (per research R2) so large GIFs
    // don't pin a decoded copy in memory; the PDF renderer reads it for
    // text extraction. Foundational text/code/markdown renderers ignore
    // the field and pull lines from the loader's LineBuffer instead.
    Source source.Source
}
```

**Dependency direction**: `render` imports `source`, `loader`, `term`,
`graphics`, `highlight`, `search`, and the upstream `bubbles/viewport`
type. It does NOT import `internal/ui`. `internal/ui` constructs a
`RenderContext` from its `ViewerSession` on each frame and passes it in.

## `internal/search`

```go
package search

type Match struct { Line int64; Start, End int }

// State is the per-frame view of the active search consumed by render and
// surfaced in ui. Inactive when Query == "". Field semantics match
// data-model.md SearchState; this is the canonical Go type signature.
type State struct {
    Query        string
    Direction    Direction
    Regex        bool
    CaseMode     CaseMode
    Matches      []Match
    CurrentMatch int   // -1 when no match selected
    Wrapped      bool  // last navigation wrapped around
    Pending      bool  // a background scan is still running
}

func Compile(query string, regex bool, caseMode CaseMode) (Matcher, error)

type Matcher interface {
    Find(line string) []Match
}

type Index struct {
    Matches []Match
    Wrapped bool
}

// Scan walks lines through `provider` in `dir`, starting at `from`.
// Emits matches on the returned channel; closes after the final match.
// On wrap-around, emits a synthetic Match with Line == -1 as a sentinel
// before continuing (or before close if no further matches).
func Scan(ctx context.Context, provider source.LineProvider, m Matcher, dir Direction, from int64) <-chan Match

type Direction int
const (
    DirForward Direction = iota
    DirBackward
)

type CaseMode int
const (
    CaseSmart CaseMode = iota
    CaseSensitive
    CaseInsensitive
)
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

    // BaseKeyMap is the non-vim keymap (defaults plus user [keys] overrides).
    // The Model retains this so a runtime `:set novim` can restore it
    // verbatim and `:set vim` can layer keys.WithVim on top without losing
    // user overrides. When zero/nil, NewModel uses KeyMap as the base.
    // (Acceptance review M13 — fields below were undocumented.)
    BaseKeyMap keys.KeyMap

    // Highlighter is the per-session syntax highlighter. nil disables
    // highlighting (used by the foundational text path and tests).
    Highlighter *highlight.Highlighter

    // Cancel cancels the loader's background streaming goroutine; the model
    // fires it on tea.Quit so Open's goroutine exits before the program
    // returns. Optional; nil is safe.
    Cancel context.CancelFunc
}

// Implements tea.Model: Init, Update, View.
```

## Stability boundary

These signatures are *the* boundary the implementation must respect. Internal
implementation details (channel sizes, goroutine layout inside a package,
private helpers) are free to change without coordination.
