package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const sidecarToken = "Bearer soa_child"

var builtIn = claims.Claimer{Kind: revisions.AssignBuiltin, ID: "server-1"}

type sidecarAuth struct{}

func (sidecarAuth) Authenticate(_ context.Context, header string) (Credential, bool, error) {
	if header == sidecarToken {
		return Credential{Claimer: &builtIn, Loopback: true, ClaimOnly: true}, true, nil
	}
	return Credential{}, false, nil
}

type claimingService struct {
	mu      sync.Mutex
	claimed []claims.Claimer
}

func (s *claimingService) Claim(_ context.Context, c claims.Claimer, _ time.Duration) (claims.Offer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimed = append(s.claimed, c)
	return claims.Offer{Run: &claims.OfferedRun{ID: runRequestID, Attempt: 1, Kind: "create", Source: "observe", Prompt: "checkout latency"}}, nil
}

func (*claimingService) ClaimedRun(context.Context, claims.Claimer, string, int) (revisions.Request, error) {
	return revisions.Request{}, claims.ErrNotYourRun
}

func (*claimingService) Report(context.Context, claims.Claimer, claims.ReportInput) (claims.ReportAnswer, error) {
	return claims.ReportAnswer{}, claims.ErrNotYourRun
}

func newSidecarHandler(t *testing.T) (http.Handler, *claimingService, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(&syncWriter{w: logs}, nil))
	service := &claimingService{}
	reg := tools.New(tools.Deps{Logger: logger, Claims: service})
	base, err := url.Parse(publicURL)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(reg, sidecarAuth{}, Options{PublicURL: base, Version: "test", Logger: logger}), service, logs
}

func postFrom(h http.Handler, remote, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.RemoteAddr = remote
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSidecarSessionListsExactlyTheFourClaimTools(t *testing.T) {
	h, _, _ := newSidecarHandler(t)
	rec := postFrom(h, "127.0.0.1:50000", sidecarToken, rpc(1, "tools/list", map[string]any{}))
	var result struct {
		Tools []listedTool `json:"tools"`
	}
	decodeResult(t, rec, &result)
	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	want := []string{"claim_run", "finish_run", "get_run_brief", "report_run"}
	if !slices.Equal(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
}

func TestSidecarSessionIsRefusedFromANonLoopbackPeer(t *testing.T) {
	h, service, _ := newSidecarHandler(t)
	for _, remote := range []string{"10.0.0.5:50000", "[2001:db8::1]:50000", "not-an-address"} {
		rec := postFrom(h, remote, sidecarToken, rpc(1, "tools/list", map[string]any{}))
		assertErrorBody(t, rec, http.StatusUnauthorized, "unauthenticated")
	}
	if len(service.claimed) != 0 {
		t.Fatalf("claimed from a refused peer: %v", service.claimed)
	}
}

func TestSidecarSessionAcceptsLoopbackPeersOfBothFamilies(t *testing.T) {
	h, _, _ := newSidecarHandler(t)
	for _, remote := range []string{"127.0.0.1:50000", "[::1]:50000", "[::ffff:127.0.0.1]:50000"} {
		rec := postFrom(h, remote, sidecarToken, rpc(1, "tools/list", map[string]any{}))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", remote, rec.Code, rec.Body.String())
		}
	}
}

func TestSidecarClaimRunClaimsAsTheBuiltInClaimer(t *testing.T) {
	h, service, logs := newSidecarHandler(t)
	body := rpc(1, "tools/call", map[string]any{"name": "claim_run", "arguments": map[string]any{"wait_seconds": 0}})
	rec := postFrom(h, "127.0.0.1:50000", sidecarToken, body)
	var result toolResult
	decodeResult(t, rec, &result)
	if result.IsError {
		t.Fatalf("claim_run failed: %s", rec.Body.String())
	}
	run, _ := result.StructuredContent["run"].(map[string]any)
	if run["id"] != runRequestID || run["prompt"] != "checkout latency" {
		t.Fatalf("run = %v", run)
	}
	if len(service.claimed) != 1 || service.claimed[0] != builtIn {
		t.Fatalf("claimers = %v, want [%v]", service.claimed, builtIn)
	}
	if strings.Contains(logs.String(), "checkout latency") {
		t.Fatalf("prompt text was logged: %s", logs.String())
	}
}

func TestSidecarSessionRefusesATokenToolOutsideTheClaimSet(t *testing.T) {
	h, _, _ := newSidecarHandler(t)
	body := rpc(1, "tools/call", map[string]any{"name": "list_dashboards", "arguments": map[string]any{}})
	rec := postFrom(h, "127.0.0.1:50000", sidecarToken, body)
	if resp := decodeRPC(t, rec); resp.Error == nil {
		t.Fatalf("a tool outside the claim set ran: %s", rec.Body.String())
	}
}

func decodeResult(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeRPC(t, rec)
	if resp.Error != nil {
		t.Fatalf("rpc error: %s", rec.Body.String())
	}
	if err := json.Unmarshal(resp.Result, into); err != nil {
		t.Fatal(err)
	}
}
