// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
)

// kittyChunkSize is the maximum payload bytes per Kitty graphics chunk
// (per https://sw.kovidgoyal.net/kitty/graphics-protocol/#transferring-pixel-data:
// "the data must be base64 encoded, in chunks no larger than 4096 bytes
// of base64 encoded data per chunk").
const kittyChunkSize = 4096

// kittyDeleteAll is the "delete all images" escape Kitty documents under
// `a=d,d=A`. Emitting it on cleanup wipes any residual frames the
// renderer left in the terminal — load-bearing when the user quits
// mid-scroll or panics through a tea.Cmd (research R10).
const kittyDeleteAll = "\x1b_Ga=d,d=A;\x1b\\"

// encodeKitty packs `img` into the Kitty graphics protocol's chunked
// base64 transmission format. The payload is a PNG (`f=100`) so we get
// lossless framing without re-implementing Kitty's RGBA layout. Output
// is the full escape stream — caller writes it directly to the TTY.
//
// The 4096 B chunk cap is the documented ceiling on *base64* bytes per
// chunk; we slice the encoded string after base64 to stay strictly
// inside the contract.
func encodeKitty(img image.Image) (string, error) {
	if img == nil {
		return "", fmt.Errorf("kitty: nil image")
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return "", fmt.Errorf("kitty: encode png: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

	var out bytes.Buffer
	for i := 0; i < len(encoded); i += kittyChunkSize {
		end := i + kittyChunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]
		// First chunk carries `a=T,f=100` (transmit + PNG); subsequent
		// chunks omit those and just carry `m=1` (more chunks). The
		// final chunk uses `m=0` to signal end-of-image.
		more := 1
		if end == len(encoded) {
			more = 0
		}
		switch {
		case i == 0 && more == 0:
			// Single-chunk image: no `m` flag at all (Kitty protocol
			// allows omitting it when the entire payload fits in one
			// frame).
			fmt.Fprintf(&out, "\x1b_Ga=T,f=100;%s\x1b\\", chunk)
		case i == 0:
			fmt.Fprintf(&out, "\x1b_Ga=T,f=100,m=%d;%s\x1b\\", more, chunk)
		default:
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
	}
	return out.String(), nil
}
