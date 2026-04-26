<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos
SPDX-License-Identifier: MIT OR Apache-2.0
-->

# M7 Investigation — PTY first-`q` flake

**Status**: root cause identified and fixed in production code; test
harness updated to remove retry loops; retransmit pattern retained in
dismiss benchmark for measurement fidelity.

**Cross-references**:

- `internal/term/theme_unix.go` — **production fix**: `probeOSC11Background`
  now uses `pollReadOSC` (O_NONBLOCK + 5 ms spin) instead of
  `raceReadOSCReply` (goroutine), eliminating the goroutine that leaked
  and consumed the first keystroke
- `tests/integration/pty_sanity_test.go` — fixed: `TestPTYSanity_QuitOnQ`
  and `TestPTYSanity_QuitOnQBigFile` now wait for rendered content
  and send a single `q`; retry loops removed
- `tests/integration/pty.go` — `BracketedPasteEnable` constant added
  with documentation explaining why it is NOT a sufficient input-ready
  signal
- `tests/perf/dismiss_bench_test.go:97-103` (retransmit comment
  updated to reflect the race is fixed; pattern retained for SC-007
  measurement fidelity)
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
probe has long since *returned* — but crucially, a **goroutine
spawned inside the probe was still alive**.

**Revised verdict**: **CONFIRMED root cause — goroutine leak**.

`probeOSC11Background` calls `raceReadOSCReply(ctx, f)`, which
spawns a goroutine G1 to call `readOSCReply(ctx, f)`. G1 calls
`f.Read(buf[:])` where `f.Fd()` has already put the file into
blocking mode (Go's netpoller is no longer managing it). When the
50 ms context deadline fires, `raceReadOSCReply` returns nil — but
G1 remains alive, blocked in `read(fd, ...)` at the OS level.

On Linux, `close(fd)` does **not** interrupt a blocked `read(fd)` on
another OS thread. After `defer f.Close()` runs, fd is freed. The
`EpollCreate1(0)` call inside Bubble Tea's `initCancelReader` reuses
that fd number — but G1 is blocking on the **original dev/tty file
description** (not the fd number), so it keeps waiting on the PTY
slave device.

When the test harness writes `q` to the PTY master, the kernel wakes
up any waiter for the PTY slave's read buffer. Both G1's blocking
`read()` and Bubble Tea's cancelreader (via `EpollWait`) are waiting.
G1's OS thread is already in the `read()` syscall and wins the race
on every iteration where the spy process hasn't been running long
enough for the goroutine scheduler to have settled (which at T ≈ 100 ms
is consistently the case in CI).

**Fix**: `probeOSC11Background` in `internal/term/theme_unix.go` now
calls `pollReadOSC(ctx, fd)` instead of `raceReadOSCReply(ctx, f)`.
`pollReadOSC` sets the fd to `O_NONBLOCK` via
`syscall.SetNonblock`, reads in a spin loop checking `ctx.Err()`
every 5 ms, and restores blocking mode before returning. No goroutine
is spawned, so no goroutine can outlive the context deadline and
compete with Bubble Tea's input reader.

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

**Verdict**: a contributing factor but **not the primary root cause**.
The harness's "wait for `\x1b[?2004h`" check IS insufficient on its
own — but the dominant cause was H2's goroutine leak, which consumed
the keystroke before the epoll subscription was even consulted. Once
H2's goroutine is eliminated, waiting for rendered content (which
guarantees the epoll subscription is active) is sufficient.

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

## Fix Applied

### Production fix — `internal/term/theme_unix.go`

`probeOSC11Background` was rewritten to use `pollReadOSC` instead of
`raceReadOSCReply`. `pollReadOSC` calls `syscall.SetNonblock(fd, true)`
then loops calling `syscall.Read`, sleeping 5 ms on `EAGAIN` and
breaking on `ctx.Err()`. No goroutine is spawned; the entire probe
runs on the calling goroutine and exits within ~5 ms of the context
deadline. `raceReadOSCReply` and `readOSCReply` remain in `theme.go`
and are exercised by their dedicated unit tests.

### Test harness changes — `tests/integration/pty_sanity_test.go`

- **`TestPTYSanity_QuitOnQ`**: waits for `"2 lines"` (streaming-complete
  footer for the 2-line fixture) before sending a single `q`. The 250 ms
  sleep and 5-iteration retry loop were removed.
- **`TestPTYSanity_QuitOnQBigFile`**: waits for `"line"` (first viewport
  content row) before sending a single `q`. The 500 ms sleep was removed.

### Other changes

- **`tests/integration/pty.go`**: `BracketedPasteEnable` constant added
  with a doc comment explaining it is emitted *before* `initCancelReader`
  and is therefore NOT a reliable input-ready signal.
- **`tests/perf/dismiss_bench_test.go`**: the "drop" comment updated;
  the retransmit/10 ms floor pattern is retained for SC-007 measurement
  fidelity (not as a race workaround).

## Conclusion

The primary root cause was **H2 — goroutine leak in `probeOSC11Background`**.
The `readOSCReply` goroutine (G1) remained alive in a blocking `read()`
on `/dev/tty`'s file description after the 50 ms OSC probe budget
expired. On Linux, `close(fd)` does not interrupt a blocked `read()`,
so G1 competed with Bubble Tea's cancelreader for the first byte
arriving on the PTY slave. G1 won consistently at early test timing
(T ≈ 100 ms) because its OS thread was already in the `read()` syscall,
and intermittently at later timing (~1–5%) because of goroutine
scheduler variability.

H3 (Bubble Tea's epoll subscription not yet established) was a
contributing factor for the "first paint is not a reliable signal"
observation, but not the dominant cause. The rendered-content wait is
still the correct harness signal because it guarantees the epoll
subscription is active.

A concrete fix required either:

1. ~~**Bubble Tea upstream change**~~ — adding a `Program.OnReady` hook
   (still open as a follow-up, but not blocking).
2. ~~**Harness change only**~~ — insufficient by itself; rendered-content
   waits still failed because G1 consumed the keystroke.
3. **Production fix** — **implemented**: `pollReadOSC` in
   `internal/term/theme_unix.go` replaces the goroutine with an inline
   O_NONBLOCK spin loop.

## Action Items

1. ~~**Upstream follow-up**~~: H3's epoll-race is now a non-issue since
   H2 is fixed; the rendered-content wait handles the residual ordering
   concern. The Bubble Tea `OnReady` issue remains open for consideration
   in a future release but is no longer urgent.
2. **Re-evaluate** if the OSC probe is ever ported to a platform where
   `O_NONBLOCK` on a `/dev/tty`-like device is not supported.

## Reproducer Notes

To reproduce locally with high probability:

```sh
go test -count=20 -run TestPTYSanity_QuitOnQ ./tests/integration/...
```

The flake rate is roughly 1-5% per iteration on bare-metal Linux,
higher under CI noise.
