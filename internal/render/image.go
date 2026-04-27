// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package render

import (
	"fmt"
	"image"
	_ "image/gif"  // register decoder
	_ "image/jpeg" // register decoder
	_ "image/png"  // register decoder
	"strings"

	_ "golang.org/x/image/bmp"  // register decoder
	_ "golang.org/x/image/webp" // register decoder

	"github.com/knitli/spy/internal/graphics"
	"github.com/knitli/spy/internal/source"
	"github.com/knitli/spy/internal/term"
)

// imageRenderer draws image sources. On graphics-capable terminals the
// raw image bytes are routed through [graphics.Render] for the active
// protocol; otherwise a deterministic metadata block is shown so the
// user knows the file type and dimensions even without inline graphics.
//
// Per research R2 the source bytes are re-read at render time rather
// than caching the decoded [image.Image] across frames — large GIFs
// would otherwise blow the SC-005 500 MB ceiling.
//
// Bubble Tea v2's cell-based diff renderer (ultraviolet) strips APC
// escape sequences (Kitty, iTerm2 inline images) when it parses View
// content into cells — APC sequences have zero display width and their
// bytes are never written to the PTY. To work around this, Render()
// returns blank viewport content when a graphics protocol is active;
// the actual APC escape sequence is exposed via GraphicsRaw() so the
// caller can emit it via tea.Raw(), which bypasses the cell renderer.
type imageRenderer struct {
	deps Dependencies
	src  source.Source

	// cachedFrame is the last rendered frame keyed by (proto, cols,
	// rows). Stored regardless of whether the graphics path succeeded
	// or fell back to the metadata block — both outputs are stable
	// for a given key, so repeat renders (every key press is one)
	// hit the cache instead of re-decoding / re-encoding the PNG or
	// re-stat'ing the source for the metadata block (Copilot review
	// PR#11 round-3).
	cachedFrame string
	// cachedPayload holds the raw APC / graphics-protocol escape
	// sequence for the last successful non-None render. It is set by
	// Render() and read by GraphicsRaw(). Empty when proto is None or
	// when encoding failed (fallback to metadata block fires instead).
	cachedPayload string
	cachedProto   term.Graphics
	cachedCols    int
	cachedRows    int
	cacheValid    bool
}

// newImageRenderer wires the per-source state. Returning a fresh
// renderer per [source.Source] means the cache state can't leak across
// `:open` swaps.
func newImageRenderer(deps Dependencies, src source.Source) *imageRenderer {
	return &imageRenderer{deps: deps, src: src}
}

func (r *imageRenderer) Render(ctx RenderContext) string {
	if r.src == nil {
		// nil-source placeholder is constant — no need to cache.
		return r.metadataBlock(ctx, "no source attached")
	}
	proto := ctx.Capabilities.Graphics
	cols := ctx.Capabilities.Cols
	rows := ctx.Capabilities.Rows

	// Cache hit: the frame for this (proto, cols, rows) was already
	// computed (success OR fallback). Repeat renders (every key
	// press triggers one) skip both the graphics encode AND the
	// metadata-block stat'ing — both are expensive on slow disks /
	// terminals and produce a deterministic result for a stable key.
	if r.cacheValid && r.cachedProto == proto &&
		r.cachedCols == cols && r.cachedRows == rows {
		return r.cachedFrame
	}

	out, payload := r.renderFresh(ctx, proto, cols, rows)
	r.cachedFrame = out
	r.cachedPayload = payload
	r.cachedProto = proto
	r.cachedCols = cols
	r.cachedRows = rows
	r.cacheValid = true
	return out
}

// GraphicsRaw returns the raw APC / graphics-protocol escape sequence
// for the last successful Render call. Empty when the active protocol
// is None (or when encoding failed and the metadata-block fallback
// fired instead). The caller is responsible for emitting this via
// tea.Raw() so it bypasses Bubble Tea v2's cell renderer.
func (r *imageRenderer) GraphicsRaw(_ RenderContext) string {
	return r.cachedPayload
}

// renderFresh produces the viewport content and raw protocol payload for
// the given key without touching the cache. The graphics path is
// short-circuited on GraphicsNone so the fallback fires before the
// renderer pays the open / decode cost.
//
// Return values: (viewportContent, rawPayload). For GraphicsNone and
// error paths, rawPayload is empty and viewportContent is the metadata
// block. For active graphics protocols, viewportContent is an empty
// string (the viewport stays blank; the image is displayed by the
// terminal via the APC escape) and rawPayload holds the protocol bytes
// to emit via tea.Raw().
func (r *imageRenderer) renderFresh(ctx RenderContext, proto term.Graphics, cols, rows int) (string, string) {
	if proto == term.GraphicsNone {
		return r.metadataBlock(ctx, ""), ""
	}
	img, err := r.decode()
	if err != nil {
		return r.metadataBlock(ctx, fmt.Sprintf("decode failed: %v", err)), ""
	}
	out, err := graphics.Render(proto, img, cols, rows)
	if err != nil {
		return r.metadataBlock(ctx, fmt.Sprintf("encode failed: %v", err)), ""
	}
	if out == "" {
		// graphics.Render returns empty for unknown protocols; treat the
		// same as the GraphicsNone branch so we still show metadata.
		return r.metadataBlock(ctx, ""), ""
	}
	// The raw APC sequence is returned as the payload for tea.Raw();
	// the viewport content is left blank so the terminal renders the
	// image overlay without interference from cell content.
	return "", out
}

func (r *imageRenderer) RowToLine(_ RenderContext, _ int) int64 { return 0 }

// decode opens the source fresh and decodes via the standard image
// decoders. The decoders are registered via blank imports above.
//
// `image.Decode` is the trust boundary for attacker-controlled image
// bytes. The stdlib PNG/JPEG/GIF decoders are panic-safe in the
// general case, but third-party `golang.org/x/image/{bmp,webp}`
// decoders have triggered runtime panics on malformed input
// historically. The deferred recover here keeps `spy <evil.png>`
// from tearing down the alt-screen — surfaced as
// [ErrUnsupportedDecoder] so the metadata-fallback block fires.
func (r *imageRenderer) decode() (img image.Image, err error) {
	rc, ferr := r.src.Open()
	if ferr != nil {
		return nil, fmt.Errorf("open: %w", ferr)
	}
	defer rc.Close()
	defer func() {
		if rec := recover(); rec != nil {
			img = nil
			err = fmt.Errorf("%w: image decoder panicked: %v", ErrUnsupportedDecoder, rec)
		}
	}()
	im, _, derr := image.Decode(rc)
	if derr != nil {
		return nil, fmt.Errorf("decode: %w", derr)
	}
	return im, nil
}

// metadataBlock formats the fallback message displayed when graphics
// are unavailable or decoding fails. The block is deterministic so the
// integration tests can assert against it without flake.
//
// DisplayName, the source path, and the optional `note` all pass
// through [Neutralize] so an attacker-controlled filename containing
// OSC payload bytes cannot reach the terminal. Acceptance review C4.
func (r *imageRenderer) metadataBlock(_ RenderContext, note string) string {
	if r.src == nil {
		return "[image: no source attached]\n"
	}
	md := r.src.Metadata()
	dims := r.dimensions()
	name := Neutralize(r.src.DisplayName())
	var b strings.Builder
	fmt.Fprintf(&b, "[image: %s]\n", name)
	if dims != "" {
		fmt.Fprintf(&b, "  dimensions: %s\n", dims)
	}
	if md.Size > 0 {
		fmt.Fprintf(&b, "  size: %s\n", humanSize(md.Size))
	}
	if md.Path != "" && md.Path != r.src.DisplayName() {
		fmt.Fprintf(&b, "  path: %s\n", Neutralize(md.Path))
	}
	// The fallback message branches on whether `note` was set: a
	// non-empty note means the renderer hit a real processing error
	// (decode / encode failed) — claiming the terminal lacks support
	// is misleading in that case (Copilot review PR#11 #7).
	if note != "" {
		fmt.Fprintf(&b, "  fallback: unable to render inline image\n")
		fmt.Fprintf(&b, "  note: %s\n", Neutralize(note))
	} else {
		fmt.Fprintf(&b, "  fallback: terminal lacks inline-image support\n")
	}
	return b.String()
}

// dimensions reads the source bytes once and pulls width / height from
// the image metadata via [image.DecodeConfig] (cheap — no full
// rasterization). Returns empty on any error.
//
// Like [imageRenderer.decode], the deferred recover here keeps a
// hostile / corrupt blob from tearing down the program — the
// metadata block falls back to "" dimensions instead of crashing.
func (r *imageRenderer) dimensions() (s string) {
	if r.src == nil {
		return ""
	}
	defer func() {
		if rec := recover(); rec != nil {
			s = ""
		}
	}()
	rc, err := r.src.Open()
	if err != nil {
		return ""
	}
	defer rc.Close()
	cfg, _, err := image.DecodeConfig(rc)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d×%d", cfg.Width, cfg.Height)
}

// humanSize is a byte-pretty printer kept local to this file so the
// formatting logic stays next to the image renderer rather than
// introducing another shared helper.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n2 := n / unit; n2 >= unit; n2 /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
