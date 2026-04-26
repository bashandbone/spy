// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package search

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knitli/spy/internal/source"
)

// pendingProvider returns -1 from Total() until [pendingProvider.Ready]
// is called, modeling the [loader.LineBuffer] state where the user
// hits `/` before the streamer's first chunk has populated the buffer.
//
// Exists in this package (rather than the loader test package) so the
// scan loop's pre-stream wait is exercised end-to-end without a
// goroutine in the loader package poking unexported fields.
type pendingProvider struct {
	mu         sync.Mutex
	ready      atomic.Bool
	lines      []source.Line
	totalN     int64 // value returned once ready is true
	totalCalls atomic.Int64
}

func newPendingProvider(raw ...string) *pendingProvider {
	out := make([]source.Line, len(raw))
	for i, r := range raw {
		out[i] = source.Line{Number: int64(i + 1), Raw: r}
	}
	return &pendingProvider{lines: out, totalN: int64(len(out))}
}

func (p *pendingProvider) Total() int64 {
	p.totalCalls.Add(1)
	if !p.ready.Load() {
		return -1
	}
	return p.totalN
}

// TotalCalls returns the number of times Total() has been invoked,
// for assertions on whether the scan entered the -1 polling path.
func (p *pendingProvider) TotalCalls() int64 { return p.totalCalls.Load() }

func (p *pendingProvider) Slice(start, end int64) []source.Line {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.ready.Load() {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if end > int64(len(p.lines)) {
		end = int64(len(p.lines))
	}
	if end <= start {
		return nil
	}
	out := make([]source.Line, end-start)
	copy(out, p.lines[start:end])
	return out
}

func (p *pendingProvider) Ready() {
	p.ready.Store(true)
}

// TestScan_WaitsForTotalWhenUnknown verifies the M5 fix: when the
// provider reports the -1 "unknown total" sentinel (loader hasn't
// populated yet), a search kicked off in that window must wait for the
// first chunk rather than bailing silently. After the provider flips
// to ready, the scan must produce the expected match (Copilot review
// acceptance M5).
func TestScan_WaitsForTotalWhenUnknown(t *testing.T) {
	t.Parallel()
	prov := newPendingProvider("alpha", "beta", "gamma")
	m, err := Compile("beta", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// Kick off the search BEFORE marking ready, modeling "/" typed
	// the moment after open. The scan must wait in waitForTotal
	// rather than returning empty.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Scan(ctx, prov, m, DirForward, 1)

	// Flip the provider to ready after a brief delay — well within the
	// scan's wait budget so the search completes successfully.
	go func() {
		time.Sleep(15 * time.Millisecond)
		prov.Ready()
	}()

	got := drain(t, ch, 500*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 match after provider became ready, got %d (%+v)", len(got), got)
	}
	if got[0].Line != 2 {
		t.Errorf("match line: got %d want 2", got[0].Line)
	}
}

// TestScan_PreStreamCancellationExitsCleanly ensures cancelling the
// search context during the pre-stream wait closes the result channel
// promptly — the scan must not deadlock on the wait poll if the
// provider never becomes ready and ctx is cancelled.
func TestScan_PreStreamCancellationExitsCleanly(t *testing.T) {
	t.Parallel()
	prov := newPendingProvider("alpha", "beta", "gamma")
	m, err := Compile("beta", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := Scan(ctx, prov, m, DirForward, 1)
	// Cancel before the provider becomes ready.
	cancel()

	// The result channel must close within a small bound of the
	// cancellation; collect with a generous timeout to avoid
	// flake on CI.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // closed cleanly — pass.
			}
		case <-deadline:
			t.Fatal("scan did not exit after ctx cancel during pre-stream wait")
		}
	}
}

// TestScan_ZeroTotalIsConfirmedEmpty verifies the boundary between -1
// (unknown) and 0 (confirmed empty): a provider that reports 0 must
// bail immediately without entering the -1 polling path — empty file
// should not cost the user 100 ms of search latency.
//
// Asserts via the pendingProvider's Total() call counter rather than
// wall-clock elapsed time: the polling path would call Total() many
// times across totalUnknownMaxWaits attempts, so a small bound on
// the call count is a more deterministic regression guard than a
// 50 ms wall-clock budget that flakes on loaded CI runners (PR#24
// review).
func TestScan_ZeroTotalIsConfirmedEmpty(t *testing.T) {
	t.Parallel()
	prov := newPendingProvider() // empty: zero lines, totalN=0
	prov.Ready()                 // flip immediately so Total() returns 0, not -1
	m, err := Compile("anything", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ch := Scan(context.Background(), prov, m, DirForward, 1)
	got := drain(t, ch, 200*time.Millisecond)
	if len(got) != 0 {
		t.Errorf("expected no matches on empty provider, got %d", len(got))
	}
	// The polling path calls Total() up to totalUnknownMaxWaits times
	// (currently 20). A bound of 4 covers any reasonable initial-read
	// + termination-check pattern while making the polling path's
	// fingerprint plainly visible if it's accidentally re-entered.
	if calls := prov.TotalCalls(); calls > 4 {
		t.Errorf("Total() called %d times on a confirmed-empty provider; "+
			"polling path entered (max-waits=%d). Scan must short-circuit on Total()==0",
			calls, totalUnknownMaxWaits)
	}
}
