// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/search"
)

// TestH5_RunSearchReturnsCmdNotDirectResults pins the acceptance-review
// H5 contract: runSearch MUST return a tea.Cmd that drains the
// [search.Scan] channel off the Bubble Tea event-loop goroutine, NOT
// apply matches synchronously inside Update.
//
// Pre-fix runSearch drained the unbuffered Scan channel match-by-match
// on the event loop — against a 50 MB / 1 M-line resident buffer
// (MaxResidentBytes == 0) the loop blocked for seconds, freezing key
// and resize events. The async refactor moves the drain into the cmd
// so Update returns immediately with Pending=true and the result lands
// later via [searchResultMsg].
func TestH5_RunSearchReturnsCmdNotDirectResults(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo bar\nbaz\nfoo qux\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	mm, cmd := m.runSearch("foo", search.DirForward)
	updated := mm.(Model)
	if cmd == nil {
		t.Fatalf("runSearch must return a non-nil tea.Cmd for async drain (H5)")
	}
	// Synchronously after dispatch the live state must reflect "scan
	// in flight" — the user shouldn't see stale matches from a prior
	// search rendered against the new query.
	if !updated.search.Pending {
		t.Errorf("search.Pending must be true immediately after runSearch dispatch; got false")
	}
	if updated.search.Query != "foo" {
		t.Errorf("search.Query: got %q want foo", updated.search.Query)
	}
	if len(updated.search.Matches) != 0 {
		t.Errorf("search.Matches must be empty before searchResultMsg arrives; got %d", len(updated.search.Matches))
	}
	if updated.search.CurrentMatch != -1 {
		t.Errorf("search.CurrentMatch: got %d want -1", updated.search.CurrentMatch)
	}
}

// TestH5_PendingClearsOnResultMsg confirms the round-trip: the cmd's
// [searchResultMsg] arrival flips Pending=false and applies the
// matches.
func TestH5_PendingClearsOnResultMsg(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo bar\nbaz\nfoo qux\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	mm, cmd := m.runSearch("foo", search.DirForward)
	m = mm.(Model)
	if !m.search.Pending {
		t.Fatalf("precondition: Pending should be true after dispatch")
	}
	msg := cmd()
	res, ok := msg.(searchResultMsg)
	if !ok {
		t.Fatalf("cmd produced %T; expected searchResultMsg", msg)
	}
	if res.gen != m.searchGen {
		t.Errorf("searchResultMsg.gen mismatch: got %d want %d", res.gen, m.searchGen)
	}
	updated, _ := m.Update(res)
	mm2 := updated.(Model)
	if mm2.search.Pending {
		t.Errorf("Pending must clear once searchResultMsg is consumed; got true")
	}
	if len(mm2.search.Matches) != 2 {
		t.Errorf("expected 2 matches after result applied; got %d", len(mm2.search.Matches))
	}
	if mm2.search.CurrentMatch != 0 {
		t.Errorf("CurrentMatch should be 0 after result; got %d", mm2.search.CurrentMatch)
	}
}

// TestH5_NewSearchCancelsPriorContext is the rapid-typing guard. When
// the user submits a fresh search before the previous one completes,
// runSearch MUST cancel the previous context so its goroutine exits
// promptly rather than continuing to scan a buffer that's about to be
// reused.
//
// The test replaces the in-flight searchCancel with an instrumented
// wrapper so we can observe that the second runSearch invocation
// actually fires it (not just installs a new one).
func TestH5_NewSearchCancelsPriorContext(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo bar\nbaz\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	// First search.
	mm, _ := m.runSearch("foo", search.DirForward)
	m = mm.(Model)
	priorGen := m.searchGen
	realPriorCancel := m.searchCancel
	if realPriorCancel == nil {
		t.Fatalf("first runSearch should install searchCancel")
	}
	// Wrap the cancel so we can observe the second call invoking it.
	priorCancelCalled := false
	m.searchCancel = func() {
		priorCancelCalled = true
		realPriorCancel()
	}

	// Second search before draining the first cmd. Cancel must fire on
	// the prior context AND a fresh searchCancel must be installed.
	mm2, _ := m.runSearch("bar", search.DirForward)
	m2 := mm2.(Model)
	if !priorCancelCalled {
		t.Errorf("second runSearch must invoke the prior searchCancel — without this, " +
			"a slow first scan keeps draining the channel against the soon-to-be-stale buffer")
	}
	if m2.searchCancel == nil {
		t.Fatalf("second runSearch should install a fresh searchCancel")
	}
	if m2.searchGen <= priorGen {
		t.Errorf("searchGen must increment on each dispatch: prior=%d post=%d", priorGen, m2.searchGen)
	}
}

// TestH5_StaleSearchResultIgnored ensures that a [searchResultMsg]
// from a cancelled / superseded search NEVER overwrites the live
// state. Without this guard a slow first scan that completes after
// the user has started a second one would silently replace the
// second's matches with stale results.
func TestH5_StaleSearchResultIgnored(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo bar\nbaz\nfoo qux\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	// First search; capture the cmd's would-be result without applying.
	mm, cmd1 := m.runSearch("foo", search.DirForward)
	m = mm.(Model)
	res1Msg := cmd1()
	res1, ok := res1Msg.(searchResultMsg)
	if !ok {
		t.Fatalf("first cmd: expected searchResultMsg; got %T", res1Msg)
	}
	staleGen := res1.gen

	// Second search BEFORE the first result has been applied. This
	// bumps searchGen so res1's gen is now stale.
	mm2, cmd2 := m.runSearch("bar", search.DirForward)
	m = mm2.(Model)
	if m.searchGen == staleGen {
		t.Fatalf("searchGen must advance past the stale gen %d", staleGen)
	}

	// Apply the second cmd's result — this is the LIVE search.
	res2Msg := cmd2()
	updated, _ := m.Update(res2Msg)
	live := updated.(Model)
	liveQuery := live.search.Query
	liveMatchCount := len(live.search.Matches)
	if liveQuery != "bar" {
		t.Fatalf("post-cmd2 live query: got %q want bar", liveQuery)
	}

	// Now feed the STALE first result through Update; it must be a
	// no-op because its gen no longer matches m.searchGen.
	updated2, _ := live.Update(res1)
	post := updated2.(Model)
	if post.search.Query != liveQuery {
		t.Errorf("stale searchResultMsg overwrote live query: got %q want %q", post.search.Query, liveQuery)
	}
	if len(post.search.Matches) != liveMatchCount {
		t.Errorf("stale searchResultMsg overwrote live matches: got %d want %d",
			len(post.search.Matches), liveMatchCount)
	}
	if post.searchGen != live.searchGen {
		t.Errorf("stale message must not mutate searchGen: got %d want %d",
			post.searchGen, live.searchGen)
	}
}

// TestH5_CancelledResultClearsPendingButDropsMatches covers the
// branch where the goroutine exits via ctx.Done() rather than channel
// close. The Update handler must clear Pending defensively but not
// apply the (potentially partial) matches.
func TestH5_CancelledResultClearsPendingButDropsMatches(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "foo\nfoo\nfoo\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	mm, _ := m.runSearch("foo", search.DirForward)
	m = mm.(Model)
	gen := m.searchGen

	// Synthesize a cancelled result for the LIVE generation.
	cancelled := searchResultMsg{
		gen:        gen,
		query:      "foo",
		dir:        search.DirForward,
		matches:    []search.Match{{Line: 1}}, // partial — must be ignored
		scanWrapAt: -1,
		cancelled:  true,
	}
	updated, _ := m.Update(cancelled)
	post := updated.(Model)
	if post.search.Pending {
		t.Errorf("cancelled result must clear Pending defensively")
	}
	// The live state's Matches field reflects the in-flight nil
	// (Pending=true installs nil); cancelled drops the partial set.
	if len(post.search.Matches) != 0 {
		t.Errorf("cancelled result must NOT apply partial matches; got %d", len(post.search.Matches))
	}
}

// TestH5_RapidTypingNoGoroutineLeak exercises the rapid-typing path
// end-to-end: dispatch N searches in quick succession, drain each
// goroutine to completion, and verify (a) the live state reflects the
// LAST search and (b) no scanner goroutine is left running (proven
// indirectly by the race detector when run with `go test -race`).
//
// The contextual contract is "cancel-prior-on-new"; the WaitGroup
// here just confirms every spawned goroutine actually exits.
func TestH5_RapidTypingNoGoroutineLeak(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo\nbar\nfoo\nbaz\nfoo\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	queries := []string{"alpha", "foo", "bar", "baz", "foo"}
	cmds := make([]tea.Cmd, 0, len(queries))
	for _, q := range queries {
		mm, cmd := m.runSearch(q, search.DirForward)
		m = mm.(Model)
		cmds = append(cmds, cmd)
	}

	// Drain every cmd to completion — each goroutine must return a
	// searchResultMsg (even cancelled ones produce a synthesized
	// cancelled=true msg). Run in parallel so a hung goroutine surfaces
	// as a test deadline timeout rather than a silent stall.
	var wg sync.WaitGroup
	results := make([]searchResultMsg, len(cmds))
	for i, cmd := range cmds {
		i, cmd := i, cmd
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cmd == nil {
				return
			}
			msg := cmd()
			if r, ok := msg.(searchResultMsg); ok {
				results[i] = r
			}
		}()
	}
	wg.Wait()

	// Apply all results in arrival order — most are stale; only the
	// last one's gen will match m.searchGen and actually mutate state.
	for _, r := range results {
		if r.gen == 0 {
			continue
		}
		updated, _ := m.Update(r)
		m = updated.(Model)
	}

	// Live state must reflect the LAST query. Earlier searches that
	// completed after their successor was dispatched must not have
	// overwritten the live state via the gen guard.
	if m.search.Query != queries[len(queries)-1] {
		t.Errorf("live query after rapid-typing: got %q want %q",
			m.search.Query, queries[len(queries)-1])
	}
	if m.search.Pending {
		t.Errorf("Pending must be false after the final result lands")
	}
}

// TestH5_SearchResultMsgUnknownGenIsNoOp covers the bare-message
// case: a [searchResultMsg] arriving with a gen that doesn't match
// m.searchGen (e.g. fired by a buffer swap that incremented the
// counter) must be a complete no-op — no mutation of search.State,
// no change to statusAdvisory, no goroutine work.
func TestH5_SearchResultMsgUnknownGenIsNoOp(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "x\n")
	m, _ = applyResize(m, 80, 24)
	m.statusAdvisory = "preserved"
	priorState := m.search

	stale := searchResultMsg{
		gen:        m.searchGen + 9999,
		query:      "ghost",
		matches:    []search.Match{{Line: 1}},
		scanWrapAt: -1,
	}
	updated, _ := m.Update(stale)
	post := updated.(Model)
	if post.statusAdvisory != "preserved" {
		t.Errorf("stale searchResultMsg overwrote statusAdvisory; got %q", post.statusAdvisory)
	}
	if post.search.Query != priorState.Query {
		t.Errorf("stale searchResultMsg overwrote search.Query; got %q want %q",
			post.search.Query, priorState.Query)
	}
}

// TestH5_RenderContextReflectsPending proves the Pending flag flows
// into the renderer's context, so the renderer / footer can show a
// "scan in flight" indicator if it chooses. The flag has to be live
// in the same frame that the dispatch happens in — without this the
// user sees no visible feedback that the search is running.
func TestH5_RenderContextReflectsPending(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo\nbar\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	mm, _ := m.runSearch("foo", search.DirForward)
	m = mm.(Model)
	ctx := m.renderContext()
	if !ctx.Search.Pending {
		t.Errorf("renderContext().Search.Pending: got false want true")
	}
	if ctx.Search.Query != "foo" {
		t.Errorf("renderContext().Search.Query during Pending: got %q want foo", ctx.Search.Query)
	}
}

// TestH5_ReloadCancelsInFlightSearch confirms that a buffer swap
// (ActionReload) cancels any in-flight search so its goroutine
// doesn't continue scanning a buffer that's about to be GC'd. The
// gen bump is the observable side-effect: the in-flight result, if
// it lands, will be dropped on arrival.
func TestH5_ReloadCancelsInFlightSearch(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "foo\nbar\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	mm, _ := m.runSearch("foo", search.DirForward)
	m = mm.(Model)
	priorGen := m.searchGen
	if m.searchCancel == nil {
		t.Fatalf("precondition: searchCancel should be set after runSearch")
	}

	// Trigger reload. The handler must cancel the in-flight search
	// and bump searchGen so any racing result is dropped on arrival.
	updated, _ := m.Update(reloadMsg{})
	post := updated.(Model)
	if post.searchGen <= priorGen {
		t.Errorf("reload should bump searchGen (was %d, now %d) so racing results are dropped",
			priorGen, post.searchGen)
	}
	if post.searchCancel != nil {
		t.Errorf("reload should clear searchCancel after invoking it")
	}
}

// TestH5_CompileErrorPathStaysSynchronous regression-guards the
// invalid-pattern early return: when [search.Compile] rejects the
// query, runSearch must still return (model, nil) synchronously
// rather than dispatching a cmd. The advisory replaces the partial
// "search in flight" state.
func TestH5_CompileErrorPathStaysSynchronous(t *testing.T) {
	t.Parallel()
	cfg := newTestModel(t, "foo\n")
	cfg.cfg.RegexDefault = true
	cfg, _ = applyResize(cfg, 80, 24)
	cfg = drainStream(t, cfg)

	mm, cmd := cfg.runSearch("[invalid", search.DirForward)
	post := mm.(Model)
	if cmd != nil {
		t.Errorf("invalid pattern should not dispatch a cmd; got non-nil")
	}
	if !strings.Contains(post.statusAdvisory, "invalid pattern") {
		t.Errorf("expected 'invalid pattern' advisory; got %q", post.statusAdvisory)
	}
	if post.search.Pending {
		t.Errorf("compile-error path must not leave Pending=true")
	}
}

// TestH5_CompileErrorCancelsPriorInFlightSearch regression-guards the
// fix from PR#22 Copilot review: if the user submits a query that
// fails [search.Compile] while a prior async search is still Pending,
// runSearch MUST cancel that prior scan and bump searchGen so its
// result is dropped on arrival. Pre-fix the cancel/bump happened only
// after a successful compile, so the prior scan's matches would land
// AFTER the "invalid pattern" advisory and silently overwrite it.
func TestH5_CompileErrorCancelsPriorInFlightSearch(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "alpha\nfoo\nbar\nfoo\n")
	m.cfg.RegexDefault = true
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	// First search — capture its cmd without applying the result yet.
	mm, cmd1 := m.runSearch("foo", search.DirForward)
	m = mm.(Model)
	priorGen := m.searchGen
	realPriorCancel := m.searchCancel
	if realPriorCancel == nil {
		t.Fatalf("first runSearch should install searchCancel")
	}
	priorCancelCalled := false
	m.searchCancel = func() {
		priorCancelCalled = true
		realPriorCancel()
	}
	res1Msg := cmd1()
	res1, ok := res1Msg.(searchResultMsg)
	if !ok {
		t.Fatalf("cmd1: expected searchResultMsg; got %T", res1Msg)
	}
	staleGen := res1.gen

	// Second submission: invalid regex. Must cancel the prior scan
	// AND bump gen so the still-pending res1 is now stale.
	mm2, cmd2 := m.runSearch("[invalid", search.DirForward)
	post := mm2.(Model)
	if cmd2 != nil {
		t.Errorf("invalid pattern should not dispatch a cmd; got non-nil")
	}
	if !strings.Contains(post.statusAdvisory, "invalid pattern") {
		t.Errorf("expected 'invalid pattern' advisory; got %q", post.statusAdvisory)
	}
	if !priorCancelCalled {
		t.Errorf("compile-error path MUST cancel a prior in-flight search — " +
			"without this its result would clobber the error advisory")
	}
	if post.searchGen <= priorGen {
		t.Errorf("compile-error path must bump searchGen (was %d, now %d) so the prior result is stale",
			priorGen, post.searchGen)
	}
	if post.searchGen == staleGen {
		t.Fatalf("post-error searchGen still matches the prior cmd's gen %d — its result would not be dropped", staleGen)
	}

	// Now feed the prior scan's result through Update; it must be
	// dropped (gen mismatch) and must NOT clobber the advisory.
	updated, _ := post.Update(res1)
	final := updated.(Model)
	if !strings.Contains(final.statusAdvisory, "invalid pattern") {
		t.Errorf("stale prior-scan result clobbered the error advisory: %q", final.statusAdvisory)
	}
	if len(final.search.Matches) != 0 {
		t.Errorf("stale prior-scan result populated Matches: got %d", len(final.search.Matches))
	}
	if final.search.Pending {
		t.Errorf("Pending must stay false after compile-error + stale result")
	}
}

// TestH5_ReloadClearsInFlightSearchState regression-guards the fix
// from PR#22 Copilot review: a reload that races with a Pending
// search must explicitly clear Pending / Matches / CurrentMatch.
// Pre-fix, the gen bump invalidated the result on arrival but the
// model still showed Pending=true indefinitely (the cancelled result
// drops Pending, but a gen-mismatched result is a no-op so the flag
// would have stayed stuck forever).
func TestH5_ReloadClearsInFlightSearchState(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, "foo\nbar\nfoo\n")
	m, _ = applyResize(m, 80, 24)
	m = drainStream(t, m)

	mm, _ := m.runSearch("foo", search.DirForward)
	m = mm.(Model)
	if !m.search.Pending {
		t.Fatalf("precondition: search should be Pending after dispatch")
	}

	updated, _ := m.Update(reloadMsg{})
	post := updated.(Model)
	if post.search.Pending {
		t.Errorf("reload must clear search.Pending; otherwise a Pending+stale-gen race leaves Pending stuck on")
	}
	if post.search.Matches != nil {
		t.Errorf("reload must clear search.Matches (offsets no longer map to the new buffer); got len=%d",
			len(post.search.Matches))
	}
	if post.search.CurrentMatch != 0 {
		t.Errorf("reload must reset search.CurrentMatch to 0; got %d", post.search.CurrentMatch)
	}
}
