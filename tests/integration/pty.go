// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package integration provides PTY-driven end-to-end harnesses for tests
// that exercise capability paths (alt-screen, signal handling, graphics
// emit, theme probes) which cannot be covered by unit tests alone.
package integration

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// AltScreenEnter is the canonical CSI sequence Bubble Tea emits when
// it switches into the alt-screen. Tests use it to wait for first
// paint without parsing intermediate frames.
const AltScreenEnter = "\x1b[?1049h"

// PTYProgram is the handle returned by [NewPTYProgram]. It exposes the
// minimal surface integration tests need: write keystrokes, read raw
// PTY bytes, snapshot the buffered output so far, close the PTY, and
// retrieve the exit code.
//
// PTYProgram is safe for sequential use only — Send, Read, Snapshot, and
// Close from a single goroutine. Close is idempotent.
type PTYProgram struct {
	t *testing.T

	cmd *exec.Cmd
	pty *os.File

	mu   sync.Mutex
	buf  bytes.Buffer // accumulated PTY output
	done bool
	err  error // wait error, captured by drainer

	exit  int
	exitR bool

	closeOnce sync.Once
}

// PTYOptions tune the spawn surface. The zero value yields an 80x24
// terminal with the spy binary at the spec-default path.
type PTYOptions struct {
	// BinaryPath, when non-empty, is used as the spy binary instead of
	// `go build`-ing the package. Tests that build the binary in their
	// own t.TempDir() should set this.
	BinaryPath string
	// Cols, Rows control the initial PTY size. Defaults to 80x24.
	Cols, Rows uint16
	// CWD, when non-empty, is the working directory of the spawned
	// process. Defaults to t.TempDir().
	CWD string
	// SkipCleanup, when true, prevents [NewPTYProgramOpts] from
	// registering a [testing.T.Cleanup] that closes the PTY at end of
	// test. Benchmarks that spawn many programs in a loop should use
	// this and call [PTYProgram.Close] explicitly per iteration —
	// otherwise every iteration's PTY FD + drain buffer stays alive
	// until the test returns, exhausting FDs and bloating memory.
	SkipCleanup bool
}

// NewPTYProgram builds the spy binary (if needed) and spawns it under a
// fresh PTY with the given args and environment. Tests that don't have
// a PTY-capable platform (Windows without ConPTY support, etc.) are
// skipped with a clear reason.
//
// The caller is responsible for calling [PTYProgram.Close] (typically
// via t.Cleanup).
func NewPTYProgram(t *testing.T, args []string, env map[string]string) *PTYProgram {
	t.Helper()
	return NewPTYProgramOpts(t, args, env, PTYOptions{})
}

// NewPTYProgramWithStdin spawns spy with stdin attached to the
// returned `stdin` pipe and stdout/stderr attached to a PTY. Tests
// for the US5 stdin-piping path use this to simulate the contract
// `cat file.go | spy` (stdin: non-TTY pipe; stdout: TTY).
//
// The caller writes input bytes into `stdin` and closes it to signal
// EOF (so spy's loader sees end-of-stream and the streaming "…"
// indicator collapses to the final line count). The PTY remains the
// caller's window onto the alt-screen frames.
//
// `t.Cleanup` closes both PTY and stdin pipe at end of test.
func NewPTYProgramWithStdin(t *testing.T, args []string, env map[string]string, opts PTYOptions) (p *PTYProgram, stdin *os.File) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY harness requires a Unix-like OS (creack/pty does not provide ConPTY parity)")
	}
	bin := opts.BinaryPath
	if bin == "" {
		bin = buildBinary(t)
	}
	cwd := opts.CWD
	if cwd == "" {
		cwd = t.TempDir()
	}
	cols := opts.Cols
	if cols == 0 {
		cols = 80
	}
	rows := opts.Rows
	if rows == 0 {
		rows = 24
	}

	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	if err := pty.Setsize(master, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		_ = master.Close()
		_ = slave.Close()
		t.Fatalf("pty.Setsize: %v", err)
	}

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		_ = master.Close()
		_ = slave.Close()
		t.Fatalf("os.Pipe: %v", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = cwd
	cmd.Env = mergeEnv(env)
	cmd.Stdin = stdinR
	cmd.Stdout = slave
	cmd.Stderr = slave
	// Make the PTY slave the controlling terminal of the spawned
	// process — without Setsid+Setctty, golang.org/x/term.IsTerminal(1)
	// returns true on the slave fd but Bubble Tea's renderer fails to
	// install signal handlers and key parsing. With these set the
	// child's session leader is the new pgid and the slave at fd 1
	// (cmd.Stdout) is its controlling tty.
	//
	// Ctty: 1 — the slave is at cmd.Stdout (child fd 1), not stdin
	// (which we routed to a pipe for the test input). Without an
	// explicit Ctty index Setctty defaults to fd 0 and ioctl fails
	// with "inappropriate ioctl for device" because the pipe isn't
	// a tty.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 1}

	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		_ = stdinR.Close()
		_ = stdinW.Close()
		t.Fatalf("cmd.Start: %v", err)
	}
	// Close our copies of the slave and stdin-read ends — only the
	// child holds them past this point.
	_ = slave.Close()
	_ = stdinR.Close()

	p = &PTYProgram{t: t, cmd: cmd, pty: master}
	if !opts.SkipCleanup {
		t.Cleanup(func() {
			_ = stdinW.Close()
			_ = p.Close()
		})
	}
	go p.drain()
	return p, stdinW
}

// NewPTYProgramOpts is the option-bearing form of [NewPTYProgram].
func NewPTYProgramOpts(t *testing.T, args []string, env map[string]string, opts PTYOptions) *PTYProgram {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY harness requires a Unix-like OS (creack/pty does not provide ConPTY parity)")
	}
	bin := opts.BinaryPath
	if bin == "" {
		bin = buildBinary(t)
	}
	cwd := opts.CWD
	if cwd == "" {
		cwd = t.TempDir()
	}
	cols := opts.Cols
	if cols == 0 {
		cols = 80
	}
	rows := opts.Rows
	if rows == 0 {
		rows = 24
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = cwd
	cmd.Env = mergeEnv(env)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		t.Fatalf("pty.Start: %v", err)
	}
	p := &PTYProgram{t: t, cmd: cmd, pty: f}
	if !opts.SkipCleanup {
		t.Cleanup(func() {
			_ = p.Close()
		})
	}
	go p.drain()
	return p
}

// Send writes raw bytes to the PTY (e.g., `q`, `\x1b[B`, `:1\r`). The
// caller is responsible for any required escape sequences.
func (p *PTYProgram) Send(s string) {
	p.t.Helper()
	if _, err := p.pty.Write([]byte(s)); err != nil {
		p.t.Fatalf("pty write %q: %v", s, err)
	}
}

// Read returns and consumes every byte accumulated since the last
// successful [PTYProgram.Read]. [PTYProgram.Snapshot] is non-consuming
// and does not affect this cursor. Returns the empty slice when
// nothing is buffered.
func (p *PTYProgram) Read() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]byte(nil), p.buf.Bytes()...)
	p.buf.Reset()
	return out
}

// Snapshot returns every byte accumulated since the program started
// (or since the last [PTYProgram.Read], whichever is newer) without
// consuming the buffer. Tests use it to scan for substrings or match
// against goldens after [PTYProgram.WaitFor].
func (p *PTYProgram) Snapshot() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.buf.Bytes()...)
}

// WaitFor blocks until `needle` appears in the accumulated PTY output
// or `timeout` elapses. Returns true on success. The buffer is not
// consumed; use [PTYProgram.Snapshot] / [PTYProgram.Read] afterwards.
func (p *PTYProgram) WaitFor(needle string, timeout time.Duration) bool {
	p.t.Helper()
	// Convert the needle once; bytes.Contains over the precomputed
	// slice avoids an allocation on every polling iteration.
	target := []byte(needle)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		hit := bytes.Contains(p.buf.Bytes(), target)
		p.mu.Unlock()
		if hit {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// WaitForExit blocks until the spawned process exits or `timeout`
// elapses. Returns true on clean exit (any code).
func (p *PTYProgram) WaitForExit(timeout time.Duration) bool {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		done := p.done
		p.mu.Unlock()
		if done {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Resize sends a SIGWINCH-equivalent resize event to the PTY.
func (p *PTYProgram) Resize(cols, rows uint16) {
	p.t.Helper()
	if err := pty.Setsize(p.pty, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		p.t.Fatalf("pty resize: %v", err)
	}
}

// Signal forwards a Unix signal to the spawned process (NOT the
// process group). Children spawned by the binary itself are not
// signaled; if a future test needs group-wide signaling, the spawn
// path should call Setpgid via SysProcAttr and this method can route
// through syscall.Kill on the negative pgid.
func (p *PTYProgram) Signal(sig os.Signal) {
	p.t.Helper()
	if p.cmd.Process == nil {
		return
	}
	if err := p.cmd.Process.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) {
		p.t.Fatalf("send signal %v: %v", sig, err)
	}
}

// Close tears down the PTY and reaps the process. Idempotent.
//
// Returns the first PTY-close error if any; the process kill and
// drain-reap paths swallow their errors because they're best-effort
// (the process may already be defunct).
func (p *PTYProgram) Close() error {
	var err error
	p.closeOnce.Do(func() {
		err = p.pty.Close()
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		// Best-effort wait — the drainer goroutine handles the real
		// reap and stores the exit code; we just wait briefly to give
		// it a chance to finish.
		for i := 0; i < 50; i++ {
			p.mu.Lock()
			done := p.done
			p.mu.Unlock()
			if done {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	return err
}

// ExitCode returns the process exit code; calls [PTYProgram.WaitForExit]
// with a 2s timeout if the process hasn't exited yet. Returns -1 if the
// process is still running after the timeout.
func (p *PTYProgram) ExitCode() int {
	p.t.Helper()
	if !p.WaitForExit(2 * time.Second) {
		return -1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.exitR {
		return p.exit
	}
	return -1
}

// drain copies PTY output into the buffer and reaps the process when
// the read returns EOF / the PTY closes. Runs as a goroutine for the
// lifetime of the PTYProgram.
func (p *PTYProgram) drain() {
	buf := make([]byte, 4096)
	for {
		n, err := p.pty.Read(buf)
		if n > 0 {
			p.mu.Lock()
			p.buf.Write(buf[:n])
			p.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	werr := p.cmd.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done = true
	p.err = werr
	if p.cmd.ProcessState != nil {
		p.exit = p.cmd.ProcessState.ExitCode()
		p.exitR = true
		// On signal-induced termination, ExitCode() is -1 but the
		// SIGINT/SIGTERM convention is 128+sig; reconstruct it.
		if p.exit == -1 {
			if ws, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				p.exit = 128 + int(ws.Signal())
			}
		}
	}
}

// buildBinary `go build`s the spy command into a process-cached
// path and returns it. The built binary is reused across NewPTYProgram
// calls in the same test by caching on a process-wide sync.Once.
//
// The build runs with the default tags; tests that need `-tags fitz`
// (PDF rasterization) override BinaryPath via PTYOptions and supply
// their own pre-built binary.
func buildBinary(t *testing.T) string {
	t.Helper()
	return buildOnce(t)
}

var (
	buildOnceMu sync.Mutex
	buildOnceP  string
	buildOnceE  error
)

// buildOnce performs the package-level cached build. Concurrent test
// invocations share the same binary; the build error (if any) is
// reported through t.Fatalf in every caller so individual tests fail
// rather than silently inheriting an empty path.
func buildOnce(t *testing.T) string {
	t.Helper()
	buildOnceMu.Lock()
	defer buildOnceMu.Unlock()
	if buildOnceP != "" || buildOnceE != nil {
		if buildOnceE != nil {
			t.Fatalf("build spy binary: %v", buildOnceE)
		}
		return buildOnceP
	}
	dir, err := os.MkdirTemp("", "spy-pty-bin-")
	if err != nil {
		buildOnceE = fmt.Errorf("mktemp: %w", err)
		t.Fatalf("%v", buildOnceE)
	}
	out := filepath.Join(dir, "spy")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/spy")
	// Walk up to the module root from the current package directory
	// so the build resolves go.mod regardless of where `go test` was
	// invoked from.
	cmd.Dir = moduleRoot(t)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(dir)
		buildOnceE = fmt.Errorf("go build: %w", err)
		t.Fatalf("%v", buildOnceE)
	}
	buildOnceP = out
	return out
}

// moduleRoot walks up from the current package directory until it
// finds a go.mod, returning that directory. Tests panic via t.Fatal
// if no go.mod ancestor exists.
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("module root not found above %s", wd)
		}
		dir = parent
	}
}

// mergeEnv composes the spawned process environment from the parent
// environment plus `env`. Keys in `env` override the inherited value.
// A nil `env` yields the parent environment unchanged.
func mergeEnv(env map[string]string) []string {
	parent := os.Environ()
	if len(env) == 0 {
		return parent
	}
	// Drop any inherited entry the caller is overriding.
	out := make([]string, 0, len(parent)+len(env))
	for _, kv := range parent {
		key := kv
		if i := indexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if _, ok := env[key]; ok {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// CopyOutput is a debugging helper that streams the PTY output to a
// writer in real time. Useful when developing a new test — call from
// the test setup with `os.Stderr` to see the spawned program's output
// inline.
func CopyOutput(p *PTYProgram, w io.Writer) {
	go func() {
		for {
			b := p.Read()
			if len(b) > 0 {
				_, _ = w.Write(b)
			}
			p.mu.Lock()
			done := p.done
			p.mu.Unlock()
			if done {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
}
