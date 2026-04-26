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
type imageRenderer struct {
	deps Dependencies
	src  source.Source

	// cachedFrame stores the last successful render keyed by protocol
	// + frame size. Re-rendering the same image on every viewport tick
	// would re-encode the PNG / sixel which is expensive on every key
	// press; the cache invalidates whenever the protocol or dimensions
	// change. Note this is the *encoded escape stream* — not the raw
	// image — so the resident memory cost is the same order as the
	// transmitted bytes (10s of KB for typical screenshots).
	cachedFrame  string
	cachedProto  term.Graphics
	cachedCols   int
	cachedRows   int
	cachedFailed bool
}

// newImageRenderer wires the per-source state. Returning a fresh
// renderer per [source.Source] means the cache state can't leak across
// `:open` swaps.
func newImageRenderer(deps Dependencies, src source.Source) *imageRenderer {
	return &imageRenderer{deps: deps, src: src}
}

func (r *imageRenderer) Render(ctx RenderContext) string {
	if r.src == nil {
		return r.metadataBlock(ctx, "no source attached")
	}
	proto := ctx.Capabilities.Graphics
	cols := ctx.Capabilities.Cols
	rows := ctx.Capabilities.Rows
	if proto == term.GraphicsNone {
		return r.metadataBlock(ctx, "")
	}
	// Reuse a cached frame when the protocol and dimensions haven't
	// changed. The renderer is otherwise re-invoked on every chunk and
	// every key press — encoding a multi-MB PNG each time is wasteful
	// and causes audible latency on slow terminals.
	if !r.cachedFailed && r.cachedFrame != "" &&
		r.cachedProto == proto && r.cachedCols == cols && r.cachedRows == rows {
		return r.cachedFrame
	}
	img, err := r.decode()
	if err != nil {
		r.cachedFailed = true
		return r.metadataBlock(ctx, fmt.Sprintf("decode failed: %v", err))
	}
	out, err := graphics.Render(proto, img, cols, rows)
	if err != nil {
		r.cachedFailed = true
		return r.metadataBlock(ctx, fmt.Sprintf("encode failed: %v", err))
	}
	if out == "" {
		// graphics.Render returns empty for unknown protocols; treat the
		// same as the GraphicsNone branch so we still show metadata.
		return r.metadataBlock(ctx, "")
	}
	r.cachedFrame = out
	r.cachedProto = proto
	r.cachedCols = cols
	r.cachedRows = rows
	r.cachedFailed = false
	return out
}

func (r *imageRenderer) RowToLine(_ RenderContext, _ int) int64 { return 0 }

// decode opens the source fresh and decodes via the standard image
// decoders. The decoders are registered via blank imports above.
func (r *imageRenderer) decode() (image.Image, error) {
	rc, err := r.src.Open()
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer rc.Close()
	img, _, err := image.Decode(rc)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return img, nil
}

// metadataBlock formats the fallback message displayed when graphics
// are unavailable or decoding fails. The block is deterministic so the
// integration tests can assert against it without flake.
func (r *imageRenderer) metadataBlock(_ RenderContext, note string) string {
	if r.src == nil {
		return "[image: no source attached]\n"
	}
	md := r.src.Metadata()
	dims := r.dimensions()
	var b strings.Builder
	fmt.Fprintf(&b, "[image: %s]\n", r.src.DisplayName())
	if dims != "" {
		fmt.Fprintf(&b, "  dimensions: %s\n", dims)
	}
	if md.Size > 0 {
		fmt.Fprintf(&b, "  size: %s\n", humanSize(md.Size))
	}
	if md.Path != "" && md.Path != r.src.DisplayName() {
		fmt.Fprintf(&b, "  path: %s\n", md.Path)
	}
	// The fallback message branches on whether `note` was set: a
	// non-empty note means the renderer hit a real processing error
	// (decode / encode failed) — claiming the terminal lacks support
	// is misleading in that case (Copilot review PR#11 #7).
	if note != "" {
		fmt.Fprintf(&b, "  fallback: unable to render inline image\n")
		fmt.Fprintf(&b, "  note: %s\n", note)
	} else {
		fmt.Fprintf(&b, "  fallback: terminal lacks inline-image support\n")
	}
	return b.String()
}

// dimensions reads the source bytes once and pulls width / height from
// the image metadata via [image.DecodeConfig] (cheap — no full
// rasterization). Returns empty on any error.
func (r *imageRenderer) dimensions() string {
	if r.src == nil {
		return ""
	}
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
