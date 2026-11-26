package contextsources

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/egress/egresstest"
)

const fakeHost = "tools.example.com"

type staticList struct{ list domains.List }

func (s staticList) Allowed(context.Context) (domains.List, error) { return s.list, nil }

func listOf(t *testing.T, hosts ...string) staticList {
	t.Helper()
	var list domains.List
	for _, h := range hosts {
		p, err := domains.ParsePattern(h)
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, p)
	}
	return staticList{list: list}
}

type fakeMCP struct {
	mu      sync.Mutex
	secret  string
	tools   []mcp.Tool
	headers []string
	inner   http.Handler
	answers map[string]server.ToolHandlerFunc
}

func (f *fakeMCP) rebuild() {
	s := server.NewMCPServer("fake", "1.0.0")
	for _, tool := range f.tools {
		answer := f.answers[tool.Name]
		if answer == nil {
			answer = func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mcp.NewToolResultText("ok"), nil
			}
		}
		s.AddTool(tool, answer)
	}
	f.inner = server.NewStreamableHTTPServer(s, server.WithDisableLocalhostProtection(true))
}

func (f *fakeMCP) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.headers)
}

func (f *fakeMCP) setTools(tools ...mcp.Tool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tools = tools
	f.rebuild()
}

func (f *fakeMCP) handler() http.Handler {
	f.mu.Lock()
	f.rebuild()
	f.mu.Unlock()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Api-Key")
		f.mu.Lock()
		f.headers = append(f.headers, got)
		inner, secret := f.inner, f.secret
		f.mu.Unlock()
		if secret != "" && got != secret {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func readOnlyTool(name, description string) mcp.Tool {
	return mcp.NewTool(name,
		mcp.WithDescription(description),
		mcp.WithString("query", mcp.Required()),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func connectorFor(t *testing.T, allow staticList, f *fakeMCP) MCPConnector {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/mcp", f.handler())
	c := egresstest.NewClient(t, allow, mux, fakeHost)
	return MCPConnector{Trip: c.CallTripper(), Version: "test"}
}

func fakeTarget(secret string) Target {
	return Target{URL: "https://" + fakeHost + "/mcp", Header: "X-Api-Key", Secret: datasource.NewSecret(secret)}
}

func TestConnectorListsToolsWithTheirHints(t *testing.T) {
	f := &fakeMCP{secret: "s3cret-value", tools: []mcp.Tool{
		readOnlyTool("search", "Search things"),
		mcp.NewTool("wipe", mcp.WithDescription("Wipe it"), mcp.WithDestructiveHintAnnotation(true)),
		mcp.NewTool("plain", mcp.WithDescription("No hints")),
	}}
	c := connectorFor(t, listOf(t, fakeHost), f)
	found, err := c.Tools(t.Context(), fakeTarget("s3cret-value"))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Found{}
	for _, tool := range found {
		by[tool.Name] = tool
	}
	if len(by) != 3 {
		t.Fatalf("found %d tools, want 3", len(by))
	}
	search := by["search"]
	if search.Description != "Search things" || search.ReadOnly == nil || !*search.ReadOnly {
		t.Errorf("search = %+v", search)
	}
	if !strings.Contains(string(search.InputSchema), `"query"`) {
		t.Errorf("schema = %s", search.InputSchema)
	}
	if d := by["wipe"].Destructive; d == nil || !*d {
		t.Errorf("wipe destructive = %v", d)
	}
	if by["plain"].ReadOnly != nil && *by["plain"].ReadOnly {
		t.Errorf("plain must not be read-only")
	}
}

func TestConnectorSendsTheSecretInTheNamedHeader(t *testing.T) {
	f := &fakeMCP{secret: "s3cret-value", tools: []mcp.Tool{readOnlyTool("search", "d")}}
	c := connectorFor(t, listOf(t, fakeHost), f)
	if _, err := c.Tools(t.Context(), fakeTarget("s3cret-value")); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.headers) == 0 {
		t.Fatal("server saw no requests")
	}
	for _, h := range f.headers {
		if h != "s3cret-value" {
			t.Errorf("header = %q", h)
		}
	}
}

func TestConnectorReportsAWrongSecretAsUnauthorized(t *testing.T) {
	f := &fakeMCP{secret: "s3cret-value", tools: []mcp.Tool{readOnlyTool("search", "d")}}
	c := connectorFor(t, listOf(t, fakeHost), f)
	_, err := c.Tools(t.Context(), fakeTarget("wrong"))
	if got := failureCode(err); got != FailureUnauthorized {
		t.Fatalf("failure = %q (%v), want %q", got, err, FailureUnauthorized)
	}
	if strings.Contains(err.Error(), "wrong") || strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks the secret: %v", err)
	}
}

func TestConnectorRefusesAHostOffTheAllowlist(t *testing.T) {
	f := &fakeMCP{tools: []mcp.Tool{readOnlyTool("search", "d")}}
	c := connectorFor(t, listOf(t, "other.example.com"), f)
	_, err := c.Tools(t.Context(), fakeTarget("x"))
	if got := failureCode(err); got != FailureNotAllowed {
		t.Fatalf("failure = %q (%v), want %q", got, err, FailureNotAllowed)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.headers) != 0 {
		t.Errorf("server saw %d requests, want 0", len(f.headers))
	}
}

func TestConnectorReportsANonMCPServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("<html>not json</html>"))
	})
	c := egresstest.NewClient(t, listOf(t, fakeHost), mux, fakeHost)
	conn := MCPConnector{Trip: c.CallTripper(), Version: "test"}
	_, err := conn.Tools(t.Context(), fakeTarget("x"))
	if err == nil {
		t.Fatal("want an error")
	}
	if got := failureCode(err); got == FailureNotAllowed || got == FailureUnauthorized {
		t.Errorf("failure = %q", got)
	}
}

func TestFailureCodeDefaultsToUnreachable(t *testing.T) {
	if got := failureCode(errors.New("boom")); got != FailureUnreachable {
		t.Errorf("failure = %q", got)
	}
}

type refusingTripper struct{ called *bool }

func (r refusingTripper) RoundTrip(*http.Request) (*http.Response, error) {
	*r.called = true
	return nil, errors.New("the public tripper must not carry a private source")
}

func TestAPrivateTargetGoesThroughThePrivateTripperOnly(t *testing.T) {
	f := &fakeMCP{secret: "s3cret-value", tools: []mcp.Tool{readOnlyTool("search", "Search things")}}
	mux := http.NewServeMux()
	mux.Handle("/mcp", f.handler())
	c := egresstest.NewClient(t, listOf(t, fakeHost), mux, fakeHost)
	var publicUsed bool
	conn := MCPConnector{Trip: refusingTripper{called: &publicUsed}, PrivateTrip: c.PrivateCallTripper(), Version: "test"}
	target := fakeTarget("s3cret-value")
	target.Private = true
	found, err := conn.Tools(t.Context(), target)
	if err != nil || len(found) != 1 {
		t.Fatalf("found %v, err %v", found, err)
	}
	if publicUsed {
		t.Error("a private source used the public tripper")
	}
	if _, err := conn.Tools(t.Context(), fakeTarget("s3cret-value")); err == nil || !publicUsed {
		t.Errorf("a public source must use the public tripper: %v", err)
	}
}

func TestAPrivateTargetWithoutAPrivateTripperIsRefusedNotSentOverTheDefaultTransport(t *testing.T) {
	var publicUsed bool
	conn := MCPConnector{Trip: refusingTripper{called: &publicUsed}, Version: "test"}
	target := fakeTarget("x")
	target.Private = true
	_, err := conn.Tools(t.Context(), target)
	if err == nil || publicUsed {
		t.Fatalf("err %v, public used %v", err, publicUsed)
	}
	if got := failureCode(err); got != FailureUnreachable {
		t.Errorf("failure = %q", got)
	}
}
