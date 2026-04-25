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
