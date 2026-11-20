package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

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
	streams := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err == nil {
			streams <- resp
		}
	}()
	<-opened
	defer func() {
		select {
		case resp := <-streams:
			_ = resp.Body.Close()
		case <-time.After(time.Second):
		}
	}()
	start := time.Now()
	if err := shutdown(srv, cutRequests, 50*time.Millisecond, 5*time.Second); err != nil {
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
