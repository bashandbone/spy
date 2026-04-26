// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/knitli/spy/internal/loader"
	"github.com/knitli/spy/internal/source"
)

// TestC7_StreamErrSurfacedAsAdvisory pins the acceptance-review C7
// contract: a non-nil error arriving on [loader.Stream.Errs] MUST
// surface in the UI as a status-bar advisory rather than being
// silently dropped.
//
// Pre-fix the loader's Errs channel was buffered and written to
// (via the loader's own select/default best-effort send) but never
// drained from the production UI — when the buffer filled up the
// `default` arm of the select discarded warnings without trace, so
// users were never told a 100 KiB+ line was truncated or that
// stdin scroll-back was disabled.
func TestC7_StreamErrSurfacedAsAdvisory(t *testing.T) {
	m := newTestModel(t, "alpha\nbeta\n")
	wrapped := fmt.Errorf("%w: line 42", loader.ErrLineTruncated)

	updated, _ := m.Update(streamErrMsg{err: wrapped, stream: m.stream})
	got := updated.(Model).statusAdvisory
	if got == "" {
		t.Fatalf("statusAdvisory unset after streamErrMsg — warning silently dropped")
	}
	if !strings.Contains(got, "line 42") {
		t.Errorf("statusAdvisory missing line number; got %q", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("statusAdvisory missing 'truncated' marker; got %q", got)
	}
}

// TestC7_StdinNonSeekableSurfacedAsAdvisory exercises the second
// documented sentinel — windowed-mode entry against non-seekable
// stdin. The renderer's wording is preserved verbatim because it
// already explains what the user lost.
func TestC7_StdinNonSeekableSurfacedAsAdvisory(t *testing.T) {
	m := newTestModel(t, "x\n")
	updated, _ := m.Update(streamErrMsg{err: loader.ErrStdinNonSeekable, stream: m.stream})
	got := updated.(Model).statusAdvisory
	if !strings.Contains(got, "stdin") || !strings.Contains(got, "scroll-back") {
		t.Errorf("statusAdvisory does not surface ErrStdinNonSeekable; got %q", got)
	}
}

// TestC7_StaleStreamErrIgnored verifies the stream-pointer guard:
// an err arriving from a stream that ActionReload / :open has
// swapped out must NOT overwrite the new session's advisory.
//
// Mirrors the same stale-message guard pattern that
// chunkLoadedMsg / streamDoneMsg already use (Copilot review PR#8
// #2 on the original loader wiring).
func TestC7_StaleStreamErrIgnored(t *testing.T) {
	m := newTestModel(t, "x\n")
	m.statusAdvisory = "preserved"

	staleStream := &loader.Stream{} // distinct pointer; no Errs needed
	updated, _ := m.Update(streamErrMsg{err: loader.ErrLineTruncated, stream: staleStream})
	if got := updated.(Model).statusAdvisory; got != "preserved" {
		t.Errorf("stale streamErrMsg overrode the active advisory; got %q want %q", got, "preserved")
	}
}

// TestC7_NilErrSkippedReSubscribes covers a defensive edge case:
// the loader's `select / default` send is best-effort, and a nil
// send through that channel (theoretically impossible but cheap to
// guard) must not corrupt the advisory. The handler should re-
// subscribe so subsequent real warnings still arrive.
func TestC7_NilErrSkippedReSubscribes(t *testing.T) {
	m := newTestModel(t, "x\n")
	m.statusAdvisory = "preserved"

	updated, cmd := m.Update(streamErrMsg{err: nil, stream: m.stream})
	if got := updated.(Model).statusAdvisory; got != "preserved" {
		t.Errorf("nil streamErrMsg overrode the active advisory; got %q want %q", got, "preserved")
	}
	if cmd == nil {
		t.Errorf("nil streamErrMsg did not re-subscribe — subsequent warnings would never arrive")
	}
}

// TestC7_FormatStreamErr_Sentinels covers the per-sentinel string
// shaping. Loader wraps ErrLineTruncated as `"%w: line N"` —
// formatStreamErr surfaces the line number first ("more relevant to
// the user") and the cause second.
func TestC7_FormatStreamErr_Sentinels(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantSubstr []string
	}{
		{
			name:       "line_truncated_wrapped",
			err:        fmt.Errorf("%w: line 100", loader.ErrLineTruncated),
			wantSubstr: []string{"line 100", "truncated"},
		},
		{
			name:       "stdin_non_seekable_verbatim",
			err:        loader.ErrStdinNonSeekable,
			wantSubstr: []string{"stdin", "scroll-back"},
		},
		{
			name:       "unknown_error_wrapped",
			err:        errors.New("synthetic loader fault"),
			wantSubstr: []string{"loader:", "synthetic loader fault"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatStreamErr(tc.err)
			for _, sub := range tc.wantSubstr {
				if !strings.Contains(got, sub) {
					t.Errorf("formatStreamErr(%v) = %q; want substring %q", tc.err, got, sub)
				}
			}
		})
	}
}

// TestC7_Init_SubscribesToErrsChannel proves the Init pipeline
// includes waitForStreamErr — without it, a real warning emitted
// by the loader's truncation path would never reach a subscriber.
//
// The test drives a synthetic stream whose Errs channel has a
// pre-injected warning, calls waitForStreamErr directly, and
// asserts the resulting cmd yields a streamErrMsg carrying that
// warning.
//
// We don't drive Init() end-to-end here — that returns a tea.Batch
// whose order is implementation-detail and whose subordinate cmds
// can block on an empty Errs channel. The contract that matters is
// "subscribed AND tagged correctly", which waitForStreamErr fully
// expresses.
func TestC7_WaitForStreamErr_DeliversTaggedMsg(t *testing.T) {
	errs := make(chan error, 2)
	stream := &loader.Stream{Errs: errs}

	// Inject a warning so the cmd has something to read; no real
	// loader is running.
	errs <- fmt.Errorf("%w: line 7", loader.ErrLineTruncated)

	cmd := waitForStreamErr(stream)
	if cmd == nil {
		t.Fatalf("waitForStreamErr returned nil — production wiring would have no subscriber")
	}
	msg := cmd()
	got, ok := msg.(streamErrMsg)
	if !ok {
		t.Fatalf("cmd did not yield streamErrMsg; got %T (%v)", msg, msg)
	}
	if got.stream != stream {
		t.Errorf("streamErrMsg.stream mismatch — stale-stream guard would misfire")
	}
	if !errors.Is(got.err, loader.ErrLineTruncated) {
		t.Errorf("streamErrMsg.err did not carry the injected sentinel; got %v", got.err)
	}
}

// TestC7_WaitForStreamErr_NilOnClose covers the channel-closed
// branch. The loader closes Errs after Updates, so a closed Errs
// signals "streaming has fully ended". We return nil so the
// Bubble Tea command machinery treats it as a no-op rather than
// dispatching a misleading streamErrMsg with a zero-value error.
func TestC7_WaitForStreamErr_NilOnClose(t *testing.T) {
	errs := make(chan error)
	close(errs)
	stream := &loader.Stream{Errs: errs}

	cmd := waitForStreamErr(stream)
	if cmd == nil {
		t.Fatalf("waitForStreamErr returned nil cmd before invocation")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("cmd on closed Errs returned non-nil msg %T (%v); expected nil", msg, msg)
	}
}

// TestC7_EndToEnd_TruncationFromRealLoader is the production-path
// integration: drive the actual loader against a source whose first
// line exceeds MaxLineBytes, run the model through Init() and a
// short event loop, then confirm the truncation warning surfaced
// in m.statusAdvisory.
//
// This exercises the full chain: bufio.Scanner → readChunk's
// per-line cap → loader's errs send → waitForStreamErr → Update's
// onStreamErr → m.statusAdvisory. Each step in isolation has a
// unit test above; this one proves the wires connect.
func TestC7_EndToEnd_TruncationFromRealLoader(t *testing.T) {
	// 200 KiB single line — well past the default 100 KiB
	// MaxLineBytes cap.
	body := strings.Repeat("x", 200*1024) + "\n"

	src := &fakeSource{body: body, kind: source.KindText}
	stream, err := loader.Open(context.Background(), src, loader.Config{})
	if err != nil {
		t.Fatalf("loader.Open: %v", err)
	}
	if stream == nil {
		t.Fatalf("loader.Open returned nil stream")
	}

	m := newTestModel(t, "ignored") // hold the model shape
	m.source = src
	m.stream = stream

	// Drive Init's commands and feed any streamErrMsg back into
	// Update — same sequence Bubble Tea's runtime would execute.
	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("Init returned nil — Errs subscription would never fire")
	}

	// Race the Init pipeline against a short deadline. Once we see
	// a streamErrMsg we feed it into Update and check the
	// advisory; once we see a chunkLoadedMsg or streamDoneMsg we
	// move on (no warning). Either path is bounded.
	deadline := time.Now().Add(2 * time.Second)
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 && time.Now().Before(deadline) {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		var msg tea.Msg
		select {
		case msg = <-done:
		case <-time.After(200 * time.Millisecond):
			continue
		}
		if msg == nil {
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, sub := range batch {
				queue = append(queue, sub)
			}
			continue
		}
		updated, next := m.Update(msg)
		m = updated.(Model)
		if next != nil {
			queue = append(queue, next)
		}
		if strings.Contains(m.statusAdvisory, "truncated") {
			return // success
		}
	}
	t.Fatalf("statusAdvisory never received truncation warning; final=%q", m.statusAdvisory)
}
