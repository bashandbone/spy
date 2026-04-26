// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package term

import (
	"context"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// oscProbeBudget is the total wall-clock budget for the OSC 11
// background-color probe. Research R6 documents 50 ms; this keeps
// [Detect] inside SC-001's 100 ms total startup ceiling even when the
// probe runs in series with the other capability checks.
const oscProbeBudget = 50 * time.Millisecond

// oscReplyMaxBytes caps the OSC 11 reply we are willing to accept. The
// strict regex below would also reject anything longer, but pre-checking
// the byte count protects us from a multi-megabyte hostile reply that
// would otherwise burn the read budget before the regex sees it.
const oscReplyMaxBytes = 64

// oscReplyRegex pins the exact shape of an OSC 11 background-color
// reply per research R6 ("defensive parsing requirements"): only
// `\x1b]11;rgb:RRRR/GGGG/BBBB` followed by either BEL (`\x07`) or ST
// (`\x1b\\`). The `^…$` anchors are critical — without them an
// adversarial reply could smuggle a CSI sequence before the prefix or
// after the terminator and the parser would still extract a luminance
// instead of NaN, exposing the embedded bytes to a downstream renderer.
var oscReplyRegex = regexp.MustCompile(`^\x1b\]11;rgb:([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})(?:\x07|\x1b\\)$`)

// envReader reads a single env var. The indirection lets the test suite
// stub the lookup so [detectBackgroundLuminance] can be exercised
// deterministically without mutating the global environment.
type envReader interface {
	Getenv(key string) string
}

// envFromOS is the production [envReader]; it delegates to [os.Getenv].
type envFromOS struct{}

// Getenv satisfies [envReader] for the production path.
func (envFromOS) Getenv(key string) string { return os.Getenv(key) }

// parseOSC11Reply parses an OSC 11 background-color response and
// returns its relative luminance in [0, 1]. Returns NaN when the reply
// is missing, oversize, malformed, or contains any byte outside the
// strict OSC 11 grammar — including replies that smuggle a CSI sequence
// between the prefix and the terminator.
//
// The function is pure and never echoes any of its input; an
// adversarial reply has no path to stdout via this code, so a hostile
// or buggy emulator cannot use the probe to clear the screen,
// reposition the cursor, or rewrite the window title even when it
// hands us crafted bytes (research R6 §5).
func parseOSC11Reply(reply string) float64 {
	if reply == "" || len(reply) > oscReplyMaxBytes {
		return math.NaN()
	}
	m := oscReplyRegex.FindStringSubmatch(reply)
	if m == nil {
		return math.NaN()
	}
	r := parseHexComponent(m[1])
	g := parseHexComponent(m[2])
	b := parseHexComponent(m[3])
	if math.IsNaN(r) || math.IsNaN(g) || math.IsNaN(b) {
		return math.NaN()
	}
	// Rec. 709 luma weights — the same approximation termenv uses for
	// HasDarkBackground; staying consistent with that table keeps the
	// auto theme aligned with what termenv-aware downstream libraries
	// would have picked.
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// parseHexComponent normalizes a 1–4 digit hex value (per OSC 11's
// `rgb:RRRR/GGGG/BBBB` shape) to a [0, 1] float. Returns NaN when the
// input is empty, longer than four digits, or not parseable as a
// hexadecimal integer.
func parseHexComponent(h string) float64 {
	if h == "" || len(h) > 4 {
		return math.NaN()
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return math.NaN()
	}
	max := float64(uint64(1)<<(4*len(h)) - 1)
	return float64(n) / max
}

// ansiPalette is a 16-entry sRGB approximation of the standard ANSI
// color palette, used by [parseColorFGBG] to map a palette index to a
// luminance estimate. The values are the VGA-style defaults documented
// by xterm and accepted by every emulator that respects COLORFGBG.
var ansiPalette = [16][3]float64{
	{0.0, 0.0, 0.0},    // 0 black
	{0.5, 0.0, 0.0},    // 1 red
	{0.0, 0.5, 0.0},    // 2 green
	{0.5, 0.5, 0.0},    // 3 yellow
	{0.0, 0.0, 0.5},    // 4 blue
	{0.5, 0.0, 0.5},    // 5 magenta
	{0.0, 0.5, 0.5},    // 6 cyan
	{0.75, 0.75, 0.75}, // 7 light gray
	{0.5, 0.5, 0.5},    // 8 dark gray
	{1.0, 0.0, 0.0},    // 9 bright red
	{0.0, 1.0, 0.0},    // 10 bright green
	{1.0, 1.0, 0.0},    // 11 bright yellow
	{0.0, 0.0, 1.0},    // 12 bright blue
	{1.0, 0.0, 1.0},    // 13 bright magenta
	{0.0, 1.0, 1.0},    // 14 bright cyan
	{1.0, 1.0, 1.0},    // 15 white
}

// parseColorFGBG extracts the background palette index from a
// COLORFGBG env value (`fg;bg` or rxvt's three-field `fg;default;bg`)
// and maps it to a luminance value. Returns NaN when the value is
// empty, missing the `;` separator, has a non-numeric trailing field,
// or names an out-of-range palette index.
//
// The palette → luminance mapping reuses [ansiPalette]; the resulting
// numbers are deliberately coarse (most indices land at exactly 0.0,
// 0.5, or 1.0) because the only consumer is [render.ResolveTheme],
// which only cares whether the value is below or above 0.5.
func parseColorFGBG(v string) float64 {
	if v == "" || !strings.Contains(v, ";") {
		return math.NaN()
	}
	parts := strings.Split(v, ";")
	last := strings.TrimSpace(parts[len(parts)-1])
	n, err := strconv.Atoi(last)
	if err != nil || n < 0 || n >= len(ansiPalette) {
		return math.NaN()
	}
	rgb := ansiPalette[n]
	return 0.2126*rgb[0] + 0.7152*rgb[1] + 0.0722*rgb[2]
}

// detectBackgroundLuminance returns the terminal background luminance
// in [0, 1], or NaN when the value cannot be determined. Resolution
// order matches research R6:
//
//  1. Bypass entirely when SPY_THEME is set, NO_COLOR is set, or
//     stdout is not a TTY — in those cases the caller already knows
//     what theme to use and the probe would only burn time.
//  2. Run the OSC 11 probe via `query` (50 ms total budget). A
//     well-formed reply wins.
//  3. Fall back to COLORFGBG. A parseable value wins.
//  4. Otherwise return NaN; the caller is expected to default to dark.
//
// `query` is injected so tests can drive synthetic replies without a
// real PTY; production callers pass [probeOSC11Background], which
// performs the actual TTY round-trip.
func detectBackgroundLuminance(ctx context.Context, env envReader, isTTY bool, query func(context.Context) string) float64 {
	if env.Getenv("SPY_THEME") != "" {
		return math.NaN()
	}
	if env.Getenv("NO_COLOR") != "" {
		return math.NaN()
	}
	if !isTTY {
		return math.NaN()
	}
	if ctx.Err() != nil {
		return math.NaN()
	}
	if query != nil {
		probeCtx, cancel := context.WithTimeout(ctx, oscProbeBudget)
		defer cancel()
		if reply := query(probeCtx); reply != "" {
			if l := parseOSC11Reply(reply); !math.IsNaN(l) {
				return l
			}
		}
	}
	if l := parseColorFGBG(env.Getenv("COLORFGBG")); !math.IsNaN(l) {
		return l
	}
	return math.NaN()
}

// seenTerminator reports whether the buffered OSC reply has been
// terminated by either BEL (`\x07`) or ST (`\x1b\\`). The platform
// probe uses this to stop reading the moment the reply is complete so
// it doesn't accidentally consume the user's next keystroke.
func seenTerminator(buf []byte) bool {
	if len(buf) == 0 {
		return false
	}
	if buf[len(buf)-1] == 0x07 {
		return true
	}
	if len(buf) >= 2 && buf[len(buf)-2] == 0x1b && buf[len(buf)-1] == '\\' {
		return true
	}
	return false
}

// raceReadOSCReply runs [readOSCReply] in a goroutine and races its
// completion against `ctx.Done()`. Returns the goroutine's buffer when
// the read finishes first, nil when the context fires first. Extracting
// this race here keeps the platform-specific [probeOSC11Background]
// thin enough that the only untestable lines are the calls that
// genuinely need a real /dev/tty (open + raw-mode switch + write).
func raceReadOSCReply(ctx context.Context, r io.Reader) []byte {
	ch := make(chan []byte, 1)
	go func() {
		ch <- readOSCReply(ctx, r)
	}()
	select {
	case buf := <-ch:
		return buf
	case <-ctx.Done():
		return nil
	}
}

// readOSCReply pulls bytes from `r` one at a time until any of:
//   - [seenTerminator] is satisfied,
//   - the buffer reaches [oscReplyMaxBytes],
//   - Read returns an error or zero bytes, or
//   - `ctx` is cancelled between Reads.
//
// One byte at a time matters when `r` is a real terminal FD (the
// /dev/tty production path): a bulk Read can over-consume past the OSC
// terminator and swallow whatever the user typed immediately after,
// silently dropping a keystroke into the void (Copilot review PR#10
// round-2 #2). The byte-at-a-time loop stops exactly at BEL/ST so any
// trailing bytes stay on the FD for the next reader.
//
// Returns the accumulated bytes (nil when the very first read failed).
// Extracting this loop here lets us drive it from unit tests with a
// mock io.Reader, keeping the platform-specific
// [probeOSC11Background] thin enough that the lines that *do* require
// a real /dev/tty (open + raw-mode switch) are the only untested ones.
func readOSCReply(ctx context.Context, r io.Reader) []byte {
	out := make([]byte, 0, oscReplyMaxBytes)
	var buf [1]byte
	for len(out) < oscReplyMaxBytes {
		select {
		case <-ctx.Done():
			if len(out) == 0 {
				return nil
			}
			return out
		default:
		}
		n, err := r.Read(buf[:])
		if err != nil || n <= 0 {
			break
		}
		out = append(out, buf[0])
		if seenTerminator(out) {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
