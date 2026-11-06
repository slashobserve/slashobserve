package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const secretToken = "sok_0123456789abcdef0123456789abcdef012345"

type fakeDex struct {
	mu      sync.Mutex
	order   []string
	landing string
	token   string
	saCode  int
	origins []string
}

func (f *fakeDex) handler() http.Handler {
	mux := http.NewServeMux()
	record := func(step string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.order = append(f.order, step)
	}
	mux.HandleFunc("GET /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		record("authorize " + r.URL.Query().Get("return_to"))
		http.Redirect(w, r, "/dex/auth?client_id=so", http.StatusFound)
	})
	mux.HandleFunc("GET /dex/auth", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dex/auth/local/login?state=abc", http.StatusFound)
	})
	mux.HandleFunc("GET /dex/auth/local/login", func(w http.ResponseWriter, _ *http.Request) {
		record("form")
		_, _ = io.WriteString(w, `<form method="post"><input name="login"><input name="password"></form>`)
	})
	mux.HandleFunc("POST /dex/auth/local/login", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		record("credentials " + r.PostForm.Get("login") + ":" + r.PostForm.Get("password"))
		http.Redirect(w, r, "/api/v1/auth/callback?code=c&state=abc", http.StatusFound)
	})
	mux.HandleFunc("GET /api/v1/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "so_session", Value: "session", Path: "/", HttpOnly: true}) //nolint:gosec // a Secure cookie would not be sent back over the plain-HTTP test server
		http.Redirect(w, r, f.landing, http.StatusFound)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "app") })
	mux.HandleFunc("GET /elsewhere", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "app") })
	signedIn := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		f.origins = append(f.origins, r.Header.Get("Origin"))
		f.mu.Unlock()
		if _, err := r.Cookie("so_session"); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /api/v1/service-accounts", func(w http.ResponseWriter, r *http.Request) {
		if !signedIn(w, r) {
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		record("service account " + body.Name)
		if f.saCode != 0 {
			w.WriteHeader(f.saCode)
			_, _ = io.WriteString(w, `{"error":"name taken","code":"conflict"}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"sa-1","name":"`+body.Name+`"}`)
	})
	mux.HandleFunc("POST /api/v1/service-accounts/sa-1/keys", func(w http.ResponseWriter, r *http.Request) {
		if !signedIn(w, r) {
			return
		}
		record("key")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"key":{"id":"k-1"},"token":"`+f.token+`"}`)
	})
	return mux
}

func startDex(t *testing.T) (*fakeDex, *httptest.Server) {
	t.Helper()
	f := &fakeDex{landing: "/", token: secretToken}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func TestBootstrapSignsInThroughDexAndIssuesAKey(t *testing.T) {
	f, srv := startDex(t)
	token, err := bootstrapKey(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if token != secretToken {
		t.Fatalf("token %q, want the issued key", token)
	}
	steps := strings.Join(f.order, "\n")
	for _, want := range []string{"authorize /", "form", "credentials admin@example.com:password", "service account evals-", "key"} {
		if !strings.Contains(steps, want) {
			t.Errorf("steps %q miss %q", steps, want)
		}
	}
	for _, origin := range f.origins {
		if origin != srv.URL {
			t.Errorf("origin %q, want %q on a cookie-authenticated mutation", origin, srv.URL)
		}
	}
}

func TestBootstrapRefusesHostsThatAreNotLoopback(t *testing.T) {
	cases := []string{
		"http://example.com",
		"http://localhost.evil.com:8080",
		"http://127.0.0.1.evil.com",
		"http://10.0.0.5:8080",
		"http://[::1]:8080",
		"localhost:8080",
		"",
	}
	for _, server := range cases {
		t.Run(server, func(t *testing.T) {
			_, err := bootstrapKey(context.Background(), server)
			if err == nil || !strings.Contains(err.Error(), "--bootstrap-dex") {
				t.Fatalf("error %v, want a --bootstrap-dex refusal", err)
			}
		})
	}
}

func TestBootstrapAcceptsLocalhostAndLoopbackAddress(t *testing.T) {
	for _, server := range []string{"http://localhost:28080", "http://127.0.0.1:28080/", "https://localhost"} {
		if _, err := checkLoopback(server); err != nil {
			t.Errorf("%s: %v", server, err)
		}
	}
}

func TestBootstrapFailures(t *testing.T) {
	cases := map[string]struct {
		setup func(f *fakeDex)
		want  string
	}{
		"login lands somewhere else": {
			setup: func(f *fakeDex) { f.landing = "/elsewhere" },
			want:  "landed on",
		},
		"service account refused": {
			setup: func(f *fakeDex) { f.saCode = http.StatusConflict },
			want:  "http 409 conflict",
		},
		"token has the wrong shape": {
			setup: func(f *fakeDex) { f.token = "hunter2-not-a-key" },
			want:  "no token",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, srv := startDex(t)
			tc.setup(f)
			token, err := bootstrapKey(context.Background(), srv.URL)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("token %q, error %v, want %q", token, err, tc.want)
			}
			if token != "" {
				t.Fatalf("a failed bootstrap returned the token %q", token)
			}
			if strings.Contains(err.Error(), f.token) {
				t.Fatalf("the error %q holds the issued token", err)
			}
		})
	}
}

func TestBootstrapStopsWhenTheFormIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "no form here")
	}))
	t.Cleanup(srv.Close)
	_, err := bootstrapKey(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "login form was not reached") {
		t.Fatalf("error %v, want the missing form", err)
	}
}

func TestAPIErrorsNeverCarryTheToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		echoed := map[string]string{"error": "bad credentials " + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "code": "forbidden"}
		_ = json.NewEncoder(w).Encode(echoed)
	}))
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, secretToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.agent(context.Background())
	if err == nil || strings.Contains(err.Error(), secretToken) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error %v must name the failure and hide the token", err)
	}
}
