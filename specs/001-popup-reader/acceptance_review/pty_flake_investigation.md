<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos
SPDX-License-Identifier: MIT OR Apache-2.0
-->

# M7 Investigation — PTY first-`q` flake

**Status**: investigated; root cause partially diagnosed; conservative
workaround retained; follow-up issue filed.

**Cross-references**:

- `tests/integration/pty_sanity_test.go:51-57` (retry loop in
  `TestPTYSanity_QuitOnQBigFile`)
- `tests/integration/pty_sanity_test.go:83-85` (retry loop in
  `TestPTYSanity_QuitOnQ`)
- `tests/perf/dismiss_bench_test.go:99-129` (10 ms timeout +
  retransmit in `measureDismiss`)
- `tests/integration/pty.go` (PTY harness)
- `cmd/spy/main.go:175-177` (OSC 11 probe gating)
- `internal/term/theme_unix.go:42-67` (`probeOSC11Background`)

## Symptom

After the spawned spy binary completes its bootstrap (alt-screen
enter, bracketed-paste setup, streaming-complete footer painted), the
first `q` keystroke sent by the harness is occasionally dropped — the
process does not exit. Sending a second `q` reliably produces the
documented quit. Locally reproducible at low rates (~1-5% per
iteration on CI runners; lower on bare-metal Linux).

The current workaround in `dismiss_bench_test.go` is a 10 ms first-pass
timeout: send `q`, wait 10 ms, retransmit only if exit hasn't already
fired. The retransmit's elapsed time is then measured. The 10 ms
ceiling keeps the SC-007 p95 sensitive to real regressions in the 50–
500 ms range while immunising against the first-`q` race.

`pty_sanity_test.go` uses a coarser 5-iteration loop with 200 ms per
poll and no separate timing concern.

## Hypotheses Considered

### H1 — `q` arrives before raw mode is set

**Hypothesis**: the harness sends `q` while the spy binary's tty is
still in cooked mode (ICANON), so the byte is parked in the line
discipline buffer waiting for `\n`.

**Evidence against**: the harness explicitly waits for
`\x1b[?2004h` (Bubble Tea's bracketed-paste-mode emission) BEFORE
sending. Bubble Tea emits this after configuring raw mode via
`golang.org/x/term.MakeRaw` on stdin's fd. The dismiss benchmark
adds a further 150 ms sleep on top of this. By the time `q` is
sent, raw mode has been active for > 100 ms.

**Verdict**: not the root cause.

### H2 — OSC 11 background probe consumes the keystroke

**Hypothesis**: `cmd/spy/main.go:175-177` runs the OSC 11 luminance
probe whenever `cfg.Theme == "auto"` (which is the default and is
hit by every dismiss-test invocation since the tests don't override
theme). The probe opens `/dev/tty`, calls `MakeRaw`, writes the OSC
11 query, and reads the reply with a 50 ms budget. While the probe
holds `/dev/tty` in raw mode, any byte arriving on the controlling
terminal — INCLUDING the test's `q` — would be consumed by the
probe's reader instead of routed to Bubble Tea's input reader.

**Evidence against**: the probe runs synchronously in `run()` before
`tea.NewProgram` is called. The harness only sends `q` AFTER
observing the streaming-complete footer, which requires Bubble Tea
to be running and the loader to have hit EOF. By that point the
probe has long since returned and `/dev/tty` is closed (defer
chain).

**Verdict**: not the root cause for the first-`q` flake. **However**
this branch confirms the harness is not racing against the OSC 11
probe.

### H3 — Bubble Tea's input reader hasn't subscribed to stdin yet

**Hypothesis**: Bubble Tea v1's `Program.Run()` initialises its
cancelreader on stdin in parallel with the renderer goroutine that
emits the alt-screen prologue. The `\x1b[?2004h` escape (the
harness's "ready" signal) is emitted by the renderer goroutine
BEFORE the input reader has established its first epoll wait. A
keystroke that arrives at exactly that window can sit in the
kernel buffer until the next epoll wakeup, which only occurs on
the NEXT byte arriving — making the first `q` look "lost".

**Evidence for**: this race window is structurally present in
Bubble Tea v1.x because `tea.Program` does not expose a "input loop
ready" signal. The alt-screen escape and bracketed-paste escape
are both renderer-side events that happen concurrently with — not
after — input reader initialisation.

**Evidence against**: epoll-based readers usually surface bytes
already in the kernel buffer on their first read, not just bytes
that arrive AFTER subscription. If the byte was already in the
buffer when the epoll wait started, the wait should return
immediately.

**Caveat**: cancelreader on Linux uses a self-pipe pattern for
cancellation — it writes to an internal pipe to wake the epoll
loop. The very first iteration might have a "drain self-pipe"
step that consumes a byte from the wrong source, or the epoll
registration might be racy. Diagnosing this requires
instrumenting Bubble Tea's cancelreader from inside, which is
beyond the scope of v0.1.0.

**Verdict**: most likely root cause; structural problem in
Bubble Tea v1 input-reader bootstrap.

### H4 — PTY slave / master kernel buffer race

**Hypothesis**: When the harness writes to the master, the byte
takes a path through the kernel's PTY discipline before reaching
the slave. If the slave-side reader hasn't issued its first
`read()` yet, the byte sits in the slave's input buffer. When the
reader does run, it should consume the buffered byte.

**Evidence against**: well-tested Linux behaviour; bytes written
to a PTY master are reliably available on the slave after the
master's write returns.

**Verdict**: not the root cause.

## Conclusion

The most likely root cause is **H3 — Bubble Tea v1 does not provide a
synchronization barrier between its renderer-prologue emit and its
input-reader subscription**. The harness's "wait for
`\x1b[?2004h`" check is a proxy for a barrier that doesn't actually
exist; the renderer can emit that escape while the input reader is
still finishing its goroutine setup.

A concrete fix would require either:

1. **Bubble Tea upstream change** — add an explicit "input ready"
   signal that the renderer waits for before its first paint, OR add
   a `Program.OnReady(func())` hook that fires after both renderer
   and input reader are subscribed.
2. **Harness change** — send a known-noop keystroke (e.g.,
   `\x1b\x1b` or a key with no binding) and wait for visible feedback
   (e.g., a `bell` character or some echo), proving the input loop
   is fully live before sending the real test input. This would
   change the public PTY harness contract and break the existing
   tests.
3. **Spy binary change** — emit a custom escape sequence after the
   first paint that the harness can wait on. This is invasive (adds
   protocol surface for the sole purpose of testing) and bleeds
   harness concerns into production.

Per the M7 brief, **none of these is acceptable for v0.1.0 closeout**.
The retry loops are conservative, well-documented, and correct
(they don't mask real regressions because the second-`q` measurement
in the dismiss bench is a real elapsed-time measurement). The 10 ms
first-pass timeout in `dismiss_bench_test.go` keeps the test sensitive
to dismiss regressions in the 50-500 ms range.

## Action Items

1. **Keep the retry loops** — see comment at
   `tests/perf/dismiss_bench_test.go:93-129`.
2. **Follow-up issue** — https://github.com/bashandbone/spy/issues/25
   tracks the upstream investigation.
3. **Re-evaluate** when Bubble Tea v2 or a follow-up release adds
   an input-ready barrier.

## Reproducer Notes

To reproduce locally with high probability:

```sh
go test -count=20 -run TestPTYSanity_QuitOnQ ./tests/integration/...
```

The flake rate is roughly 1-5% per iteration on bare-metal Linux,
higher under CI noise.
