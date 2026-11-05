package egress

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

var jsonTypes = []string{"application/json"}

func newJSONMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"database":"ok"}`))
	})
	mux.HandleFunc("/api/big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat("a", int(2<<20))))
	})
	mux.HandleFunc("/api/huge", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat("a", int(hardMaxBytes+1))))
	})
	mux.HandleFunc("/api/denied", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	})
	mux.HandleFunc("/api/same-host", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/health", http.StatusFound)
	})
	mux.HandleFunc("/api/other-host", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://other.test/api/health", http.StatusFound)
	})
	return mux
}

func authorized(url string) AuthorizedRequest {
	return AuthorizedRequest{Request: Request{URL: url, Accept: jsonTypes}, Authorization: "Bearer glsa_secret"}
}

func TestFetchAuthorizedSendsTheHeaderAndNeverCaches(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test"), newJSONMux())
	for i := range 2 {
		body, err := c.FetchAuthorized(context.Background(), authorized(fetchURL("feed.test", "/api/health")))
		if err != nil || string(body) != `{"database":"ok"}` {
			t.Fatalf("fetch %d: body %q, err %v", i, body, err)
		}
	}
	if up.hits.Load() != 2 {
		t.Errorf("upstream saw %d requests, want 2 because authorized fetches are never cached", up.hits.Load())
	}
	if got := up.last(); got.Authorization != "Bearer glsa_secret" || got.UserAgent != "TestAgent/1" || got.Accept != "application/json" {
		t.Errorf("request headers = %+v", got)
	}

	if _, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", "/api/health"), Accept: jsonTypes}); err != nil {
		t.Fatal(err)
	}
	if got := up.last(); got.Authorization != "" {
		t.Errorf("the shared fetch sent Authorization %q", got.Authorization)
	}
	if up.hits.Load() != 3 {
		t.Errorf("the shared fetch reused an authorized response: %d requests, want 3", up.hits.Load())
	}
}

func TestFetchAuthorizedRefusesEveryRedirect(t *testing.T) {
	for _, path := range []string{"/api/same-host", "/api/other-host"} {
		t.Run(path, func(t *testing.T) {
			up, c := newUpstream(t, newAllow(t, "feed.test", "other.test"), newJSONMux())
			_, err := c.FetchAuthorized(context.Background(), authorized(fetchURL("feed.test", path)))
			var hostErr *HostError
			if !errors.Is(err, ErrRedirected) || !errors.As(err, &hostErr) || hostErr.Host != "feed.test" {
				t.Fatalf("error = %#v, want a refused redirect from feed.test", err)
			}
			if up.hits.Load() != 1 {
				t.Errorf("upstream saw %d requests, want only the first", up.hits.Load())
			}
		})
	}
}

func TestFetchAuthorizedReportsTheStatusAndDoesNotCacheFailures(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test"), newJSONMux())
	for range 2 {
		_, err := c.FetchAuthorized(context.Background(), authorized(fetchURL("feed.test", "/api/denied")))
		var status *StatusError
		if !errors.As(err, &status) || status.Status != http.StatusUnauthorized {
			t.Fatalf("error = %#v, want status 401", err)
		}
	}
	if up.hits.Load() != 2 {
		t.Errorf("upstream saw %d requests, want 2", up.hits.Load())
	}
}

func TestFetchAuthorizedSizeCap(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		maxBytes int64
		wantErr  error
	}{
		{"default cap refuses 2 MiB", "/api/big", 0, ErrTooLarge},
		{"raised cap accepts 2 MiB", "/api/big", 4 << 20, nil},
		{"cap above 8 MiB is clamped", "/api/huge", 64 << 20, ErrTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, c := newUpstream(t, newAllow(t, "feed.test"), newJSONMux())
			req := authorized(fetchURL("feed.test", tc.path))
			req.MaxBytes = tc.maxBytes
			body, err := c.FetchAuthorized(context.Background(), req)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && len(body) != 2<<20 {
				t.Errorf("body is %d bytes", len(body))
			}
		})
	}
}

func TestFetchAuthorizedURLRules(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		private bool
		wantErr error
	}{
		{"default refuses plain http", "http://feed.test/api/health", false, ErrInvalidURL},
		{"default refuses another port", "https://feed.test:3000/api/health", false, ErrInvalidURL},
		{"private accepts another port", "https://feed.test:3000/api/health", true, nil},
		{"private refuses user info", "https://u:p@feed.test/api/health", true, ErrInvalidURL},
		{"private refuses other schemes", "ftp://feed.test/api/health", true, ErrInvalidURL},
		{"private refuses an ip literal", "https://127.0.0.1/api/health", true, ErrInvalidURL},
		{"private still needs the allowlist", "https://nope.test/api/health", true, ErrNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up, c := newUpstream(t, newAllow(t, "feed.test"), newJSONMux())
			req := authorized(tc.url)
			req.AllowPrivate = tc.private
			_, err := c.FetchAuthorized(context.Background(), req)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && up.hits.Load() != 0 {
				t.Errorf("upstream saw %d requests", up.hits.Load())
			}
		})
	}
}

func TestOnlyAllowPrivateReachesALoopbackAddress(t *testing.T) {
	cert, pool := testCertificate(t)
	var hits atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		newJSONMux().ServeHTTP(w, r)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	t.Cleanup(server.Close)
	c := NewDialingTo(newAllow(t, "feed.test"), Options{}, server.Listener.Addr().String(), pool)
	t.Cleanup(c.private.CloseIdleConnections)

	_, err := c.FetchAuthorized(context.Background(), authorized(fetchURL("feed.test", "/api/health")))
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("default authorized fetch error = %v, want ErrBlockedAddress", err)
	}
	if _, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", "/api/health"), Accept: jsonTypes}); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("shared fetch error = %v, want ErrBlockedAddress", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the loopback server saw %d requests before AllowPrivate", hits.Load())
	}

	req := authorized(fetchURL("feed.test", "/api/health"))
	req.AllowPrivate = true
	if body, err := c.FetchAuthorized(context.Background(), req); err != nil || string(body) != `{"database":"ok"}` {
		t.Fatalf("private fetch: body %q, err %v", body, err)
	}
}

func TestAllowPrivateSpeaksPlainHTTP(t *testing.T) {
	var auth atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		newJSONMux().ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	c := NewDialingTo(newAllow(t, "grafana.internal.test"), Options{}, server.Listener.Addr().String(), nil)
	t.Cleanup(c.private.CloseIdleConnections)

	req := authorized("http://grafana.internal.test:3000/api/health")
	req.AllowPrivate = true
	if _, err := c.FetchAuthorized(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got, _ := auth.Load().(string); got != "Bearer glsa_secret" {
		t.Errorf("Authorization = %q", got)
	}
}
