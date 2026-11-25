package egress

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func mcpLikeMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(r.Method + " " + string(body)))
	})
	return mux
}

func TestCallTripperPostsToAnAllowedHostWithTheClientsUserAgent(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test"), mcpLikeMux())
	client := &http.Client{Transport: c.CallTripper()}
	resp, err := client.Post("https://feed.test/mcp", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `POST {"a":1}` {
		t.Errorf("body = %q", body)
	}
	if got := up.last().UserAgent; got != "TestAgent/1" {
		t.Errorf("user agent = %q", got)
	}
}

func TestCallTripperRefusesAHostOffTheListBeforeAnyRequest(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test"), mcpLikeMux())
	client := &http.Client{Transport: c.CallTripper()}
	_, err := client.Post("https://other.test/mcp", "application/json", strings.NewReader(`{}`))
	var hostErr *HostError
	if !errors.As(err, &hostErr) || !errors.Is(err, ErrNotAllowed) || hostErr.Host != "other.test" {
		t.Fatalf("error = %v, want other.test not allowed", err)
	}
	if up.hits.Load() != 0 {
		t.Errorf("upstream saw %d requests", up.hits.Load())
	}
}

func TestCallTripperReadsTheListOnEveryRequest(t *testing.T) {
	allow := newAllow(t, "feed.test")
	_, c := newUpstream(t, allow, mcpLikeMux())
	client := &http.Client{Transport: c.CallTripper()}
	do := func() error {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://feed.test/mcp", strings.NewReader(`{}`))
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		return err
	}
	if err := do(); err != nil {
		t.Fatal(err)
	}
	allow.set(t, "other.test")
	if err := do(); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("after removal: %v, want not allowed", err)
	}
}

func TestCallTripperRefusesAnAddressWithoutADomainName(t *testing.T) {
	_, c := newUpstream(t, newAllow(t, "feed.test"), mcpLikeMux())
	client := &http.Client{Transport: c.CallTripper()}
	for _, address := range []string{"https://127.0.0.1/mcp", "https://localhost/mcp", "https://user:pw@feed.test/mcp", "ftp://feed.test/mcp"} {
		if _, err := client.Post(address, "application/json", strings.NewReader(`{}`)); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%s: error = %v, want an invalid url", address, err)
		}
	}
}
