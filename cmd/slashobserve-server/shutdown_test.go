package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/internal/server"
)

func TestServeIsReadyUntilTheSignalAndStoppingWhileItDrains(t *testing.T) {
	t.Parallel()
	logger, logs := logtest.New()
	phases := server.StartPhases(logger)
	phases.Enter(server.PhaseMigrating)
	phases.Enter(server.PhaseProvisioning)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, signal := context.WithCancel(context.Background())
	defer signal()
	reportPhase := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, string(phases.Current())) })
	served := make(chan error, 1)
	go func() { served <- serve(ctx, logger, phases, ln, reportPhase) }()

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	phaseServed := func() string {
		t.Helper()
		resp, err := client.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	if got := phaseServed(); got != "ready" {
		t.Fatalf("a serving server reports %q, want ready", got)
	}

	signal()
	draining := phaseServed()
	for deadline := time.Now().Add(2 * time.Second); draining != "stopping" && time.Now().Before(deadline); draining = phaseServed() {
		time.Sleep(10 * time.Millisecond)
	}
	if draining != "stopping" {
		t.Fatalf("a draining server reports %q, want stopping", draining)
	}
	if err := <-served; err != nil {
		t.Fatalf("serve: %v", err)
	}
	if got := logs.Find(t, "listening"); got["phase"] != "ready" || got["addr"] != ln.Addr().String() {
		t.Errorf("listening line = %v", got)
	}
	if got := logs.Find(t, "shutting down"); got["phase"] != "stopping" {
		t.Errorf("shutting down line = %v", got)
	}
}

func TestShutdownEndsAStreamBeforeItsDeadlineSoItCanFinish(t *testing.T) {
	requests, cutRequests := context.WithCancel(context.Background())
	defer cutRequests()
	opened, finished := make(chan struct{}), make(chan struct{})
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(opened)
			<-r.Context().Done()
			close(finished)
		}),
		BaseContext:       func(net.Listener) context.Context { return requests },
		ReadHeaderTimeout: time.Second,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	stream, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	if _, err := stream.Write([]byte("GET / HTTP/1.1\r\nHost: stream\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	<-opened
	start := time.Now()
	if err := shutdown(srv, cutRequests, 0, 50*time.Millisecond, 5*time.Second); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned while the stream's handler was still running")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("shutdown took %s with one open stream", took)
	}
}

func TestShutdownKeepsAnsweringRequestsDuringTheDrainAndThenStops(t *testing.T) {
	requests, cutRequests := context.WithCancel(context.Background())
	defer cutRequests()
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		BaseContext:       func(net.Listener) context.Context { return requests },
		ReadHeaderTimeout: time.Second,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	target := "http://" + ln.Addr().String() + "/"

	const drain = 300 * time.Millisecond
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- shutdown(srv, cutRequests, drain, 5*time.Second, 10*time.Second) }()

	time.Sleep(drain / 3)
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get(target)
	if err != nil {
		t.Fatalf("request during the drain: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("request during the drain: status %d", resp.StatusCode)
	}

	if err := <-done; err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if took := time.Since(start); took < drain {
		t.Fatalf("shutdown returned after %s, before the %s drain", took, drain)
	}
	if resp, err := client.Get(target); err == nil {
		_ = resp.Body.Close()
		t.Fatal("the listener still accepted a request after shutdown")
	}
}
