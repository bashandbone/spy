// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package search

import (
	"context"

	"github.com/knitli/spy/internal/source"
)

// scanChunk is the number of lines we pull from the [source.LineProvider]
// per Slice() call. Bounded so very large sources don't realise the
// entire buffer up-front, while large enough that the per-call overhead
// stays a small fraction of the scan time.
const scanChunk int64 = 1024

// SentinelWrapped is emitted on the result channel as a synthetic Match
// (Line == -1) immediately before the scan continues from the opposite
// end of the buffer. Callers (internal/ui) flip [State.Wrapped] on
// receipt and can surface a "search wrapped" status message.
const SentinelWrapped int64 = -1

// Scan walks the lines exposed by `provider` in `dir`, starting at the
// 1-based line number `from` (inclusive). It emits one [Match] per
// occurrence on the returned channel; the channel is closed when the
// scan reaches its starting point (post-wrap) or when ctx is cancelled.
//
// On wrap-around — i.e. the scan reaches end (forward) or beginning
// (backward) without finding the start line again — a synthetic
// [Match]{Line: SentinelWrapped} is emitted before any further matches
// (or before close, when there are none).
//
// Cancellation: a non-nil ctx that's cancelled mid-scan stops the
// goroutine on the next iteration. The producer never blocks on a full
// receiver channel — the channel is unbuffered so the consumer drives
// the cadence; if no consumer is ready, the producer waits until ctx is
// cancelled.
func Scan(ctx context.Context, provider source.LineProvider, m Matcher, dir Direction, from int64) <-chan Match {
	out := make(chan Match)
	go scanLoop(ctx, provider, m, dir, from, out)
	return out
}

// scanLoop is the goroutine body. Extracted for testability and so the
// returned channel stays close-on-exit guaranteed via defer.
func scanLoop(ctx context.Context, provider source.LineProvider, m Matcher, dir Direction, from int64, out chan<- Match) {
	defer close(out)
	if provider == nil || m == nil {
		return
	}
	total := provider.Total()
	if total <= 0 {
		return
	}
	startLine := clamp(from, 1, total)
	switch dir {
	case DirForward:
		// First pass: from startLine to total.
		if !scanRange(ctx, provider, m, startLine, total+1, out) {
			return
		}
		// Wrap.
		if startLine > 1 {
			if !sendSentinel(ctx, out) {
				return
			}
			scanRange(ctx, provider, m, 1, startLine, out)
		}
	case DirBackward:
		// First pass: from startLine down to 1.
		if !scanRangeReverse(ctx, provider, m, 1, startLine+1, out) {
			return
		}
		// Wrap.
		if startLine < total {
			if !sendSentinel(ctx, out) {
				return
			}
			scanRangeReverse(ctx, provider, m, startLine+1, total+1, out)
		}
	}
}

// scanRange walks lines in [from, to) ascending, calling m.Find on each
// raw line, and forwarding the matches with the line number filled in.
// Returns false if ctx was cancelled mid-scan so the caller can abort
// the wrap-around step.
func scanRange(ctx context.Context, provider source.LineProvider, m Matcher, from, to int64, out chan<- Match) bool {
	for cur := from; cur < to; {
		end := cur + scanChunk
		if end > to {
			end = to
		}
		// LineProvider.Slice uses 0-based half-open indices internally
		// but we have 1-based line numbers here; convert.
		lines := provider.Slice(cur-1, end-1)
		for _, l := range lines {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return false
				default:
				}
			}
			matches := m.Find(l.Raw)
			for _, mm := range matches {
				mm.Line = l.Number
				if !sendMatch(ctx, out, mm) {
					return false
				}
			}
		}
		if len(lines) == 0 {
			// Provider returned nothing — likely windowed eviction; bail
			// rather than spinning.
			return true
		}
		cur += int64(len(lines))
	}
	return true
}

// scanRangeReverse walks lines in [from, to) descending, calling m.Find
// on each raw line. Same return semantics as [scanRange].
func scanRangeReverse(ctx context.Context, provider source.LineProvider, m Matcher, from, to int64, out chan<- Match) bool {
	for cur := to; cur > from; {
		start := cur - scanChunk
		if start < from {
			start = from
		}
		lines := provider.Slice(start-1, cur-1)
		// Iterate in descending order to honour DirBackward.
		for i := len(lines) - 1; i >= 0; i-- {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return false
				default:
				}
			}
			l := lines[i]
			matches := m.Find(l.Raw)
			// Within a line, matches stay in source-order (per the
			// data-model.md SearchState spec); reverse navigation is
			// driven by line ordering, not by reversing within-line.
			for _, mm := range matches {
				mm.Line = l.Number
				if !sendMatch(ctx, out, mm) {
					return false
				}
			}
		}
		if len(lines) == 0 {
			return true
		}
		cur -= int64(len(lines))
	}
	return true
}

// sendMatch routes a Match to `out` while honouring ctx cancellation so
// the goroutine can exit even if no consumer is ready.
func sendMatch(ctx context.Context, out chan<- Match, mm Match) bool {
	if ctx == nil {
		out <- mm
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case out <- mm:
		return true
	}
}

// sendSentinel emits the wrap-around sentinel match. Same cancellation
// semantics as [sendMatch].
func sendSentinel(ctx context.Context, out chan<- Match) bool {
	return sendMatch(ctx, out, Match{Line: SentinelWrapped})
}

// clamp returns v constrained to [lo, hi].
func clamp(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
