// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package term

import (
	"context"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

// --- parseOSC11Reply: well-formed cases ---

func TestParseOSC11_DarkBELTerminator(t *testing.T) {
	// rgb:0000/0000/0000 = pure black, luminance == 0.
	got := parseOSC11Reply("\x1b]11;rgb:0000/0000/0000\x07")
	if math.IsNaN(got) || got >= 0.5 {
		t.Errorf("dark BEL reply: got %v, want < 0.5", got)
	}
}

func TestParseOSC11_LightBELTerminator(t *testing.T) {
	// rgb:ffff/ffff/ffff = pure white, luminance == 1.
	got := parseOSC11Reply("\x1b]11;rgb:ffff/ffff/ffff\x07")
	if math.IsNaN(got) || got < 0.5 {
		t.Errorf("light BEL reply: got %v, want ≥ 0.5", got)
	}
}

func TestParseOSC11_LightSTTerminator(t *testing.T) {
	// ST = ESC backslash; xterm and many emulators terminate with this
	// instead of BEL.
	got := parseOSC11Reply("\x1b]11;rgb:ffff/ffff/ffff\x1b\\")
	if math.IsNaN(got) || got < 0.5 {
		t.Errorf("light ST reply: got %v, want ≥ 0.5", got)
	}
}

func TestParseOSC11_ShortHexComponentsSupported(t *testing.T) {
	// xterm spec allows 1–4 hex digits per component; "f/0/0" is valid.
	// Pure red: luminance = 0.2126 ≈ 0.21 (< 0.5 → dark).
	got := parseOSC11Reply("\x1b]11;rgb:f/0/0\x07")
	if math.IsNaN(got) {
		t.Fatalf("short-hex reply must parse, got NaN")
	}
	if got >= 0.5 {
		t.Errorf("pure red luminance: got %v, want < 0.5", got)
	}
}

// --- parseOSC11Reply: malformed / adversarial cases (T060 R6) ---

func TestParseOSC11_EmptyReturnsNaN(t *testing.T) {
	if got := parseOSC11Reply(""); !math.IsNaN(got) {
		t.Errorf("empty reply: got %v, want NaN", got)
	}
}

func TestParseOSC11_OversizeReturnsNaN(t *testing.T) {
	// 65+ bytes must be rejected per defensive-parsing rule.
	body := "\x1b]11;rgb:" + strings.Repeat("f", 70) + "\x07"
	if got := parseOSC11Reply(body); !math.IsNaN(got) {
		t.Errorf("oversize reply: got %v, want NaN", got)
	}
}

func TestParseOSC11_CSIEmbeddedRejected(t *testing.T) {
	// Adversarial reply that smuggles a CSI clear-screen sequence between
	// the OSC prefix and terminator. The strict regex must reject this
	// and the function must not echo any of the input.
	body := "\x1b]11;rgb:\x1b[2J/0/0\x07"
	if got := parseOSC11Reply(body); !math.IsNaN(got) {
		t.Errorf("CSI-embedded reply: got %v, want NaN", got)
	}
}

func TestParseOSC11_MidStreamAbortReturnsNaN(t *testing.T) {
	// Truncated reply (no terminator).
	body := "\x1b]11;rgb:1234/5678/9abc"
	if got := parseOSC11Reply(body); !math.IsNaN(got) {
		t.Errorf("truncated reply: got %v, want NaN", got)
	}
}

func TestParseOSC11_TrailingBytesRejected(t *testing.T) {
	// Extra trailing bytes after a valid reply must invalidate the whole
	// string — anchored regex won't match.
	body := "\x1b]11;rgb:ffff/ffff/ffff\x07XXXXX"
	if got := parseOSC11Reply(body); !math.IsNaN(got) {
		t.Errorf("trailing-bytes reply: got %v, want NaN", got)
	}
}

func TestParseOSC11_BadComponentLengthRejected(t *testing.T) {
	// 5+ digits in a component is invalid OSC 11.
	body := "\x1b]11;rgb:fffff/0000/0000\x07"
	if got := parseOSC11Reply(body); !math.IsNaN(got) {
		t.Errorf("5-digit component: got %v, want NaN", got)
	}
}

func TestParseOSC11_NonHexComponentRejected(t *testing.T) {
	body := "\x1b]11;rgb:zzzz/0000/0000\x07"
	if got := parseOSC11Reply(body); !math.IsNaN(got) {
		t.Errorf("non-hex component: got %v, want NaN", got)
	}
}

func TestParseOSC11_WrongOSCNumberRejected(t *testing.T) {
	// OSC 10 is foreground; a reply with that prefix must not be accepted
	// by the OSC 11 parser.
	body := "\x1b]10;rgb:ffff/ffff/ffff\x07"
	if got := parseOSC11Reply(body); !math.IsNaN(got) {
		t.Errorf("OSC 10 reply parsed as OSC 11: got %v", got)
	}
}

// --- parseColorFGBG ---

func TestParseColorFGBG_DarkBackground(t *testing.T) {
	// "15;0" → bg index 0 (black) → dark.
	got := parseColorFGBG("15;0")
	if math.IsNaN(got) || got >= 0.5 {
		t.Errorf("bg=0 (black): got %v, want < 0.5", got)
	}
}

func TestParseColorFGBG_LightBackground(t *testing.T) {
	// "0;15" → bg index 15 (white) → light.
	got := parseColorFGBG("0;15")
	if math.IsNaN(got) || got < 0.5 {
		t.Errorf("bg=15 (white): got %v, want ≥ 0.5", got)
	}
}

func TestParseColorFGBG_RxvtThreeFieldFormat(t *testing.T) {
	// rxvt may emit "fg;default;bg" — last field still wins.
	got := parseColorFGBG("0;default;15")
	if math.IsNaN(got) || got < 0.5 {
		t.Errorf("rxvt three-field: got %v, want ≥ 0.5", got)
	}
}

func TestParseColorFGBG_EmptyReturnsNaN(t *testing.T) {
	if got := parseColorFGBG(""); !math.IsNaN(got) {
		t.Errorf("empty: got %v, want NaN", got)
	}
}

func TestParseColorFGBG_NoSeparatorReturnsNaN(t *testing.T) {
	if got := parseColorFGBG("15"); !math.IsNaN(got) {
		t.Errorf("no separator: got %v, want NaN", got)
	}
}

func TestParseColorFGBG_OutOfRangeReturnsNaN(t *testing.T) {
	if got := parseColorFGBG("0;99"); !math.IsNaN(got) {
		t.Errorf("out-of-range index: got %v, want NaN", got)
	}
}

func TestParseColorFGBG_NonNumericReturnsNaN(t *testing.T) {
	if got := parseColorFGBG("0;foo"); !math.IsNaN(got) {
		t.Errorf("non-numeric: got %v, want NaN", got)
	}
}

// --- DetectBackgroundLuminance: bypass conditions ---

// fakeProbe returns the canned reply (or empty string) without actually
// touching the TTY. Used so DetectBackgroundLuminance can be exercised
// deterministically without a real PTY.
func fakeProbe(reply string) func(context.Context) string {
	return func(_ context.Context) string { return reply }
}

func TestDetectBackgroundLuminance_BypassWhenSPYTheme(t *testing.T) {
	t.Setenv("SPY_THEME", "dark")
	t.Setenv("NO_COLOR", "")
	// Even with a "light" reply queued, the bypass must short-circuit.
	got := detectBackgroundLuminance(context.Background(), envFromOS{}, true,
		fakeProbe("\x1b]11;rgb:ffff/ffff/ffff\x07"))
	if !math.IsNaN(got) {
		t.Errorf("SPY_THEME=dark: got %v, want NaN", got)
	}
}

func TestDetectBackgroundLuminance_BypassWhenNoColor(t *testing.T) {
	t.Setenv("SPY_THEME", "")
	t.Setenv("NO_COLOR", "1")
	got := detectBackgroundLuminance(context.Background(), envFromOS{}, true,
		fakeProbe("\x1b]11;rgb:ffff/ffff/ffff\x07"))
	if !math.IsNaN(got) {
		t.Errorf("NO_COLOR=1: got %v, want NaN", got)
	}
}

func TestDetectBackgroundLuminance_BypassWhenNonTTY(t *testing.T) {
	t.Setenv("SPY_THEME", "")
	t.Setenv("NO_COLOR", "")
	got := detectBackgroundLuminance(context.Background(), envFromOS{}, false,
		fakeProbe("\x1b]11;rgb:ffff/ffff/ffff\x07"))
	if !math.IsNaN(got) {
		t.Errorf("non-TTY: got %v, want NaN", got)
	}
}

// --- DetectBackgroundLuminance: probe + fallback paths ---

func TestDetectBackgroundLuminance_OSC11Wins(t *testing.T) {
	t.Setenv("SPY_THEME", "")
	t.Setenv("NO_COLOR", "")
	// A successful OSC 11 reply must be used directly even when
	// COLORFGBG would disagree.
	t.Setenv("COLORFGBG", "0;15") // would say "light"
	got := detectBackgroundLuminance(context.Background(), envFromOS{}, true,
		fakeProbe("\x1b]11;rgb:0000/0000/0000\x07"))
	if math.IsNaN(got) || got >= 0.5 {
		t.Errorf("OSC 11 dark reply: got %v, want < 0.5", got)
	}
}

func TestDetectBackgroundLuminance_FallsBackToColorFGBG(t *testing.T) {
	t.Setenv("SPY_THEME", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "0;15")
	got := detectBackgroundLuminance(context.Background(), envFromOS{}, true,
		fakeProbe("")) // OSC probe yields nothing
	if math.IsNaN(got) || got < 0.5 {
		t.Errorf("COLORFGBG=0;15 fallback: got %v, want ≥ 0.5", got)
	}
}

func TestDetectBackgroundLuminance_FallsBackToNaN(t *testing.T) {
	t.Setenv("SPY_THEME", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "")
	got := detectBackgroundLuminance(context.Background(), envFromOS{}, true,
		fakeProbe(""))
	if !math.IsNaN(got) {
		t.Errorf("no signal: got %v, want NaN", got)
	}
}

func TestDetectBackgroundLuminance_OSC11MalformedFallsThrough(t *testing.T) {
	// Adversarial OSC 11 reply must be discarded and the COLORFGBG
	// fallback consulted instead.
	t.Setenv("SPY_THEME", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "0;15")
	got := detectBackgroundLuminance(context.Background(), envFromOS{}, true,
		fakeProbe("\x1b]11;rgb:\x1b[2J/0/0\x07"))
	if math.IsNaN(got) || got < 0.5 {
		t.Errorf("malformed OSC + light COLORFGBG: got %v, want ≥ 0.5", got)
	}
}

// --- Detect-level integration smoke tests ---

// TestDetect_NonTTYBypassesLuminanceProbe pins the contract from
// research R6: when stdout is not a TTY [detectBackgroundLuminance]
// short-circuits to NaN even when COLORFGBG would otherwise yield a
// usable fallback. The piped-session caller has no use for theming
// and surfacing a non-NaN value would surprise the renderer.
func TestDetect_NonTTYBypassesLuminanceProbe(t *testing.T) {
	resetEnv(t)
	t.Setenv("COLORFGBG", "0;15") // would say "light" if the probe ran
	caps := Detect(context.Background())
	if caps.IsTTY {
		t.Skip("test runner has a real TTY; non-TTY bypass path not exercised")
	}
	if !math.IsNaN(caps.BackgroundLuminance) {
		t.Errorf("non-TTY Detect must bypass to NaN; got %v", caps.BackgroundLuminance)
	}
}

func TestDetect_RespectsSPYThemeBypass(t *testing.T) {
	resetEnv(t)
	t.Setenv("SPY_THEME", "light")
	caps := Detect(context.Background())
	if !math.IsNaN(caps.BackgroundLuminance) {
		t.Errorf("SPY_THEME=light bypass: got %v, want NaN", caps.BackgroundLuminance)
	}
}

func TestDetect_RespectsNoColorBypass(t *testing.T) {
	resetEnv(t)
	t.Setenv("NO_COLOR", "1")
	caps := Detect(context.Background())
	if !math.IsNaN(caps.BackgroundLuminance) {
		t.Errorf("NO_COLOR=1 bypass: got %v, want NaN", caps.BackgroundLuminance)
	}
}

// Ensure the OSC probe budget does not block well past the documented
// 50 ms total deadline. Even in environments where the probe quietly
// fails (no TTY → fakeProbe returns ""), the wall-clock should be
// trivially small.
func TestDetectBackgroundLuminance_BudgetIsBounded(t *testing.T) {
	t.Setenv("SPY_THEME", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "")
	start := time.Now()
	_ = detectBackgroundLuminance(context.Background(), envFromOS{}, true, fakeProbe(""))
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("detect took %v, expected ≤ 100ms (budget is 50ms + slack)", d)
	}
}

// --- seenTerminator ---

func TestSeenTerminator_BEL(t *testing.T) {
	if !seenTerminator([]byte("\x1b]11;rgb:0/0/0\x07")) {
		t.Errorf("BEL terminator: should be seen")
	}
}

func TestSeenTerminator_ST(t *testing.T) {
	if !seenTerminator([]byte("\x1b]11;rgb:0/0/0\x1b\\")) {
		t.Errorf("ST terminator: should be seen")
	}
}

func TestSeenTerminator_PartialBuffer(t *testing.T) {
	if seenTerminator([]byte("\x1b]11;rgb:0/0/0")) {
		t.Errorf("no terminator: should not be seen")
	}
	if seenTerminator(nil) {
		t.Errorf("empty buffer: should not be seen")
	}
}

func TestSeenTerminator_LonelyBackslash(t *testing.T) {
	// A backslash without a preceding ESC is not ST.
	if seenTerminator([]byte("foo\\")) {
		t.Errorf("lone backslash: must not register as ST terminator")
	}
}

// --- readOSCReply ---

// scriptedReader hands out canned chunks one Read at a time; once the
// list is drained, subsequent Reads return io.EOF. This lets us drive
// readOSCReply through specific multi-packet shapes without spinning up
// a real TTY.
type scriptedReader struct {
	chunks [][]byte
	idx    int
	err    error
}

func (s *scriptedReader) Read(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if s.idx >= len(s.chunks) {
		// Signal end-of-input once all scripted chunks have been read
		// so [readOSCReply] stops looping even when the script never
		// supplies an explicit OSC terminator.
		return 0, io.EOF
	}
	c := s.chunks[s.idx]
	s.idx++
	n := copy(p, c)
	return n, nil
}

func TestReadOSCReply_SinglePacketTerminator(t *testing.T) {
	r := &scriptedReader{chunks: [][]byte{[]byte("\x1b]11;rgb:0000/0000/0000\x07")}}
	got := readOSCReply(context.Background(), r)
	if string(got) != "\x1b]11;rgb:0000/0000/0000\x07" {
		t.Errorf("single-packet: got %q", string(got))
	}
}

func TestReadOSCReply_MultiPacketTerminator(t *testing.T) {
	// Body + terminator delivered in two separate chunks. The drain
	// loop must keep reading until seenTerminator is satisfied.
	r := &scriptedReader{chunks: [][]byte{
		[]byte("\x1b]11;rgb:0000/0000/0000"),
		[]byte("\x07"),
	}}
	got := readOSCReply(context.Background(), r)
	if string(got) != "\x1b]11;rgb:0000/0000/0000\x07" {
		t.Errorf("multi-packet: got %q", string(got))
	}
}

func TestReadOSCReply_FirstReadEmptyReturnsNil(t *testing.T) {
	r := &scriptedReader{err: io.ErrClosedPipe}
	got := readOSCReply(context.Background(), r)
	if got != nil {
		t.Errorf("first-read error: want nil, got %q", string(got))
	}
}

func TestReadOSCReply_StopsOnContextCancel(t *testing.T) {
	// First chunk has no terminator; readOSCReply should re-enter the
	// loop, observe ctx.Done(), and return the partial buffer.
	r := &scriptedReader{chunks: [][]byte{[]byte("\x1b]11;rgb:0000/0000")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := readOSCReply(ctx, r)
	if string(got) != "\x1b]11;rgb:0000/0000" {
		t.Errorf("ctx cancel: got %q", string(got))
	}
}

func TestReadOSCReply_StopsAtBufferCap(t *testing.T) {
	// 60-byte body with no terminator. After the first read fills 60
	// bytes the loop continues to drain; with a 64-byte cap the second
	// read can add at most 4 more. We supply more than that to verify
	// the trim logic.
	first := make([]byte, 60)
	for i := range first {
		first[i] = '.'
	}
	second := make([]byte, 32)
	for i := range second {
		second[i] = '!'
	}
	r := &scriptedReader{chunks: [][]byte{first, second}}
	got := readOSCReply(context.Background(), r)
	if len(got) != oscReplyMaxBytes {
		t.Errorf("buffer cap: len=%d want %d", len(got), oscReplyMaxBytes)
	}
}

// --- raceReadOSCReply ---

// blockingReader returns the first chunk after a delay, simulating a
// terminal that's slow to respond. The blocking matters because we
// want to verify ctx cancellation actually wins the race, not just
// that an immediate read happens to return first.
type blockingReader struct {
	chunk []byte
	delay time.Duration
	done  bool
}

func (b *blockingReader) Read(p []byte) (int, error) {
	if b.done {
		return 0, io.EOF
	}
	time.Sleep(b.delay)
	b.done = true
	return copy(p, b.chunk), nil
}

func TestRaceReadOSCReply_ReadWins(t *testing.T) {
	r := &scriptedReader{chunks: [][]byte{[]byte("\x1b]11;rgb:0/0/0\x07")}}
	got := raceReadOSCReply(context.Background(), r)
	if string(got) != "\x1b]11;rgb:0/0/0\x07" {
		t.Errorf("read-wins: got %q", string(got))
	}
}

func TestRaceReadOSCReply_ContextCancelWins(t *testing.T) {
	// Block the reader long enough that the cancelled ctx returns first.
	r := &blockingReader{chunk: []byte("late"), delay: 100 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	got := raceReadOSCReply(ctx, r)
	if got != nil {
		t.Errorf("ctx wins: want nil buffer, got %q", string(got))
	}
}

// --- parseHexComponent length guard ---

func TestParseHexComponent_RejectsOversize(t *testing.T) {
	if got := parseHexComponent("12345"); !math.IsNaN(got) {
		t.Errorf("5-digit hex: got %v want NaN", got)
	}
}

func TestParseHexComponent_RejectsEmpty(t *testing.T) {
	if got := parseHexComponent(""); !math.IsNaN(got) {
		t.Errorf("empty: got %v want NaN", got)
	}
}
