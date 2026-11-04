package llm_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/llm"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func watchedClient(w *llm.IdleWatch) *http.Client {
	return &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		return w.Middleware(r, http.DefaultTransport.RoundTrip)
	})}
}

func TestIdleWatchFiresWithoutBytes(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"before the headers": func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
		"after the headers": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		},
	}
	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			t.Cleanup(srv.Close)
			watch := llm.NewIdleWatch(100 * time.Millisecond)
			start := time.Now()
			resp, err := watchedClient(watch).Get(srv.URL)
			if err == nil {
				_, err = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}
			if err == nil || !watch.Fired() {
				t.Fatalf("err = %v, fired = %v, want a failure from the watch", err, watch.Fired())
			}
			if took := time.Since(start); took > 2*time.Second {
				t.Fatalf("took %s with a 100ms idle timeout", took)
			}
			failure := watch.Failure(context.Background(), context.Background(), "anthropic")
			var callErr *llm.CallError
			if !errors.As(failure, &callErr) || !callErr.Idle || callErr.Failure != llm.CallTimeout {
				t.Fatalf("Failure = %v, want an idle call_timeout", failure)
			}
		})
	}
}

func TestIdleWatchResetsOnEveryRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for range 8 {
			_, _ = io.WriteString(w, "x")
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)
	watch := llm.NewIdleWatch(200 * time.Millisecond)
	resp, err := watchedClient(watch).Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) != 8 || watch.Fired() {
		t.Fatalf("read %d bytes, err %v, fired %v, want all 8 bytes and no firing", len(body), err, watch.Fired())
	}
	if failure := watch.Failure(context.Background(), context.Background(), "openai"); failure != nil {
		t.Fatalf("Failure = %v, want nil", failure)
	}
}

func TestIdleWatchOffAtZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, "late")
	}))
	t.Cleanup(srv.Close)
	watch := llm.NewIdleWatch(0)
	resp, err := watchedClient(watch).Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if body, _ := io.ReadAll(resp.Body); string(body) != "late" || watch.Fired() {
		t.Fatalf("body %q, fired %v", body, watch.Fired())
	}
}

func TestIdleWatchReportsNothingWhenTheCallerCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	t.Cleanup(srv.Close)
	watch := llm.NewIdleWatch(100 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := watchedClient(watch).Do(req); err == nil {
		_ = resp.Body.Close()
	}
	if !watch.Fired() {
		t.Fatal("the watch did not fire")
	}
	cancel()
	if failure := watch.Failure(ctx, context.Background(), "openai"); failure != nil {
		t.Fatalf("Failure = %v, want nil once the caller canceled", failure)
	}
}
