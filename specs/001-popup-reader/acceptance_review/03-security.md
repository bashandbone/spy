<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos
SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Independent Security Review — spy v0.1.0

**Reviewer**: independent (claude-opus-4-7)
**Date**: 2026-04-26
**Repo state**: clean working tree on `main` (post-merge of #13)
**Scope**: validation of the implementer self-review at
`specs/001-popup-reader/checklists/security-review.md` (T109b),
plus exploratory review of (1)–(9) from the parent prompt.

---

## TL;DR

The implementer's self-review **overstates** the safety posture in two
material ways:

1. **(e) Graphics decoder safety — claim is FALSE.** There is no
   `defer recover()` anywhere in `internal/graphics/*.go` or around
   `image.Decode` / `go-fitz` calls in `internal/render/`. The cited
   `TestGraphics_RecoversFromDecoderPanic` does not exist. A
   panicking decoder will tear down the Bubble Tea program. (HIGH)
2. **(a) Path handling — file-mode rejection claim is FALSE.**
   `newFileSourceWithHint` (`internal/source/file.go:46`) only
   rejects directories. FIFOs, sockets, character devices, and block
   devices are accepted. `spy /dev/zero` will sit forever; opening a
   FIFO blocks indefinitely. (MEDIUM)

The other four checklist categories (b TOML fuzz, c escape
neutralization in text/code renderers, d OSC 11 regex, f no-network
gate) substantially hold up. However, escape neutralization has gaps
in the **status bar** and **PDF/image renderers** that the implementer
missed, and these are exploitable by simply naming a file with embedded
ESC bytes (HIGH; see Finding #1 below).

| # | Severity | Finding |
|---|----------|---------|
| 1 | HIGH | Filename-driven escape injection via status bar / PDF / image renderers |
| 2 | HIGH | No panic recovery around go-fitz / image.Decode (claimed but absent) |
| 3 | MEDIUM | File-mode rejection limited to directories — FIFOs/devices/sockets accepted |
| 4 | MEDIUM | Stderr emission of attacker-controlled paths bypasses neutralization |
| 5 | MEDIUM | TOCTOU between `EvalSymlinks` → `Stat` → `Open` (no `O_NOFOLLOW`) |
| 6 | MEDIUM | OSC 11 read loop bounded only by 64 B and ctx — slow per-byte exhaustion possible |
| 7 | LOW | Glamour markdown renderer never sees `neutralizeEscapes` (see Finding #7) |
| 8 | LOW | CI no-network gate has narrow blind spots (`net/http.Client{}` literal, `url.Parse`-driven libs) |
| 9 | LOW | `runDegenerate` copies attacker bytes verbatim to stdout (intentional but worth documenting) |
| 10 | LOW | `parseColorFGBG` accepts whitespace-trimmed last field — not exploitable but loose |
| 11 | LOW | Reload TOCTOU theoretical but unexploitable in practice |

No CRITICAL findings (no exploitable RCE, no auth-bypass, no data
exfiltration channel). The HIGH items are realistic terminal hijack
/ availability bugs against a v0.1.0 install.

---

## Validation of implementer's six categories

### (a) Path handling — PARTIAL

Verified facts:

- `internal/source/source.go:194` calls `filepath.EvalSymlinks(path)`,
  which internally `Clean`s. Symlinks are followed.
- `internal/source/file.go:42` calls `os.Stat(resolved)`.
- `internal/source/file.go:46` rejects `info.IsDir()`.
- The pseudo-fs denylist is correctly marked FOLLOWUP.

**Refuted claim**: the implementer states "PASS — `newFileSource`
calls `os.Stat` and rejects non-regular files with `ErrUnsupported`."
This is **not what the code does**. Only directories are rejected.
See Finding #3 for the realistic exploit and fix.

### (b) TOML parser robustness — PASS

- `internal/config/fuzz_test.go` has an adequate adversarial corpus
  (8 seeds). I ran `go test -fuzz=FuzzConfigLoad -fuzztime=20s` and
  it passed with 109 new interesting inputs and 0 crashes.
- The fuzz target asserts `cfg != nil` after `Load`, matching the
  contract.
- Seeds could be richer (very long table-key chains, NaN/Inf floats,
  duplicate-key shadowing, time values past year 9999), but the
  current set is sufficient for v0.1.0.

### (c) Terminal escape injection from file content — PARTIAL

The implementer's checklist enumerates the call sites for
`neutralizeEscapes`:

- `code.go::styleLine` — every fallback path: VERIFIED
- `code.go::Render` — match-overlay mono path + wrap path: VERIFIED
- `match.go::applyMatchHighlights` — VERIFIED
- `text.go` — both wrap and no-wrap paths: VERIFIED

**Gaps the checklist missed:**

- `internal/render/markdown.go::Render` passes raw bytes through
  Glamour. Glamour will pass through ESC bytes embedded inside code
  fences (` ``` ... ``` `) verbatim. See Finding #7.
- `internal/render/image.go:134,142` and `internal/render/pdf.go:199,
  201,215` emit `r.src.DisplayName()` and `md.Path` via `fmt.Fprintf`
  — no neutralization. See Finding #1.
- `internal/render/statusbar.go` renders `in.DisplayName` and
  `in.Advisory` verbatim through `theme.Footer.Render` — no
  neutralization. The advisory carries user-controlled values like
  `"open <path>: <err>"` and `"invalid pattern: %v"`. See Finding #1.
- `cmd/spy/main.go:295,304,307,310,313` print attacker-controlled
  paths to **stderr** without sanitization. See Finding #4.

The token-level neutralization in `code.go` (`needsTokenNeutralization`
+ `neutralizeTokens`) is well thought out — Chroma's `Text` token can
copy raw bytes through. But it only protects the code renderer, not
markdown/PDF/image/statusbar/stderr.

### (d) OSC 11 reply parsing — PASS, with one MEDIUM gap

- `internal/term/theme.go:37` regex is correctly anchored with `^...$`,
  rejects oversize replies up front (line 64), and only allows hex +
  `/` + a single terminator (BEL or ST).
- The hex-component widths are bounded to 1–4 chars, then re-bounded
  in `parseHexComponent` (line 89).
- The function is pure; no echo path. The doc comment correctly
  identifies the CSI-smuggling attack and rules it out.

The read side (`readOSCReply`, `internal/term/theme.go:244`) reads
**one byte at a time** and stops at `seenTerminator` OR
`oscReplyMaxBytes` (64) OR ctx cancel OR read error. **Important
nuance**: the per-iteration `Read` blocks indefinitely with no
per-byte deadline — only the outer 50 ms ctx (`oscProbeBudget`)
caps total wall time. A hostile terminal that drip-feeds one byte
every 49 ms would still exit cleanly. Correct in theory; the actual
behavior is bounded by the goroutine race in `raceReadOSCReply`. See
Finding #6 for the small remaining concern.

### (e) Graphics decoder safety — FAIL

This claim is **incorrect** in two ways:

1. **No `defer recover()` exists** in `internal/graphics/graphics.go`.
   `grep -n "recover" internal/graphics/*.go` returns matches in
   `graphics_test.go` only (testing `CleanupFunc`'s `sync.Once`
   no-op). The cited "internal/graphics/graphics.go:60" is a
   comment about `sync.Once` idempotency, not a recovery handler.
2. **The cited test does not exist.** `grep -r
   "TestGraphics_RecoversFromDecoderPanic" /home/knitli/spy` returns
   zero hits.

`go-fitz` v1.24.15 is a CGo wrapper around MuPDF. MuPDF's
fz_throw/fz_catch error model is bridged to Go errors by the binding,
**but** the underlying C library has documented heap-corruption and
SIGSEGV cases on adversarial PDFs (the binding does set up a longjmp
guard, but a SIGSEGV during decode kills the process — Go cannot
recover from Go-runtime-detected SIGSEGV in CGo when the segfault is
inside the C library).

`image.Decode` from the stdlib + `golang.org/x/image/{bmp,webp}` are
generally panic-safe but historically produce `runtime error: index
out of range` on truncated WebP / BMP headers (CVE-2021-3115 style).

See Finding #2 for the fix.

### (f) No accidental network calls — PASS

The CI grep gate at `.github/workflows/ci.yml:126` is:
```
grep -rE '(\bhttp\.(Get|Post|Head|Do|Client|NewRequest)\b|\bnet\.Dial[A-Za-z]*\b|\bhttp\.NewRequest\b)' \
    internal/ cmd/ --include='*.go' | grep -v '_test\.go:'
```

The implementer's checklist documents an earlier looser `\.Get(`
pattern that was correctly tightened so chroma's `styles.Get(...)`,
`lexers.Get(...)`, `formatters.Get(...)` calls (verified at 11 call
sites) don't false-positive. I re-ran the gate locally — passes
clean. See Finding #8 for the small remaining blind spots.

---

## Findings

### #1 — HIGH: Filename-driven escape injection bypasses `neutralizeEscapes`

**Files**:
- `internal/render/statusbar.go:119,123,130,131,181-183` (advisory + display name)
- `internal/render/image.go:134,141-142` (display name + metadata path)
- `internal/render/pdf.go:199,201,215` (display name)
- `internal/ui/update.go:710` (advisory built from `path` + err)

**Attack vector**: Linux (and most Unix file systems) accept ESC
(`\x1b`), BEL (`\x07`), CSI (`\x9b`), and any other control byte in
filenames. I confirmed this by creating
`/tmp/spysec/evil$'\x1b]2;hijack\x07'.txt` successfully. An attacker
who can plant a file whose **basename** contains the OSC 2 sequence
— a tarball with a malicious filename, a `git clone` of a hostile
repo, an HTTP-Last-Modified-named download — can trigger:

- `spy <evil_file>` → `filepath.Base(path)` returns the raw bytes →
  flows into `m.source.DisplayName()` → `footerLine()` builds
  `StatusInput.DisplayName` → `renderFull`/`renderCollapsed` writes
  it through `theme.Footer.Render(line)`. Lipgloss does not strip
  control bytes; the embedded `\x1b]2;hijack\x07` will reach the
  user's terminal and **change the window title**.
- `spy <evil_pdf>` → `pdfRenderer.formatTextPage` line 199 emits
  `[pdf: <name> — page m/n]` with raw bytes.
- `spy <evil_img>` → `imageRenderer.metadataBlock` line 134 emits
  `[image: <name>]` with raw bytes when graphics aren't available.
- `:open <evil_path>` (runtime) → on failure produces
  `m.statusAdvisory = fmt.Sprintf("open %s: %v", path, err)` at
  `update.go:710` — `path` flows verbatim into the advisory and into
  the next status-bar render. Even on success the new file becomes
  the active source and triggers the DisplayName path above.

**Worst case**: OSC 52 (clipboard write — supported by xterm,
foot, kitty, alacritty, wezterm, mintty, st), OSC 8 (hyperlink with
arbitrary `file://` target), DCS sequences (more terminal-specific
RCE primitives like `\eP+q...\e\\` on certain emulators).

**PoC** (do not run unless you mean to):
```
mkdir -p /tmp/poc
touch $'/tmp/poc/x\x1b]2;PWNED\x07x.txt'
echo hello > $'/tmp/poc/x\x1b]2;PWNED\x07x.txt'
spy /tmp/poc/x*.txt
```

**Fix**: Apply `neutralizeEscapes` (or a narrower equivalent) to
**every** string that originates from the source-side and reaches the
TTY. The minimal patch:

1. In `internal/source/file.go:51`, neutralize `displayName`:
   `displayName: neutralizeFilename(filepath.Base(path))`.
   Define a small `neutralizeFilename` helper in the source package
   so render doesn't have to import sanitization logic per emit
   site. The basename is already the "display name", so changing
   it once is the right layering.
2. Apply the same to `Metadata.Path` at construction time
   (`internal/source/file.go:107`).
3. Apply `neutralizeEscapes` to `StatusInput.Advisory` in
   `internal/render/statusbar.go::renderFull` and `renderCollapsed`
   before the `theme.Footer.Render` call.
4. Apply `neutralizeEscapes` to user-controlled formatted strings
   in `internal/ui/update.go:710,422,632,645,696` etc. before they
   land in `m.statusAdvisory`.

The byte-for-byte property of `neutralizeEscapes` (`\x1b` → `?`)
preserves all width math, so no downstream breakage.

---

### #2 — HIGH: Claimed `defer recover()` around image / PDF decoders does not exist

**Files**:
- `internal/graphics/graphics.go` (entire file — no recover)
- `internal/render/image.go:117` (`image.Decode` call, unprotected)
- `internal/render/pdf_fitz.go:38,46` (`fitz.NewFromMemory`, `doc.Image`, unprotected)

**Attack vector**: A malformed image (truncated PNG IHDR, malformed
WebP container, BMP with deliberately bad scanline width) or PDF
(corrupt xref table, malformed CMap, deeply nested object graph) can
panic the underlying decoder. With graphics enabled (kitty/iTerm2/
sixel detected), the `:open` flow or initial load path will trigger
`r.decode()` (`image.go:117`) or `rasterizePDFPage`
(`pdf_fitz.go:38`) and the panic propagates up through
`renderFresh` → `Render` → `viewport.SetContent` → out of any tea
handler. Bubble Tea's program panics with `tea.Quit`-equivalent
behavior; the user's terminal is left in alt-screen mode and
graphics protocols are NOT cleaned up because the deferred
`cleanupGraphics()` in `cmd/spy/main.go:147` only runs on normal
return — Go panics traverse defers, so the cleanup defer DOES run,
but only after Bubble Tea's own panic prints; the alt-screen exit
sequence is fragile here.

`go-fitz` v1.24.15 is a CGo wrapper. The Go runtime cannot generally
recover from `SIGSEGV` originating in C code (CGo programs typically
crash hard). MuPDF historically has had several CVE-class memory
safety issues on adversarial PDFs:
[CVE-2024-46951](https://nvd.nist.gov/vuln/detail/CVE-2024-46951),
CVE-2023-29408, CVE-2022-38223 etc. While most are RCE-class for
older versions and patched in modern MuPDF, the panic / abort path
remains realistic.

**Severity rationale**: This is HIGH (not CRITICAL) because exit-2
denial of service against an interactive viewer is recoverable —
the user just relaunches. RCE through MuPDF would require a
fitz-tagged binary AND an unpatched MuPDF version AND a crafted
PDF; the intersection is small but non-zero.

**Fix**:
1. Wrap `image.Decode` in `internal/render/image.go::decode` with
   `defer func() { if r := recover(); r != nil { err = fmt.Errorf("decode panic: %v", r) } }()`.
2. Wrap `rasterizePDFPage` in `internal/render/pdf_fitz.go` with
   the same pattern — though note this only catches **Go-side**
   panics; a SIGSEGV inside MuPDF still kills the process.
3. Add an actual `TestImage_RecoversFromDecoderPanic` and
   `TestPDF_RecoversFromDecoderPanic` test in
   `internal/render/{image,pdf}_test.go` that feeds a known-bad
   payload (truncated PNG ending mid-IDAT chunk) through the
   renderer and asserts we get an error, not a panic.
4. Update the implementer's checklist to remove the false claim or
   point at the actual implementation once added.

---

### #3 — MEDIUM: File-mode check rejects only directories

**File**: `internal/source/file.go:46`

```go
if info.IsDir() {
    return nil, fmt.Errorf("%w: %s is a directory", ErrUnsupported, path)
}
```

**Attack vector**:
- `spy /dev/zero` → opens, never EOFs, loader reads 100 KiB lines
  into the buffer until `MaxResidentBytes` (default 256 MiB) is hit;
  then windowed mode kicks in but the file keeps streaming. Memory
  stays within budget but the loader goroutine and CPU spin
  forever. User has to kill the process.
- `spy /dev/random` (slow, but unbounded).
- `spy /tmp/some.fifo` where the writer is the attacker — the
  attacker controls when each byte arrives and can drip-feed
  forever, holding spy in `streaming = true` indefinitely.
- `spy /dev/sda` (block device) — needs root, not a real concern.
- `spy /tmp/some.sock` (unix socket) — `os.Open` returns an error
  on most systems for sockets, so this is a soft pre-existing
  protection.

**Fix**: After `os.Stat` at line 42, check the mode:
```go
mode := info.Mode()
switch {
case mode.IsDir():
    return nil, fmt.Errorf("%w: %s is a directory", ErrUnsupported, path)
case !mode.IsRegular():
    return nil, fmt.Errorf("%w: %s is not a regular file (mode=%s)", ErrUnsupported, path, mode)
}
```

`os.FileMode.IsRegular()` returns true only when no special-file
bits (`ModeDir | ModeSymlink | ModeNamedPipe | ModeSocket | ModeDevice
| ModeCharDevice | ModeIrregular`) are set. After `EvalSymlinks` the
`ModeSymlink` bit shouldn't appear, but covering everything is the
right defensive default.

---

### #4 — MEDIUM: Stderr writes attacker-controlled paths verbatim

**File**: `cmd/spy/main.go:304,307,310,313,316,295`

```go
fmt.Fprintf(os.Stderr, "spy: cannot open: %s: not found\n", target)
fmt.Fprintf(os.Stderr, "spy: cannot open: %s: permission denied\n", target)
fmt.Fprintf(os.Stderr, "spy: binary file: %s: refusing to render binary content\n", target)
fmt.Fprintf(os.Stderr, "spy: unsupported format: %s: %v\n", target, err)
```

**Attack vector**: Same as Finding #1 but on the stderr path that
fires *before* the alt-screen even starts. Even if the user pipes
spy's stdout somewhere else, stderr typically goes to the parent
terminal — so an `spy /tmp/$'evil\x1b]2;PWN\x07'` produces a window-
title hijack at the shell prompt level. Worse: if the user
investigates with `ls /tmp/`, modern `ls` does sanitize control
chars by default, but `spy`'s stderr message just printed the raw
bytes — the user's terminal might already be in a hijacked state.

**Fix**: Define a small `safePath(s string) string` helper in
`cmd/spy/main.go` (or import from `internal/render`) that strips/
substitutes `\x1b` and `\x9b` and call it on `target` and `err`'s
`%v` rendering before printing.

---

### #5 — MEDIUM: TOCTOU between EvalSymlinks → Stat → Open (no `O_NOFOLLOW`)

**Files**:
- `internal/source/source.go:194` (EvalSymlinks)
- `internal/source/file.go:42` (Stat)
- `internal/source/file.go:77,92,125` (Open)

**Attack vector**: An attacker with write access to a path the user
will pass to `spy`:
1. Time T0: writes a regular file `/tmp/legit.txt`.
2. User runs `spy /tmp/legit.txt`.
3. Time T1: `EvalSymlinks` resolves to `/tmp/legit.txt`, `Stat`
   sees it as a regular file, `IsDir()` returns false.
4. Time T2 (race window): attacker `mv /tmp/legit.txt /tmp/old &&
   ln -s /etc/shadow /tmp/legit.txt` (or any other target).
5. Time T3: `os.Open(s.path)` actually opens — but `s.path` is the
   `EvalSymlinks` *result*, not the original. So it opens the
   resolved target captured in step 3. **This particular case is
   safe.** The TOCTOU is on the resolved target instead.

The realistic TOCTOU is therefore: between step 3 and step 5, the
**resolved target** (`/tmp/legit.txt`) gets replaced with another
symlink or with a different file. Even though `s.path` is captured,
`os.Open` at step 5 walks the path again and follows whatever
symlinks exist *now*.

**Severity**: MEDIUM rather than HIGH because (a) for a viewer
running as the invoking user, the attacker would need write access
to a directory the user controls — a much smaller threat surface
than a setuid binary, and (b) the worst-case is reading a different
file the user already has access to (the viewer doesn't write).
But it does break the "the file you saw at startup is the file
you're viewing" invariant; on `:reload` the gap reopens.

**Fix**: Open with `O_NOFOLLOW` after the initial `EvalSymlinks`:

```go
f, err := os.OpenFile(s.path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
```

This causes `Open` to fail if `s.path` has become a symlink between
resolution and open. Combine with an `fstat` after open to compare
inode numbers against the inode captured at construction time, and
reject on mismatch. (Both require `internal/source/file_unix.go` /
`file_windows.go` build-tag separation; `O_NOFOLLOW` is POSIX-ish.)

This also closes the **reload TOCTOU** (Finding #11): on
`ActionReload`, the same `s.path` is reopened and an attacker who
swapped the file between the original load and the reload can sneak
binary content past the upfront detection (which doesn't re-run on
reload).

---

### #6 — MEDIUM: OSC 11 read loop has no per-byte deadline

**File**: `internal/term/theme.go:244` (`readOSCReply`)

The read loop pulls one byte at a time from `r.Read(buf[:])`, which
on a real `/dev/tty` blocks indefinitely until either a byte arrives
OR the FD is closed. The outer `raceReadOSCReply` (line 213) races
the goroutine against `ctx.Done()`, which fires after
`oscProbeBudget` (50 ms). When the ctx fires, the goroutine is
**leaked** — the parent returns nil, but the goroutine keeps
blocking inside `Read()` until something else closes the FD.
`probeOSC11Background` (`theme_unix.go:42`) defers `f.Close()` —
but that defer only runs after `raceReadOSCReply` returns, which
*does* trigger `EBADF` on the next Read in the leaked goroutine, so
this leak is actually short-lived. OK in practice.

The remaining concern: a hostile terminal that wants to **starve**
the probe could send 0 bytes for the full 50 ms; the parent gets
nil and falls back to COLORFGBG (which is fine). Not really
exploitable.

**Why I flagged it MEDIUM anyway**: The Read goroutine doesn't
honor ctx — it relies on FD-close to unblock. If a future change
moves the probe out of `defer f.Close()` discipline, this becomes
a real goroutine leak.

**Fix (defensive)**: Use `f.SetReadDeadline(time.Now().Add(50ms))`
before the read loop on platforms that support it (`*os.File` does
on Unix). Or use `unix.Poll` / `epoll` so the read is genuinely
interruptible by ctx cancellation. Lower-priority; current code
works.

---

### #7 — LOW: Glamour markdown renderer doesn't see neutralization

**File**: `internal/render/markdown.go:78-90`

```go
gr, err := glamour.NewTermRenderer(...)
body := assembleRaw(lines)  // raw line bytes joined
rendered, err := gr.Render(body)
```

`assembleRaw` builds the markdown body by concatenating
`source.Line.Raw` strings. If a markdown file contains an ESC byte
inside a fenced code block (or even outside one), Glamour passes
the bytes through more or less verbatim (Glamour styles markdown
**structure** with ANSI; it doesn't strip pre-existing control
bytes from the source).

**Severity**: LOW because the standard "magic byte" detection
(`internal/source/detect.go:201`) keys markdown off the `.md`/
`.markdown` extension, and getting an ESC byte into a markdown file
that the user opens in spy is a more constrained vector than
Finding #1. But it is the same class of bug.

**Fix**: Replace `assembleRaw`'s loop body with
`b.WriteString(neutralizeEscapes(l.Raw))`. The byte-for-byte
property preserves Glamour's column math.

---

### #8 — LOW: CI no-network gate has narrow blind spots

**File**: `.github/workflows/ci.yml:113-133`

The current regex catches the common forms but misses:
- `http.Client{Timeout: ...}` literal — the gate matches
  `\bhttp\.Client\b` only when followed by `(`, but the actual
  pattern is `http.Client{...}`. Quick fix: change the alternation
  to `\bhttp\.Client[{(]`.
- `(&http.Client{...}).Get(url)` style — caught indirectly because
  `http.Client` with a `(` doesn't appear, but the chained `.Get(`
  is intentionally unmatched (good — chroma uses it).
- A library that constructs a `*url.URL` and uses
  `http.RoundTripper` directly. Less common but possible.
- The `os/exec` route: `exec.Command("curl", ...)` would not be
  caught.

**Fix**: Expand the regex to cover `http.Client[{(]` and consider
adding an `exec.Command|exec.LookPath` guard if the project's
intended posture is "no subprocesses either". For v0.1.0 the gap
is acceptable — none of the imports do this.

---

### #9 — LOW: `runDegenerate` (cmd/spy/main.go:228) copies bytes verbatim

**File**: `cmd/spy/main.go:228`

`io.Copy(os.Stdout, rc)` on the non-TTY stdout path. By design this
is `cat`-equivalent, which means an attacker-controlled file with
embedded ESC sequences will still be written to stdout. If the user
later inspects the redirected file with `less`, they're protected by
`less -R`'s default. If they `cat` it, the escapes fire.

**This is the documented behavior** (`cat` works the same). Worth a
line in `docs/security.md` to say "spy in non-TTY mode is `cat` — if
you redirect untrusted content, treat it like `cat`'d untrusted
content." No code fix needed.

---

### #10 — LOW: `parseColorFGBG` accepts loose input

**File**: `internal/term/theme.go:133`

`strings.TrimSpace(parts[len(parts)-1])` will accept e.g.
`"  ; 7  "` as valid. Not exploitable (the value is a luminance
number that the renderer uses for theme selection). Mentioning for
completeness.

---

### #11 — LOW: Reload TOCTOU (covered by Finding #5)

`ActionReload` re-opens the source via `loader.Open` →
`src.Open()` → `os.Open(s.path)`. The cached kind from the
*original* detection is retained (`s.detected = true` short-circuits
in `detectOnce`). So an attacker who replaces the file between
original load and reload can:
- Have the original detected as KindText (passes binary check).
- Replace with a binary blob.
- On reload, the stream emits the binary bytes through the text
  renderer (which calls `neutralizeEscapes` — that's good).

Net effect: the binary check is bypassed but the escape sanitizer
is **not**. Worst case: the user sees garbled non-printable
characters in the viewer (subbed with `?`). Not exploitable for
terminal hijack thanks to (c). Worth fixing alongside #5 by
re-running detection on reload, or by switching to inode-bound
opens.

---

## Items confirmed safe (no finding)

- **Stdin OOM via crafted stream**: cannot construct a single line
  > 100 KiB (`MaxLineBytes = 100 * 1024`), and the loader truncates
  cleanly via `appendBounded`. Multi-GB single line claimed in the
  prompt would be truncated at byte 102400 and the rest silently
  consumed. `MaxResidentBytes` (default 256 MiB) caps total
  buffer footprint; eviction kicks in past that threshold. Any
  single line cap stays bounded.
- **Integer overflow in window math**: spot-checked
  `internal/loader/window.go::Slice` arithmetic. `int64` throughout;
  no narrowing conversions in the hot path. The `int(wantStartNum -
  residentStart)` conversion at line 224 is bounded by the resident
  buffer size, which `MaxResidentBytes` keeps under int32 max in
  practice.
- **Permissions on temp files**: confirmed none are created in
  product code (`grep -E "TempDir|TempFile|MkdirTemp|os.Create"`
  returns hits only in `_test.go` files).
- **Env var trust**: `NO_COLOR`, `COLORFGBG`, `SPY_*`, `TERM`,
  `TERM_PROGRAM`, `XDG_CONFIG_HOME`, `KITTY_WINDOW_ID`, `TMUX`,
  `COLUMNS`, `LINES`, `COLORTERM` are all consumed only as
  config selectors with bounded enums or numeric parses. None
  reach `exec.Command` or filesystem-mutation APIs. `XDG_CONFIG_HOME`
  builds a path that goes to `os.ReadFile` — the user can already
  set their own XDG dir; no privilege escalation.
- **Graphics encoder cols/rows**: every encoder
  (`encodeKitty`, `encodeITerm2`, `encodeSixel`) ignores the
  `cols`/`rows` parameters today (the protocol scales server-side).
  No buffer-size math is driven by the values, so no overflow.
- **Context cancellation discipline**: `cancel()` is called on the
  loader before `tea.Quit` and on every reload/open path. No
  obvious goroutine leaks under normal exit.

---

## Recommended pre-tag actions

Before tagging v0.1.0, in priority order:

1. **Fix Finding #1** (filename-driven escape injection) — small
   patch, high impact. Add an integration test that creates a file
   whose basename contains `\x1b]2;test\x07` and asserts the
   rendered status bar / metadata block contains no `\x1b]`.
2. **Fix Finding #2** (add the `defer recover()` the checklist
   already claims exists) — even a Go-side panic guard makes the
   stated property true. Add the cited test.
3. **Fix Finding #3** (reject non-regular files) — three lines.
4. **Fix Finding #4** (sanitize stderr paths) — small helper +
   five call sites.
5. **Update `specs/001-popup-reader/checklists/security-review.md`**
   to reflect the actual implementation. The current checklist
   misleads any future reader about what is actually defended.

Findings #5–#11 are tag-acceptable as documented FOLLOWUPs for
v0.1.x.
