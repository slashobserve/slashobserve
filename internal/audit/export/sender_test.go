package export

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type received struct {
	mu      sync.Mutex
	method  string
	path    string
	headers http.Header
	body    string
}

func receiver(t *testing.T, handler func(w http.ResponseWriter)) (*httptest.Server, *received) {
	t.Helper()
	got := &received{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.mu.Lock()
		got.method, got.path, got.headers, got.body = r.Method, r.URL.Path, r.Header.Clone(), string(body)
		got.mu.Unlock()
		handler(w)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestSendPostsOTLPJSONToTheLogsPathWithTheConfiguredHeaders(t *testing.T) {
	srv, got := receiver(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusOK) })
	sender := NewSender(mustParse(t, srv.URL+"/collector/"), map[string]datasource.Secret{"Authorization": datasource.NewSecret("Bearer s3cret")})
	if err := sender.Send(t.Context(), []byte(`{"resourceLogs":[]}`)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/collector/v1/logs" {
		t.Errorf("request = %s %s", got.method, got.path)
	}
	if got.headers.Get("Content-Type") != "application/json" || got.headers.Get("Authorization") != "Bearer s3cret" {
		t.Errorf("headers = %v", got.headers)
	}
	if got.body != `{"resourceLogs":[]}` {
		t.Errorf("body = %q", got.body)
	}
}

func TestSendAcceptsAnyTwoXX(t *testing.T) {
	srv, _ := receiver(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusAccepted) })
	if err := NewSender(mustParse(t, srv.URL), nil).Send(t.Context(), []byte("{}")); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestSendReportsOtherStatusesAndRetryAfter(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		retryAfter string
		want       time.Duration
	}{
		{"server error", http.StatusInternalServerError, "", 0},
		{"throttled", http.StatusTooManyRequests, "7", 7 * time.Second},
		{"retry after as a date is ignored", http.StatusServiceUnavailable, "Wed, 21 Nov 2029 07:28:00 GMT", 0},
		{"rejected", http.StatusBadRequest, "", 0},
		{"redirect is not followed", http.StatusFound, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := receiver(t, func(w http.ResponseWriter) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				if tt.status == http.StatusFound {
					w.Header().Set("Location", "http://127.0.0.1:1/elsewhere")
				}
				w.WriteHeader(tt.status)
			})
			err := NewSender(mustParse(t, srv.URL), nil).Send(t.Context(), []byte("{}"))
			var status *StatusError
			if !errors.As(err, &status) {
				t.Fatalf("Send error = %v, want a StatusError", err)
			}
			if status.Status != tt.status || status.RetryAfter != tt.want {
				t.Errorf("status %d retry after %s, want %d and %s", status.Status, status.RetryAfter, tt.status, tt.want)
			}
		})
	}
}

func TestSendFailsWhenNothingListens(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	endpoint := mustParse(t, srv.URL)
	srv.Close()
	if err := NewSender(endpoint, nil).Send(t.Context(), []byte("{}")); err == nil {
		t.Fatal("Send to a closed server succeeded")
	}
}
