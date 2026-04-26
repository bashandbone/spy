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
// is called, modelling the [loader.LineBuffer] state where the user
// hits `/` before the streamer's first chunk has populated the buffer.
//
// Exists in this package (rather than the loader test package) so the
// scan loop's pre-stream wait is exercised end-to-end without a
// goroutine in the loader package poking unexported fields.
type pendingProvider struct {
	mu     sync.Mutex
	ready  atomic.Bool
	lines  []source.Line
	totalN int64 // value returned once ready is true
}

func newPendingProvider(raw ...string) *pendingProvider {
	out := make([]source.Line, len(raw))
	for i, r := range raw {
		out[i] = source.Line{Number: int64(i + 1), Raw: r}
	}
	return &pendingProvider{lines: out, totalN: int64(len(out))}
}

func (p *pendingProvider) Total() int64 {
	if !p.ready.Load() {
		return -1
	}
	return p.totalN
}

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
	// Kick off the search BEFORE marking ready, modelling "/" typed
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
// bail immediately without polling — empty file should not cost the
// user 100 ms of search latency.
func TestScan_ZeroTotalIsConfirmedEmpty(t *testing.T) {
	t.Parallel()
	prov := newFakeProvider() // empty → Total() returns 0
	m, err := Compile("anything", false, CaseSensitive)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	start := time.Now()
	ch := Scan(context.Background(), prov, m, DirForward, 1)
	got := drain(t, ch, 200*time.Millisecond)
	elapsed := time.Since(start)
	if len(got) != 0 {
		t.Errorf("expected no matches on empty provider, got %d", len(got))
	}
	// Must not have hit the full pre-stream wait budget.
	if elapsed > 50*time.Millisecond {
		t.Errorf("scan against confirmed-empty provider took %v; expected near-zero", elapsed)
	}
}
