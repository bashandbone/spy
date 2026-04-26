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

// encodeITerm2 wraps `img` in the iTerm2 inline-image escape sequence
// per https://iterm2.com/documentation-images.html. The terminator is
// BEL (`\x07`) — iTerm2's default, also accepted by WezTerm.
//
// preserveAspectRatio=1 keeps non-square images from squashing when the
// terminal grid forces a particular cell aspect; inline=1 displays the
// image immediately rather than offering it as a download.
func encodeITerm2(img image.Image) (string, error) {
	if img == nil {
		return "", fmt.Errorf("iterm2: nil image")
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return "", fmt.Errorf("iterm2: encode png: %w", err)
	}
	payload := base64.StdEncoding.EncodeToString(pngBuf.Bytes())
	return fmt.Sprintf("\x1b]1337;File=inline=1;preserveAspectRatio=1:%s\x07", payload), nil
}
