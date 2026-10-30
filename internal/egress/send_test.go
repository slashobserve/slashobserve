package egress

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type received struct {
	Method, Path, ContentType, Custom, Body string
}

type recorder struct {
	mu   sync.Mutex
	reqs []received
}

func (r *recorder) record(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, received{
		Method: req.Method, Path: req.URL.RequestURI(), ContentType: req.Header.Get("Content-Type"),
		Custom: req.Header.Get("X-Custom"), Body: string(body),
	})
}

func (r *recorder) all() []received {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]received(nil), r.reqs...)
}

func newSendMux(rec *recorder) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		http.Redirect(w, r, "https://other.test/target", http.StatusFound)
	})
	mux.HandleFunc("/target", func(_ http.ResponseWriter, r *http.Request) { rec.record(r) })
	mux.HandleFunc("/endless", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		chunk := []byte(strings.Repeat("x", 32<<10))
		for {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	})
	mux.HandleFunc("/abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	return mux
}

func sendCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSendDeliversMethodHeadersAndBodyToAnAllowedHost(t *testing.T) {
	rec := &recorder{}
	up, c := newUpstream(t, newAllow(t, "feed.test"), newSendMux(rec))
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set("X-Custom", "yes")
	header.Set("Authorization", "Bearer leaked")
	header.Set("Proxy-Authorization", "Basic leaked")
	header.Set("Cookie", "session=leaked")
	header.Set("Referer", "https://elsewhere.test/")

	got, err := c.Send(sendCtx(t), SendRequest{Method: http.MethodDelete, URL: "https://feed.test/run?x=1", Header: header, Body: []byte(`{"a":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", got.Status)
	}
	want := received{Method: "DELETE", Path: "/run?x=1", ContentType: "application/json", Custom: "yes", Body: `{"a":1}`}
	if reqs := rec.all(); len(reqs) != 1 || reqs[0] != want {
		t.Errorf("upstream saw %+v, want %+v", reqs, want)
	}
	seen := up.last()
	if seen.UserAgent != "TestAgent/1" {
		t.Errorf("User-Agent = %q", seen.UserAgent)
	}
	if seen.Authorization != "" || seen.Cookie != "" || seen.Referer != "" {
		t.Errorf("withheld headers were sent: %+v", seen)
	}
}

func TestSendRefusesWithoutTouchingTheNetwork(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		body    []byte
		allow   []string
		wantErr error
	}{
		{"unlisted host", "https://nope.test/run", nil, []string{"feed.test"}, ErrNotAllowed},
		{"empty allowlist", "https://feed.test/run", nil, nil, ErrNotAllowed},
		{"ip literal", "https://127.0.0.1/run", nil, []string{"feed.test"}, ErrInvalidURL},
		{"plain http", "http://feed.test/run", nil, []string{"feed.test"}, ErrInvalidURL},
		{"user info", "https://user:pw@feed.test/run", nil, []string{"feed.test"}, ErrInvalidURL},
		{"other port", "https://feed.test:8443/run", nil, []string{"feed.test"}, ErrInvalidURL},
		{"unparsable", "https://%zz/run", nil, []string{"feed.test"}, ErrInvalidURL},
		{"body over 64 KiB", "https://feed.test/run", []byte(strings.Repeat("a", 64<<10+1)), []string{"feed.test"}, ErrTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			up, c := newUpstream(t, newAllow(t, tc.allow...), newSendMux(rec))
			_, err := c.Send(sendCtx(t), SendRequest{Method: http.MethodPost, URL: tc.url, Body: tc.body})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if up.hits.Load() != 0 {
				t.Errorf("upstream saw %d requests", up.hits.Load())
			}
		})
	}
}

func TestSendNamesTheHostInEveryRefusal(t *testing.T) {
	_, c := newUpstream(t, newAllow(t, "feed.test"), newSendMux(&recorder{}))
	_, err := c.Send(sendCtx(t), SendRequest{Method: http.MethodPost, URL: "https://nope.test/run"})
	var hostErr *HostError
	if !errors.As(err, &hostErr) || hostErr.Host != "nope.test" {
		t.Fatalf("error = %#v, want a HostError for nope.test", err)
	}
}

func TestSendReturnsARedirectAsItsStatusWithoutFollowingIt(t *testing.T) {
	rec := &recorder{}
	_, c := newUpstream(t, newAllow(t, "feed.test", "other.test"), newSendMux(rec))
	got, err := c.Send(sendCtx(t), SendRequest{Method: http.MethodPost, URL: "https://feed.test/redirect", Body: []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != http.StatusFound {
		t.Errorf("status = %d, want 302", got.Status)
	}
	if reqs := rec.all(); len(reqs) != 1 || reqs[0].Path != "/redirect" {
		t.Errorf("upstream saw %+v, want only /redirect", reqs)
	}
}

func TestSendReturnsARemoteErrorStatusWithoutAnError(t *testing.T) {
	_, c := newUpstream(t, newAllow(t, "feed.test"), newSendMux(&recorder{}))
	got, err := c.Send(sendCtx(t), SendRequest{Method: http.MethodPost, URL: "https://feed.test/fail"})
	if err != nil || got.Status != http.StatusInternalServerError {
		t.Fatalf("got %+v, %v; want status 500 and no error", got, err)
	}
}

func TestSendStopsReadingAnEndlessResponseAtTheLimit(t *testing.T) {
	_, c := newUpstream(t, newAllow(t, "feed.test"), newSendMux(&recorder{}))
	got, err := c.Send(sendCtx(t), SendRequest{Method: http.MethodPost, URL: "https://feed.test/endless"})
	if err != nil || got.Status != http.StatusOK {
		t.Fatalf("got %+v, %v; want status 200 and no error", got, err)
	}
}

func TestSendNeverCachesOrDeduplicates(t *testing.T) {
	rec := &recorder{}
	up, c := newUpstream(t, newAllow(t, "feed.test"), newSendMux(rec))
	for range 2 {
		if _, err := c.Send(sendCtx(t), SendRequest{Method: http.MethodPost, URL: "https://feed.test/run", Body: []byte("same")}); err != nil {
			t.Fatal(err)
		}
	}
	if up.hits.Load() != 2 {
		t.Errorf("upstream saw %d requests, want 2", up.hits.Load())
	}
}

func TestSendReportsAConnectionThatDropsAsUnavailable(t *testing.T) {
	_, c := newUpstream(t, newAllow(t, "feed.test"), newSendMux(&recorder{}))
	_, err := c.Send(sendCtx(t), SendRequest{Method: http.MethodPost, URL: "https://feed.test/abort"})
	var hostErr *HostError
	if !errors.Is(err, ErrUnavailable) || !errors.As(err, &hostErr) || hostErr.Host != "feed.test" {
		t.Fatalf("error = %#v, want ErrUnavailable for feed.test", err)
	}
}
