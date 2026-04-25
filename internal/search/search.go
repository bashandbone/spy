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
// goroutine on the next iteration. Sends on the unbuffered result
// channel synchronize with the consumer, so the consumer drives the
// cadence; when ctx is non-nil, the send/select path also honors
// ctx.Done() and aborts if cancellation is observed while waiting.
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
//
// Windowed-mode correctness: [loader.LineBuffer.Slice] returns only
// the resident overlap when the requested range partially overlaps the
// window. If the returned slice's first line.Number is greater than
// `cur`, we re-request the missing prefix with a range that lies
// entirely outside the resident window — that path inside Slice
// triggers the [source.Source.Reopen] re-seek and yields the evicted
// lines. Both progress and gap-detection use line numbers (not slice
// length) so a partial response can never silently advance past
// un-scanned content (Copilot review PR#9 round-2 #1, #2).
func scanRange(ctx context.Context, provider source.LineProvider, m Matcher, from, to int64, out chan<- Match) bool {
	for cur := from; cur < to; {
		end := cur + scanChunk
		if end > to {
			end = to
		}
		// LineProvider.Slice uses 0-based half-open indices internally
		// but we have 1-based line numbers here; convert.
		lines := provider.Slice(cur-1, end-1)
		if len(lines) == 0 {
			// Provider returned nothing for the requested range. This
			// can mean (a) the range is empty, (b) the buffer doesn't
			// have these lines and can't seek (e.g. stdin), or (c) the
			// provider has been wound down. We can't make progress
			// from `cur` in any case.
			return true
		}
		// Fill any prefix gap caused by a partial-overlap response.
		// Only keep gap-fill lines whose Number actually falls in the
		// gap [cur, first); a stubborn provider that can't seek
		// (stdin-like) may return its resident range again, which we
		// must not splice in or we'd double-scan.
		if first := lines[0].Number; first > cur {
			gap := provider.Slice(cur-1, first-1)
			filtered := gap[:0]
			for _, g := range gap {
				if g.Number >= cur && g.Number < first {
					filtered = append(filtered, g)
				}
			}
			if len(filtered) > 0 {
				merged := make([]source.Line, 0, len(filtered)+len(lines))
				merged = append(merged, filtered...)
				merged = append(merged, lines...)
				lines = merged
			}
		}
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
		// Advance based on the highest line number actually scanned so
		// a partial-overlap response can't stall (`last + 1 == cur`
		// after a gap-fill that returned nothing) or skip ahead.
		next := lines[len(lines)-1].Number + 1
		if next <= cur {
			// Defensive: provider returned overlapping/duplicate
			// content. Force progress so we don't loop.
			next = cur + 1
		}
		cur = next
	}
	return true
}

// scanRangeReverse walks lines in [from, to) descending, calling m.Find
// on each raw line. Same return semantics as [scanRange] and the same
// windowed-mode gap handling — except the gap, if any, sits at the
// high end of the response (Slice returned a resident prefix but
// dropped the requested suffix), so we re-request that range before
// iterating.
func scanRangeReverse(ctx context.Context, provider source.LineProvider, m Matcher, from, to int64, out chan<- Match) bool {
	for cur := to; cur > from; {
		start := cur - scanChunk
		if start < from {
			start = from
		}
		lines := provider.Slice(start-1, cur-1)
		if len(lines) == 0 {
			return true
		}
		// Fill any suffix gap caused by a partial-overlap response.
		// Only keep gap-fill lines whose Number actually lands in
		// (last, cur); see the forward-scan comment for why filtering
		// matters when the provider can't seek.
		if last := lines[len(lines)-1].Number; last < cur-1 {
			gapStart := last + 1
			gap := provider.Slice(gapStart-1, cur-1)
			filtered := gap[:0]
			for _, g := range gap {
				if g.Number >= gapStart && g.Number < cur {
					filtered = append(filtered, g)
				}
			}
			if len(filtered) > 0 {
				lines = append(lines, filtered...) // ascending order preserved
			}
		}
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
		// Advance based on the lowest line number actually scanned so a
		// partial-overlap response can't stall the reverse walk.
		next := lines[0].Number
		if next >= cur {
			next = cur - 1
		}
		cur = next
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
