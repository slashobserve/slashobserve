package inboundmcp

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/slashobserve/slashobserve/internal/egress/egresstest"
)

func sseTarget() Target {
	return Target{URL: "https://" + fakeHost + "/sse", Header: "X-Api-Key"}
}

func sseConnector(t *testing.T, allow staticList, secret string, requests *atomic.Int32) MCPConnector {
	t.Helper()
	s := server.NewMCPServer("fake", "1.0.0")
	s.AddTool(readOnlyTool("get_pods", "List the pods of a namespace."), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("pods in " + req.GetString("query", "")), nil
	})
	inner := server.NewSSEServer(s, server.WithBaseURL("https://"+fakeHost), server.WithSSEDisableLocalhostProtection(true))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			requests.Add(1)
		}
		if secret != "" && r.Header.Get("X-Api-Key") != secret {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
	c := egresstest.NewClient(t, allow, mux, fakeHost)
	return MCPConnector{Trip: c.CallTripper(), Version: "test"}
}

func TestConnectorListsTheToolsOfALegacySSEServer(t *testing.T) {
	conn := sseConnector(t, listOf(t, fakeHost), "", nil)
	found, err := conn.Tools(t.Context(), sseTarget())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if len(found) != 1 || found[0].Name != "get_pods" || found[0].ReadOnly == nil || !*found[0].ReadOnly {
		t.Fatalf("found = %+v", found)
	}
}

func TestConnectorCallsAToolOfALegacySSEServer(t *testing.T) {
	conn := sseConnector(t, listOf(t, fakeHost), "", nil)
	called, err := conn.Call(t.Context(), sseTarget(), "get_pods", map[string]any{"query": "shop"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if called.Text != "pods in shop" || called.IsError {
		t.Fatalf("called = %+v", called)
	}
}

func TestConnectorSendsTheSecretToALegacySSEServer(t *testing.T) {
	conn := sseConnector(t, listOf(t, fakeHost), "s3cret", nil)
	target := sseTarget()
	target.Secret = fakeTarget("s3cret").Secret
	if _, err := conn.Tools(t.Context(), target); err != nil {
		t.Fatalf("Tools with the secret: %v", err)
	}
	target.Secret = fakeTarget("wrong").Secret
	_, err := conn.Tools(t.Context(), target)
	if got := failureCode(err); got != FailureUnauthorized {
		t.Fatalf("failure = %q, want %q", got, FailureUnauthorized)
	}
}

func TestConnectorRefusesALegacySSEServerOffTheAllowlist(t *testing.T) {
	var requests atomic.Int32
	conn := sseConnector(t, listOf(t, "other.example.com"), "", &requests)
	_, err := conn.Tools(t.Context(), sseTarget())
	if got := failureCode(err); got != FailureNotAllowed {
		t.Fatalf("failure = %q, want %q", got, FailureNotAllowed)
	}
	if requests.Load() != 0 {
		t.Errorf("server saw %d requests, want 0", requests.Load())
	}
}

func TestAServerThatRefusesInitializeAndIsNotSSEIsNotAnMCPServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nothing here", http.StatusNotFound)
	})
	c := egresstest.NewClient(t, listOf(t, fakeHost), mux, fakeHost)
	conn := MCPConnector{Trip: c.CallTripper(), Version: "test"}
	_, err := conn.Tools(t.Context(), sseTarget())
	if got := failureCode(err); got != FailureNotMCP {
		t.Fatalf("failure = %q, want %q", got, FailureNotMCP)
	}
}
