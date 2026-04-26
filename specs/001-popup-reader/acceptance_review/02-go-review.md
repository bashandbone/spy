<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos
SPDX-License-Identifier: MIT OR Apache-2.0
-->

# spy v0.1.0 Final Go Code Review

Scope: cmd/spy/, internal/{config,term,source,loader,highlight,graphics,render,search,keys,ui}/
Date: 2026-04-26

---

## CRITICAL — None found.

---

## HIGH

### H1 — SIGINT/SIGTERM exit codes violate contracts/cli.md
File: cmd/spy/main.go:207-215

Bubble Tea v1.3.10 handles SIGINT by sending an internal tea.Quit message; prog.Run() returns nil. The code then falls through to return exitOK (exit 0). The contract specifies exit 130 for SIGINT and exit 143 for SIGTERM. The integration tests that would catch this (TestSIGINTRestoresTerminal, TestSIGTERMRestoresTerminal in tests/integration/signal_test.go:32,38) are permanently skipped pending Phase 9.

Fix: Install a signal.Notify channel before tea.NewProgram, record which signal fired, and after prog.Run() returns nil use that record to exit with 128+signum. Alternatively re-raise the signal after cleanup so the shell reconstructs 128+N itself.

---

### H2 — Stream.Errs is never consumed by the production UI
File: internal/loader/stream.go:65-68; internal/ui/update.go (no reference to .Errs)

loader.Open produces ErrLineTruncated warnings and fatal I/O errors on stream.Errs. The UI code has zero references to stream.Errs and never calls stream.Buffer.Warnings(). Truncation of long lines (100 KiB cap) is silently swallowed, violating FR-013. The channel is buffered at UpdatesBuffer+2=6; once full, subsequent truncation warnings are dropped with the best-effort select/default, so the user never learns lines were clipped.

Fix: Wire a tea.Cmd (analogous to waitForChunk) that receives from stream.Errs and routes entries to m.statusAdvisory.

---

### H3 — Synchronous full-buffer search blocks the Bubble Tea event loop
File: internal/ui/update.go:435-451

search.Scan launches a goroutine and runSearch drains it synchronously with for hit := range ch, holding the Bubble Tea goroutine until all matches are found. MaxResidentBytes defaults to 0 (whole file in memory). On a 50 MB / 1 M line file this can freeze key and resize events for seconds. The search.State.Pending field exists but is never set; the async path is unimplemented.

Fix: Move the drain into a tea.Cmd. Return immediately with Pending:true and send a searchResultMsg when the goroutine finishes. Use a per-search context (derived from the model's cancel) so a new /pattern cancels the in-flight scan.

---

### H4 — Markdown renderer emits unsanitized ESC bytes to the terminal
File: internal/render/markdown.go:86-89

assembleRaw(lines) concatenates raw line bytes — which may contain \x1b/\x9b from a crafted file — and passes them directly to glamour.Render. Glamour's goldmark backend does not strip raw control bytes from non-code-block content; they pass through to the terminal. The code, text, and image renderers all call neutralizeEscapes at every emission boundary; the markdown path does not, leaving T109b.c partially enforced.

Fix: Apply neutralizeEscapes to l.Raw inside assembleRaw before appending.

---

### H5 — PDF text-extraction output is not sanitized
File: internal/render/pdf.go:186-205

p.GetPlainText(nil) returns text extracted from PDF content streams. A crafted PDF can embed arbitrary bytes — OSC window-title (\x1b]2;...BEL), DCS sequences, etc. — in its text. formatTextPage writes this verbatim to the string that reaches the terminal via viewport.SetContent. The PDF text path is the default in no-fitz builds (the common case).

Fix: Apply neutralizeEscapes to text before formatTextPage builds its output.

---

## MEDIUM

### M1 — In-flight :open loader goroutine can leak on rapid quit
File: internal/ui/update.go:703-727

runOpenCommand cancels the old stream (m.cancel(); m.cancel = nil) then returns a tea.Cmd that calls loader.Open with a fresh context. If the user types :open<Enter> then q before the openResultMsg arrives, Bubble Tea drops the message. The new stream's goroutine holds a context whose cancel is installed on the model only via onOpenResult, which now never runs. For large files the goroutine runs to completion as a leak.

Fix: Add a pendingCancel context.CancelFunc field to Model; set it when the :open cmd is dispatched; call it unconditionally from the ActionQuit handler.

---

### M2 — DisplayName not sanitized in image/PDF metadata blocks
Files: internal/render/image.go:134,141; internal/render/pdf.go:199,201,215

Linux filenames can contain \x1b bytes. fmt.Fprintf(&b, "[image: %s]\n", r.src.DisplayName()) passes the raw name into the terminal output. This requires a specially crafted filename but is the same class of injection as H4/H5.

Fix: Apply neutralizeEscapes to DisplayName() return values at each call site in metadataBlock and formatTextPage.

---

### M3 — pdfRenderer mutable cache is not documented as non-goroutine-safe
File: internal/render/pdf.go:38-68

cachedFrame, cachedText, cacheValid and related fields are accessed without synchronization. This is safe under the Bubble Tea single-goroutine model but the struct has no doc comment stating that constraint. A future pre-rendering worker would silently introduce data races.

Fix: Add a doc comment noting "Not goroutine-safe; Render must be called from the Bubble Tea event loop only."

---

### M4 — Fragile double-rc.Close on error path in loader.Open
File: internal/loader/stream.go:117-128

rc.Close() is called explicitly at line 118 on the readErr branch, then the streaming goroutine (line 143) would call it again via defer rc.Close() if it ran — it doesn't, but close(updates) and close(errs) follow immediately after. The explicit close interleaved with non-blocking channel sends is easy to accidentally duplicate in a future refactor.

Fix: Use defer rc.Close() immediately after the src.Open() success check and remove the two explicit _ = rc.Close() calls inside the early-return branches; the goroutine's defer at line 143 covers the async path.

---

### M5 — Total() returns 0 briefly, causing search to silently no-op
File: internal/loader/window.go:168-175

While streaming is in progress and the buffer is empty (race between waitForChunk and first WindowSizeMsg), Total() returns 0. search.scanLoop checks total <= 0 and returns without scanning. Users who type /pattern immediately after opening a large file see "no match" until the first chunk lands.

Fix: Document the return-0-while-loading semantics on Total(); callers in search.scanLoop can treat total==0 as "retry on next chunk" by not emitting a no-match advisory immediately.

---

## LOW

### L1 — neutralizeEscapes replaces \x1b with ? (ambiguous)
File: internal/render/sanitize.go:37-42
The substitution ? is indistinguishable from a literal question mark. Using the Unicode replacement character U+FFFD would make neutralized bytes visible and conventional.

### L2 — writeWrappedLine uses rune-count, not Unicode cell-width
File: internal/render/text.go:119-148
CJK and emoji (two-cell-wide) characters overflow wrap boundaries by one cell per wide rune. Documented Phase 2 limitation; affects CJK users noticeably.

### L3 — boolPtr(false) == nil, conflating false with unset
File: cmd/spy/main.go:320-325
boolPtr returns nil for false, making explicit --vim=false indistinguishable from not passing --vim. Works today because false is the default; fragile for future flags.

### L4 — sendWarning recover() guard too broad
File: internal/loader/window.go:300-306
defer func() { _ = recover() }() silently swallows any panic inside sendWarning, not just send-on-closed. Coordinate channel lifetime with sync.Once instead.

---

## Summary

| ID | Severity | Location | Issue |
|----|----------|----------|-------|
| H1 | HIGH | cmd/spy/main.go:207-215 | SIGINT/SIGTERM exit 0 instead of 130/143 |
| H2 | HIGH | internal/loader/stream.go:65-68 | Stream.Errs never read; truncation warnings lost |
| H3 | HIGH | internal/ui/update.go:435-451 | Synchronous search blocks event loop |
| H4 | HIGH | internal/render/markdown.go:86-89 | Markdown passes raw ESC bytes to terminal |
| H5 | HIGH | internal/render/pdf.go:186-205 | PDF text passes raw ESC bytes to terminal |
| M1 | MEDIUM | internal/ui/update.go:703-727 | :open goroutine leaks on rapid quit |
| M2 | MEDIUM | internal/render/image.go:134; pdf.go:199 | Filename not sanitized in metadata blocks |
| M3 | MEDIUM | internal/render/pdf.go:38-68 | pdfRenderer cache goroutine-safety undocumented |
| M4 | MEDIUM | internal/loader/stream.go:117-128 | Fragile double-close on error path |
| M5 | MEDIUM | internal/loader/window.go:168-175 | Total()==0 causes silent search no-op |
| L1 | LOW | internal/render/sanitize.go:37 | ? replacement ambiguous |
| L2 | LOW | internal/render/text.go:119 | Rune-count wrap ignores wide chars |
| L3 | LOW | cmd/spy/main.go:320-325 | boolPtr(false)==nil ambiguity |
| L4 | LOW | internal/loader/window.go:300-306 | recover() guard too broad |

Verdict: BLOCK — five HIGH findings must be resolved before tagging v0.1.0.
