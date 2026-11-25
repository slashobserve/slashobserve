package egress

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestCallTripperNeverDialsAnAddressThatIsNotPublic(t *testing.T) {
	up, _ := newUpstream(t, newAllow(t, "feed.test"), mcpLikeMux())
	loopback := up.server.Listener.Addr().String()
	c := newClient(newAllow(t, "feed.test"), Options{}, dialer(true, loopback), dialer(false, loopback), dialer(true, loopback), nil)
	client := &http.Client{Transport: c.CallTripper()}
	_, err := client.Post("https://feed.test/mcp", "application/json", strings.NewReader(`{}`))
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("error = %v, want a blocked address", err)
	}
	if up.hits.Load() != 0 {
		t.Errorf("the loopback server saw %d requests", up.hits.Load())
	}
}

func TestANewClientGuardsTheDialerBehindCallTripper(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	c := New(newAllow(t, "feed.test"), Options{})
	conn, err := c.calls.DialContext(t.Context(), "tcp", listener.Addr().String())
	if err == nil {
		_ = conn.Close()
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("dial = %v, want a blocked address", err)
	}
}

func privateClient(t *testing.T) (*Client, func() int64) {
	t.Helper()
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		mcpLikeMux().ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	loopback := server.Listener.Addr().String()
	c := newClient(newAllow(t, "feed.test"), Options{UserAgent: "TestAgent/1"}, dialer(true, loopback), dialer(false, loopback), dialer(true, loopback), nil)
	t.Cleanup(c.privateCalls.CloseIdleConnections)
	return c, hits.Load
}

func TestPrivateCallTripperReachesAPrivateAddressOverHTTPAndTheOtherTripperDoesNot(t *testing.T) {
	c, hits := privateClient(t)
	private := &http.Client{Transport: c.PrivateCallTripper()}
	resp, err := private.Post("http://feed.test:8080/mcp", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `POST {"a":1}` || hits() != 1 {
		t.Fatalf("body %q, hits %d", body, hits())
	}
	public := &http.Client{Transport: c.CallTripper()}
	blocked, err := public.Post("http://feed.test:8080/mcp", "application/json", strings.NewReader(`{}`))
	if err == nil {
		_ = blocked.Body.Close()
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("the guarded tripper: %v, want a blocked address", err)
	}
	if hits() != 1 {
		t.Errorf("the guarded tripper reached the server: %d hits", hits())
	}
}

func TestPrivateCallTripperStillHoldsTheAllowlistAndTheDomainRule(t *testing.T) {
	c, hits := privateClient(t)
	client := &http.Client{Transport: c.PrivateCallTripper()}
	resp, err := client.Post("http://other.test:8080/mcp", "application/json", strings.NewReader(`{}`))
	if err == nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a host off the list: %v", err)
	}
	for _, address := range []string{"http://127.0.0.1:8080/mcp", "http://localhost:8080/mcp", "http://user:pw@feed.test/mcp", "ftp://feed.test/mcp"} {
		resp, err := client.Post(address, "application/json", strings.NewReader(`{}`))
		if err == nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%s: %v, want an invalid url", address, err)
		}
	}
	if hits() != 0 {
		t.Errorf("the server saw %d requests", hits())
	}
}
