// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package graphics

import (
	"bytes"
	"fmt"
	"image"

	"github.com/mattn/go-sixel"
)

// encodeSixel runs `img` through the mattn/go-sixel encoder. Output is
// the raw sixel byte stream — caller writes it directly to the TTY.
//
// go-sixel handles palette quantization internally; the SC-005 budget
// is respected because the encoder streams to bytes.Buffer rather than
// holding a decoded palette image in memory across the lifetime of the
// session.
func encodeSixel(img image.Image) (string, error) {
	if img == nil {
		return "", fmt.Errorf("sixel: nil image")
	}
	var out bytes.Buffer
	enc := sixel.NewEncoder(&out)
	if err := enc.Encode(img); err != nil {
		return "", fmt.Errorf("sixel: encode: %w", err)
	}
	return out.String(), nil
}
