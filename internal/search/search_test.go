// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/knitli/spy/internal/source"
)

// fakeProvider implements [source.LineProvider] over an in-memory slice
// for the search tests.
type fakeProvider struct {
	lines []source.Line
}

func newFakeProvider(raw ...string) *fakeProvider {
	out := make([]source.Line, len(raw))
	for i, r := range raw {
		out[i] = source.Line{Number: int64(i + 1), Raw: r}
	}
	return &fakeProvider{lines: out}
}

func (f *fakeProvider) Slice(start, end int64) []source.Line {
	if start < 0 {
		start = 0
	}
	if end > int64(len(f.lines)) {
		end = int64(len(f.lines))
	}
	if end <= start {
		return nil
	}
	out := make([]source.Line, end-start)
	copy(out, f.lines[start:end])
	return out
}

func (f *fakeProvider) Total() int64 {
	return int64(len(f.lines))
}

func TestScan_ForwardEmitsMatches(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider("alpha", "beta beta", "gamma", "beta")
	m, err := Compile("beta", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ch := Scan(context.Background(), prov, m, DirForward, 1)
	got := drain(t, ch, 100*time.Millisecond)
	wantLines := []int64{2, 2, 4}
	if len(got) != len(wantLines) {
		t.Fatalf("got %d matches, want %d (%+v)", len(got), len(wantLines), got)
	}
	for i, mm := range got {
		if mm.Line != wantLines[i] {
			t.Errorf("match %d: got line %d want %d", i, mm.Line, wantLines[i])
		}
	}
}

func TestScan_BackwardEmitsMatchesInReverseLineOrder(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider("alpha", "beta", "gamma beta", "delta beta")
	m, err := Compile("beta", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ch := Scan(context.Background(), prov, m, DirBackward, 4)
	got := drain(t, ch, 100*time.Millisecond)
	// Walking from line 4 down: 4 (delta beta), 3 (gamma beta), 2 (beta).
	wantLines := []int64{4, 3, 2}
	if len(got) != len(wantLines) {
		t.Fatalf("got %d matches, want %d (%+v)", len(got), len(wantLines), got)
	}
	for i, mm := range got {
		if mm.Line != wantLines[i] {
			t.Errorf("match %d: got line %d want %d", i, mm.Line, wantLines[i])
		}
	}
}

func TestScan_ForwardWrapsWithSentinel(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider("beta", "alpha", "gamma", "beta")
	m, err := Compile("beta", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// Start at line 2: scan forward, the only post-2 match is line 4,
	// then we wrap and find line 1.
	ch := Scan(context.Background(), prov, m, DirForward, 2)
	got := drain(t, ch, 100*time.Millisecond)
	if len(got) != 3 {
		t.Fatalf("got %d items, want 3 (line 4, sentinel, line 1) (%+v)", len(got), got)
	}
	if got[0].Line != 4 {
		t.Errorf("first hit line: got %d want 4", got[0].Line)
	}
	if got[1].Line != SentinelWrapped {
		t.Errorf("expected wrap sentinel at index 1; got %+v", got[1])
	}
	if got[2].Line != 1 {
		t.Errorf("post-wrap hit line: got %d want 1", got[2].Line)
	}
}

func TestScan_BackwardWrapsWithSentinel(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider("beta", "alpha", "gamma", "beta")
	m, err := Compile("beta", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// Start at line 3: scan backward — the only pre-3 match is line 1,
	// then wrap and find line 4.
	ch := Scan(context.Background(), prov, m, DirBackward, 3)
	got := drain(t, ch, 100*time.Millisecond)
	if len(got) != 3 {
		t.Fatalf("got %d items, want 3 (line 1, sentinel, line 4) (%+v)", len(got), got)
	}
	if got[0].Line != 1 {
		t.Errorf("first hit line: got %d want 1", got[0].Line)
	}
	if got[1].Line != SentinelWrapped {
		t.Errorf("expected wrap sentinel at index 1; got %+v", got[1])
	}
	if got[2].Line != 4 {
		t.Errorf("post-wrap hit line: got %d want 4", got[2].Line)
	}
}

func TestScan_ContextCancelStops(t *testing.T) {
	t.Parallel()
	// Build a provider with many matches so cancellation has work to do.
	raw := make([]string, 1000)
	for i := range raw {
		raw[i] = "match here"
	}
	prov := newFakeProvider(raw...)
	m, _ := Compile("match", false, CaseSensitive)

	ctx, cancel := context.WithCancel(context.Background())
	ch := Scan(ctx, prov, m, DirForward, 1)
	// Drain a few matches then cancel.
	for i := 0; i < 5; i++ {
		select {
		case <-ch:
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timed out waiting for match %d", i)
		}
	}
	cancel()
	// Channel must close in finite time after cancel.
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed — pass
			}
		case <-deadline:
			t.Fatal("scan goroutine did not exit after ctx cancel")
		}
	}
}

func TestScan_NoMatchesClosesChannel(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider("alpha", "beta", "gamma")
	m, _ := Compile("zeta", false, CaseSensitive)
	ch := Scan(context.Background(), prov, m, DirForward, 1)
	got := drain(t, ch, 100*time.Millisecond)
	if len(got) != 0 {
		t.Errorf("expected no matches; got %+v", got)
	}
}

func TestScan_EmptyProvider(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider()
	m, _ := Compile("anything", false, CaseSensitive)
	ch := Scan(context.Background(), prov, m, DirForward, 1)
	got := drain(t, ch, 100*time.Millisecond)
	if len(got) != 0 {
		t.Errorf("expected no matches on empty provider; got %+v", got)
	}
}

func TestScan_StartFromEndForwardWraps(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider("beta", "beta", "alpha")
	m, _ := Compile("beta", false, CaseSensitive)
	// from = 3 (last line, no match): immediate wrap, then both betas.
	ch := Scan(context.Background(), prov, m, DirForward, 3)
	got := drain(t, ch, 100*time.Millisecond)
	if len(got) != 3 {
		t.Fatalf("expected sentinel + 2 matches; got %d (%+v)", len(got), got)
	}
	if got[0].Line != SentinelWrapped {
		t.Errorf("expected wrap sentinel first; got %+v", got[0])
	}
	if got[1].Line != 1 || got[2].Line != 2 {
		t.Errorf("post-wrap matches got %d,%d want 1,2", got[1].Line, got[2].Line)
	}
}

func TestScan_PartialMatchesAcrossChunks(t *testing.T) {
	t.Parallel()
	// Force more than one scanChunk worth of lines so the chunked path
	// is exercised.
	count := int(scanChunk*2 + 17)
	raw := make([]string, count)
	for i := range raw {
		if i%500 == 0 {
			raw[i] = fmt.Sprintf("interesting %d", i)
		} else {
			raw[i] = "boring"
		}
	}
	prov := newFakeProvider(raw...)
	m, _ := Compile("interesting", false, CaseSensitive)
	ch := Scan(context.Background(), prov, m, DirForward, 1)
	got := drain(t, ch, 1*time.Second)
	want := count / 500
	if count%500 == 0 {
		// The exact number of i%500==0 hits in [0,count).
	} else {
		want = count/500 + 1
	}
	if len(got) != want {
		t.Errorf("got %d matches across %d lines; want %d", len(got), count, want)
	}
}

func TestScan_NilProviderIsNoOp(t *testing.T) {
	t.Parallel()
	m, _ := Compile("x", false, CaseSensitive)
	ch := Scan(context.Background(), nil, m, DirForward, 1)
	got := drain(t, ch, 50*time.Millisecond)
	if len(got) != 0 {
		t.Errorf("nil provider should produce no matches; got %+v", got)
	}
}

func TestScan_NilMatcherIsNoOp(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider("foo")
	ch := Scan(context.Background(), prov, nil, DirForward, 1)
	got := drain(t, ch, 50*time.Millisecond)
	if len(got) != 0 {
		t.Errorf("nil matcher should produce no matches; got %+v", got)
	}
}

// windowedProvider simulates [loader.LineBuffer]'s partial-overlap
// behaviour: requests for ranges that straddle `residentStart` return
// only the resident suffix on the *first* call, then the full prefix
// on a *second* call (the windowed re-seek path). Tracks the call
// pattern so tests can assert which paths fired.
type windowedProvider struct {
	all          []source.Line
	residentFrom int64 // 1-based line number; lines before this are evicted
	totalLines   int64
	calls        int
}

func newWindowedProvider(residentFrom int64, raw ...string) *windowedProvider {
	out := make([]source.Line, len(raw))
	for i, r := range raw {
		out[i] = source.Line{Number: int64(i + 1), Raw: r}
	}
	return &windowedProvider{
		all:          out,
		residentFrom: residentFrom,
		totalLines:   int64(len(raw)),
	}
}

// Slice mimics LineBuffer.Slice's three branches:
//   - entirely resident → returns the requested range
//   - partially overlapping with resident → returns just the resident
//     overlap (the call that exposed the original Scan bug)
//   - entirely outside resident → returns the requested range via the
//     "re-seek" path (we read directly from `all` so the test stays
//     deterministic)
func (w *windowedProvider) Slice(start, end int64) []source.Line {
	w.calls++
	wantStartNum := start + 1
	wantEndNum := end + 1
	if wantStartNum < 1 {
		wantStartNum = 1
	}
	if wantEndNum > w.totalLines+1 {
		wantEndNum = w.totalLines + 1
	}
	if wantEndNum <= wantStartNum {
		return nil
	}
	residentEnd := w.totalLines + 1
	// Entirely outside resident → re-seek path.
	if wantEndNum <= w.residentFrom {
		return w.copyRange(wantStartNum, wantEndNum)
	}
	// Partial overlap → return only the resident slice (the buggy
	// pre-fix behaviour we're now resilient to).
	if wantStartNum < w.residentFrom && wantEndNum > w.residentFrom {
		return w.copyRange(w.residentFrom, wantEndNum)
	}
	// Entirely resident.
	if wantStartNum >= w.residentFrom && wantEndNum <= residentEnd {
		return w.copyRange(wantStartNum, wantEndNum)
	}
	return nil
}

func (w *windowedProvider) copyRange(fromNum, toNum int64) []source.Line {
	if fromNum < 1 {
		fromNum = 1
	}
	if toNum > int64(len(w.all))+1 {
		toNum = int64(len(w.all)) + 1
	}
	if toNum <= fromNum {
		return nil
	}
	out := make([]source.Line, toNum-fromNum)
	copy(out, w.all[fromNum-1:toNum-1])
	return out
}

func (w *windowedProvider) Total() int64 {
	return w.totalLines
}

func TestScan_ForwardFillsWindowedPrefixGap(t *testing.T) {
	t.Parallel()
	// 10 lines, but only lines 6..10 are resident; "foo" lives only on
	// line 2. With the buggy advance-by-len behaviour, scan would see
	// lines [6..10] for the chunk that straddled the window, advance
	// past them, and miss line 2 entirely. The fix re-requests
	// [1, 6) via the entirely-outside path so line 2 is visited.
	prov := newWindowedProvider(6,
		"alpha", "foo here", "gamma", "delta", "epsilon",
		"zeta", "eta", "theta", "iota", "kappa",
	)
	m, err := Compile("foo", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ch := Scan(context.Background(), prov, m, DirForward, 1)
	got := drain(t, ch, 200*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 match (line 2); got %d (%+v)", len(got), got)
	}
	if got[0].Line != 2 {
		t.Errorf("got match on line %d, want 2", got[0].Line)
	}
}

func TestScan_BackwardFillsWindowedSuffixGap(t *testing.T) {
	t.Parallel()
	// Same fixture; backward scan from line 10 walks toward line 1.
	// The first chunk asks for [1..10] (assuming scanChunk > 10);
	// LineBuffer.Slice would return only [6..10], dropping the
	// evicted prefix. The fix re-requests [1, 6) via the
	// entirely-outside path so the line-2 match is reached.
	prov := newWindowedProvider(6,
		"alpha", "foo here", "gamma", "delta", "epsilon",
		"zeta", "eta", "theta", "iota", "kappa",
	)
	m, err := Compile("foo", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ch := Scan(context.Background(), prov, m, DirBackward, 10)
	got := drain(t, ch, 200*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 match (line 2); got %d (%+v)", len(got), got)
	}
	if got[0].Line != 2 {
		t.Errorf("got match on line %d, want 2", got[0].Line)
	}
}

// stubbornPartialProvider always returns just the resident suffix,
// even when asked for the prefix on a follow-up call. This simulates
// stdin (non-seekable) where the evicted lines are unrecoverable; the
// scan must terminate without infinite-looping rather than silently
// skip — bail on second-attempt failure is the only correct behaviour.
type stubbornPartialProvider struct {
	all          []source.Line
	residentFrom int64
}

func (s *stubbornPartialProvider) Slice(start, _ int64) []source.Line {
	wantStartNum := start + 1
	if wantStartNum < s.residentFrom {
		// Always trim to the resident range; never deliver evicted
		// prefix even on a re-seek attempt.
		wantStartNum = s.residentFrom
	}
	if wantStartNum > int64(len(s.all)) {
		return nil
	}
	out := make([]source.Line, int64(len(s.all))+1-wantStartNum)
	copy(out, s.all[wantStartNum-1:])
	return out
}

func (s *stubbornPartialProvider) Total() int64 { return int64(len(s.all)) }

func TestScan_TerminatesWhenGapIsUnrecoverable(t *testing.T) {
	t.Parallel()
	// Lines 6..10 resident; lines 1..5 evicted and the provider
	// refuses to seek (stdin-like). Scan from line 1 forward: the
	// fix's gap re-request returns the same resident suffix, which
	// would loop forever without the "advance past the highest line
	// number actually scanned" guard. The test asserts the scan
	// closes the channel within a finite time.
	prov := &stubbornPartialProvider{
		all: []source.Line{
			{Number: 1, Raw: "alpha"},
			{Number: 2, Raw: "beta"},
			{Number: 3, Raw: "gamma"},
			{Number: 4, Raw: "delta"},
			{Number: 5, Raw: "epsilon"},
			{Number: 6, Raw: "foo here"},
			{Number: 7, Raw: "eta"},
			{Number: 8, Raw: "theta"},
			{Number: 9, Raw: "iota"},
			{Number: 10, Raw: "kappa"},
		},
		residentFrom: 6,
	}
	m, _ := Compile("foo", false, CaseSensitive)
	ch := Scan(context.Background(), prov, m, DirForward, 1)
	got := drain(t, ch, 500*time.Millisecond)
	// Even if the evicted prefix can't be matched, the resident
	// "foo here" on line 6 must be reachable.
	if len(got) != 1 {
		t.Fatalf("expected 1 match; got %d (%+v)", len(got), got)
	}
	if got[0].Line != 6 {
		t.Errorf("got match on line %d, want 6", got[0].Line)
	}
}

func drain(t *testing.T, ch <-chan Match, timeout time.Duration) []Match {
	t.Helper()
	var out []Match
	deadline := time.After(timeout)
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, m)
		case <-deadline:
			t.Fatalf("scan did not close within %v; collected so far: %+v", timeout, out)
		}
	}
}
