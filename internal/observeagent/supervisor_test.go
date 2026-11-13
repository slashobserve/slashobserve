package observeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const (
	childArg    = "--observe-agent-test-child"
	testTimeout = 30 * time.Second
)

func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == childArg {
		runTestChild(os.Args[2])
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runTestChild(mode string) {
	switch mode {
	case "serve":
		terms := make(chan os.Signal, 1)
		signal.Notify(terms, syscall.SIGTERM)
		fmt.Printf(`{"level":"info","msg":"child started","token":%q}`+"\n", os.Getenv(EnvToken))
		<-terms
		fmt.Println(`{"level":"info","msg":"terminated"}`)
	case "serve-then-exit":
		fmt.Printf(`{"level":"info","msg":"child started","token":%q}`+"\n", os.Getenv(EnvToken))
		os.Exit(3)
	case "exit":
		os.Exit(1)
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		fmt.Println(`{"level":"info","msg":"child started"}`)
		time.Sleep(time.Hour)
	case "log":
		terms := make(chan os.Signal, 1)
		signal.Notify(terms, syscall.SIGTERM)
		fmt.Println(`{"level":"warn","msg":"hello","k":"v"}`)
		fmt.Fprintln(os.Stderr, "oops")
		fmt.Println(`{"level":"info","msg":"child started"}`)
		<-terms
	}
}

func (r *recorder) next(t *testing.T, msg string) record {
	t.Helper()
	timeout := time.After(testTimeout)
	for {
		select {
		case rec := <-r.shared.live:
			if rec.msg == msg {
				return rec
			}
		case <-timeout:
			t.Fatalf("no log record %q within %v; saw %+v", msg, testTimeout, r.all())
		}
	}
}

func (r *recorder) collect(t *testing.T, msgs ...string) map[string]record {
	t.Helper()
	seen := map[string]record{}
	timeout := time.After(testTimeout)
	for len(seen) < len(msgs) {
		select {
		case rec := <-r.shared.live:
			if slices.Contains(msgs, rec.msg) {
				seen[rec.msg] = rec
			}
		case <-timeout:
			t.Fatalf("log records %q not all seen within %v; saw %+v", msgs, testTimeout, r.all())
		}
	}
	return seen
}

type harness struct {
	s      *Supervisor
	rec    *recorder
	events chan string
	exits  atomic.Int32
	cancel context.CancelFunc
	done   chan struct{}
	ctx    context.Context
}

func newHarness(t *testing.T, mode string) *harness {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{rec: newRecorder(), events: make(chan string, 64), done: make(chan struct{})}
	h.s = NewSupervisor(SupervisorConfig{
		Path:      exe,
		ServerURL: "http://127.0.0.1:1",
		Slots:     2,
		LogLevel:  slog.LevelInfo,
		Logger:    h.rec.logger(),
		OnExit: func(context.Context) {
			h.exits.Add(1)
			h.events <- "exit"
		},
	})
	h.s.args = []string{childArg, mode}
	h.s.sleep = func(ctx context.Context, d time.Duration) {
		h.events <- "sleep " + d.String()
		<-ctx.Done()
	}
	h.ctx, h.cancel = context.WithCancel(context.Background())
	t.Cleanup(h.cancel)
	return h
}

func (h *harness) start() {
	go func() {
		defer close(h.done)
		h.s.Run(h.ctx)
	}()
}

func (h *harness) stop(t *testing.T) {
	t.Helper()
	h.cancel()
	h.wait(t)
}

func (h *harness) wait(t *testing.T) {
	t.Helper()
	select {
	case <-h.done:
	case <-time.After(testTimeout):
		t.Fatal("Run did not return after the context ended")
	}
}

func (h *harness) event(t *testing.T) string {
	t.Helper()
	select {
	case e := <-h.events:
		return e
	case <-time.After(testTimeout):
		t.Fatal("no supervisor event in time")
		return ""
	}
}

func TestSupervisorGivesChildATokenAndAuthenticatesIt(t *testing.T) {
	h := newHarness(t, "serve")
	h.start()

	token, _ := h.rec.next(t, "child started").attrs["token"].(string)
	if !strings.HasPrefix(token, TokenPrefix) {
		t.Fatalf("child token = %q, want prefix %q", token, TokenPrefix)
	}
	if !h.s.Authenticate(token) {
		t.Fatal("Authenticate refused the running child's token")
	}
	for _, other := range []string{"", token + "x", token[:len(token)-1], "soa_other"} {
		if h.s.Authenticate(other) {
			t.Fatalf("Authenticate accepted %q", other)
		}
	}

	h.stop(t)
	if h.s.Authenticate(token) {
		t.Fatal("Authenticate accepted the token after the child stopped")
	}
}

func TestSupervisorExitReleasesRunsOnceAndRotatesTheToken(t *testing.T) {
	h := newHarness(t, "serve-then-exit")
	h.start()

	first, _ := h.rec.next(t, "child started").attrs["token"].(string)
	if got := h.event(t); got != "exit" {
		t.Fatalf("first event = %q, want exit", got)
	}
	if h.s.Authenticate(first) {
		t.Fatal("Authenticate accepted the token of a child that exited")
	}
	if got := h.event(t); got != "sleep 1s" {
		t.Fatalf("second event = %q, want sleep 1s: OnExit must run once, before the backoff", got)
	}

	exited := h.rec.next(t, "observe-agent exited")
	if exited.level != slog.LevelWarn || exited.attrs["error_code"] != "observe_agent_exited" || exited.attrs["exit_code"] != int64(3) {
		t.Fatalf("exit record = %+v, want a warning coded observe_agent_exited with exit_code 3", exited)
	}

	h.stop(t)
	if got := h.exits.Load(); got != 1 {
		t.Fatalf("OnExit ran %d times, want 1", got)
	}
}

func TestSupervisorNextChildGetsANewToken(t *testing.T) {
	h := newHarness(t, "serve-then-exit")
	h.s.sleep = func(context.Context, time.Duration) {}
	h.start()

	first, _ := h.rec.next(t, "child started").attrs["token"].(string)
	second, _ := h.rec.next(t, "child started").attrs["token"].(string)
	h.stop(t)

	if first == "" || second == "" || first == second {
		t.Fatalf("tokens = %q and %q, want two different non-empty tokens", first, second)
	}
}

func TestSupervisorBackoff(t *testing.T) {
	h := newHarness(t, "exit")
	var slept []time.Duration
	h.s.sleep = func(ctx context.Context, d time.Duration) {
		slept = append(slept, d)
		if len(slept) == 9 {
			h.cancel()
		}
	}
	clock := &scriptedClock{ran: []time.Duration{0, 0, 0, 0, 0, 0, 0, 61 * time.Second, 0}}
	h.s.now = clock.now
	h.start()
	h.wait(t)

	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second, time.Second, 2 * time.Second}
	if !slices.Equal(slept, want) {
		t.Fatalf("backoff = %v, want %v", slept, want)
	}
}

type scriptedClock struct {
	mu    sync.Mutex
	calls int
	ran   []time.Duration
}

func (c *scriptedClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	base := time.Unix(1_700_000_000, 0)
	if c.calls%2 == 1 {
		return base
	}
	return base.Add(c.ran[(c.calls/2-1)%len(c.ran)])
}

func TestSupervisorStartFailureIsCodedAndBacksOff(t *testing.T) {
	h := newHarness(t, "exit")
	h.s.cfg.Path = filepath.Join(t.TempDir(), "missing", "observe-agent")
	var slept []time.Duration
	h.s.sleep = func(ctx context.Context, d time.Duration) {
		slept = append(slept, d)
		if len(slept) == 2 {
			h.cancel()
		}
	}
	h.start()
	h.wait(t)

	failed := h.rec.next(t, "observe-agent could not be started")
	if failed.level != slog.LevelError || failed.attrs["error_code"] != "observe_agent_start_failed" {
		t.Fatalf("record = %+v, want an error coded observe_agent_start_failed", failed)
	}
	if want := []time.Duration{time.Second, 2 * time.Second}; !slices.Equal(slept, want) {
		t.Fatalf("backoff = %v, want %v", slept, want)
	}
	if got := h.exits.Load(); got != 0 {
		t.Fatalf("OnExit ran %d times for a child that never started, want 0", got)
	}
	if h.s.Authenticate("") {
		t.Fatal("Authenticate accepted an empty token with no child running")
	}
}

func TestSupervisorTerminatesTheChildWhenContextEnds(t *testing.T) {
	h := newHarness(t, "serve")
	h.start()
	h.rec.next(t, "child started")

	h.stop(t)

	h.rec.next(t, "terminated")
	exited := h.rec.next(t, "observe-agent exited")
	if exited.level != slog.LevelInfo || exited.attrs["exit_code"] != int64(0) {
		t.Fatalf("exit record = %+v, want an info record with exit_code 0", exited)
	}
	if got := h.exits.Load(); got != 1 {
		t.Fatalf("OnExit ran %d times, want 1", got)
	}
}

func TestSupervisorKillsAChildThatIgnoresTerminate(t *testing.T) {
	h := newHarness(t, "ignore-term")
	h.s.grace = 200 * time.Millisecond
	h.start()
	h.rec.next(t, "child started")

	h.stop(t)

	exited := h.rec.next(t, "observe-agent exited")
	if exited.attrs["exit_code"] != int64(-1) {
		t.Fatalf("exit record = %+v, want exit_code -1 for a killed child", exited)
	}
	if got := h.exits.Load(); got != 1 {
		t.Fatalf("OnExit ran %d times, want 1", got)
	}
}

func TestSupervisorRelaysTheChildsLogs(t *testing.T) {
	h := newHarness(t, "log")
	h.start()

	seen := h.rec.collect(t, "hello", "observe-agent output")
	hello := seen["hello"]
	if hello.level != slog.LevelWarn || hello.attrs["k"] != "v" || hello.attrs["component"] != "observe-agent" {
		t.Fatalf("relayed record = %+v, want a warning with k=v and component=observe-agent", hello)
	}
	oops := seen["observe-agent output"]
	if oops.attrs["line"] != "oops" || oops.attrs["component"] != "observe-agent" {
		t.Fatalf("stderr record = %+v, want line=oops and component=observe-agent", oops)
	}
	h.stop(t)
}

func TestSupervisorNeverLogsTheToken(t *testing.T) {
	h := newHarness(t, "serve-then-exit")
	h.s.sleep = func(context.Context, time.Duration) {}
	h.start()
	token, _ := h.rec.next(t, "child started").attrs["token"].(string)
	h.rec.next(t, "observe-agent exited")
	h.stop(t)

	for _, rec := range h.rec.all() {
		if rec.msg == "child started" {
			continue
		}
		if strings.Contains(fmt.Sprint(rec.attrs), token) {
			t.Fatalf("supervisor record %q leaks the token: %+v", rec.msg, rec.attrs)
		}
	}
}

func TestFindBinary(t *testing.T) {
	t.Run("found on PATH", func(t *testing.T) {
		dir := t.TempDir()
		want := filepath.Join(dir, "observe-agent")
		if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // a stand-in executable must be runnable
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		got, err := FindBinary()
		if err != nil || got != want {
			t.Fatalf("FindBinary = %q, %v, want %q", got, err, want)
		}
	})
	t.Run("not on PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := FindBinary()
		if err == nil || !strings.Contains(err.Error(), "observe-agent is not on PATH") || !strings.Contains(err.Error(), "make agent") {
			t.Fatalf("FindBinary error = %v, want the not-on-PATH message", err)
		}
	})
}
