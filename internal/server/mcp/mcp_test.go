package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
)

const (
	goodKey   = "Bearer good"
	errorKey  = "Bearer broken"
	otherKey  = "Bearer other"
	publicURL = "http://observe.test:8080"
)

type fakeAuth struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeAuth) BearerPrincipal(_ context.Context, header string) (auth.Principal, bool, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	switch header {
	case goodKey:
		return auth.Principal{Kind: auth.KindServiceAccount, ID: "sa-1", Name: "ci"}, true, nil
	case otherKey:
		return auth.Principal{Kind: auth.KindServiceAccount, ID: "sa-2", Name: "other"}, true, nil
	case errorKey:
		return auth.Principal{}, false, errors.New("database exploded with secret-detail")
	}
	return auth.Principal{}, false, nil
}

type fixture struct {
	handler http.Handler
	authn   *fakeAuth
	logs    *bytes.Buffer
	tools   []tools.Tool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(&syncWriter{w: logs}, nil))
	reg := tools.New(tools.Deps{Logger: logger})
	base, err := url.Parse(publicURL)
	if err != nil {
		t.Fatal(err)
	}
	authn := &fakeAuth{}
	return &fixture{
		handler: NewHandler(reg, authn, Options{PublicURL: base, Version: "test", Logger: logger}),
		authn:   authn,
		logs:    logs,
		tools:   reg.List(),
	}
}

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func rpc(id int, method string, params any) string {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	return string(body)
}

func (f *fixture) post(body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	req.Header.Set("Authorization", goodKey)
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func decodeRPC(t *testing.T, rec *httptest.ResponseRecorder) rpcResponse {
	t.Helper()
	var out rpcResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON-RPC: %v: %s", err, rec.Body.String())
	}
	return out
}

type toolResult struct {
	IsError           bool           `json:"isError"`
	StructuredContent map[string]any `json:"structuredContent"`
	Content           []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func callTool(t *testing.T, f *fixture, name string, args any) toolResult {
	t.Helper()
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	rec := f.post(rpc(1, "tools/call", params), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call status %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeRPC(t, rec)
	if resp.Error != nil {
		t.Fatalf("tools/call rpc error: %s", rec.Body.String())
	}
	var out toolResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %s", rec.Body.String())
	}
	if body["code"] != code || body["error"] == "" {
		t.Fatalf("error body %v, want code %q", body, code)
	}
}

func TestMissingAuthorizationIsRejected(t *testing.T) {
	f := newFixture(t)
	rec := f.post(rpc(1, "tools/list", nil), map[string]string{"Authorization": ""})
	assertErrorBody(t, rec, http.StatusUnauthorized, "unauthenticated")
	if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
		t.Errorf("WWW-Authenticate %q", got)
	}
}

func TestInvalidKeyIsRejected(t *testing.T) {
	f := newFixture(t)
	rec := f.post(rpc(1, "tools/list", nil), map[string]string{"Authorization": "Bearer nope"})
	assertErrorBody(t, rec, http.StatusUnauthorized, "unauthenticated")
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("401 lacks WWW-Authenticate")
	}
}

func TestSessionCookieDoesNotAuthenticate(t *testing.T) {
	f := newFixture(t)
	rec := f.post(rpc(1, "tools/list", nil), map[string]string{"Authorization": "", "Cookie": "slashobserve_session=abc"})
	assertErrorBody(t, rec, http.StatusUnauthorized, "unauthenticated")
	if f.authn.calls != 0 {
		t.Errorf("authenticator called %d times without an Authorization header", f.authn.calls)
	}
}

func TestAuthenticatorFailureIsInternalAndLeaksNothing(t *testing.T) {
	f := newFixture(t)
	rec := f.post(rpc(1, "tools/list", nil), map[string]string{"Authorization": errorKey})
	assertErrorBody(t, rec, http.StatusInternalServerError, "internal")
	if strings.Contains(rec.Body.String(), "secret-detail") {
		t.Errorf("response leaks the failure: %s", rec.Body.String())
	}
	if !strings.Contains(f.logs.String(), "secret-detail") {
		t.Errorf("failure was not logged: %s", f.logs.String())
	}
}

func TestOriginIsCheckedBeforeAuthentication(t *testing.T) {
	cases := []struct {
		origin  string
		allowed bool
	}{
		{"", true},
		{publicURL, true},
		{"HTTP://OBSERVE.TEST:8080", true},
		{"http://evil.example", false},
		{"https://observe.test:8080", false},
		{"http://observe.test", false},
		{"http://observe.test:8080/path", false},
		{"null", false},
	}
	for _, tc := range cases {
		t.Run(tc.origin, func(t *testing.T) {
			f := newFixture(t)
			rec := f.post(rpc(1, "tools/list", nil), map[string]string{"Origin": tc.origin})
			if tc.allowed {
				if rec.Code != http.StatusOK {
					t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
				}
				return
			}
			assertErrorBody(t, rec, http.StatusForbidden, "forbidden_origin")
			if f.authn.calls != 0 {
				t.Errorf("authenticator ran before the origin check")
			}
		})
	}
}

func TestOriginWithoutPublicURLIsRejected(t *testing.T) {
	h := NewHandler(tools.New(tools.Deps{}), &fakeAuth{}, Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(rpc(1, "tools/list", nil)))
	req.Header.Set("Origin", publicURL)
	req.Header.Set("Authorization", goodKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assertErrorBody(t, rec, http.StatusForbidden, "forbidden_origin")
}

func TestInitializeIsStatelessAndCarriesInstructions(t *testing.T) {
	f := newFixture(t)
	rec := f.post(rpc(1, "initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	}), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if id := rec.Header().Get("Mcp-Session-Id"); id != "" {
		t.Errorf("stateless server issued a session id %q", id)
	}
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
		Instructions string `json:"instructions"`
		Capabilities struct {
			Tools map[string]any `json:"tools"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(decodeRPC(t, rec).Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.ProtocolVersion != "2025-11-25" || result.ServerInfo.Name != "slashobserve" || result.ServerInfo.Version != "test" {
		t.Errorf("initialize result %+v", result)
	}
	if !strings.Contains(result.Instructions, "get_spec_guide") || result.Capabilities.Tools == nil {
		t.Errorf("instructions or tool capability missing: %+v", result)
	}
}

func TestToolsListMirrorsTheRegistry(t *testing.T) {
	f := newFixture(t)
	rec := f.post(rpc(1, "tools/list", nil), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
			Annotations struct {
				ReadOnlyHint *bool `json:"readOnlyHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(decodeRPC(t, rec).Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != len(f.tools) || len(f.tools) != 14 {
		t.Fatalf("listed %d tools, registry has %d", len(result.Tools), len(f.tools))
	}
	for _, want := range f.tools {
		var got *int
		for i, g := range result.Tools {
			if g.Name == want.Name {
				got = &i
				break
			}
		}
		if got == nil {
			t.Errorf("tool %s is not listed", want.Name)
			continue
		}
		g := result.Tools[*got]
		if g.Description != want.Description {
			t.Errorf("%s: description differs", want.Name)
		}
		if g.Annotations.ReadOnlyHint == nil || *g.Annotations.ReadOnlyHint != want.ReadOnly {
			t.Errorf("%s: readOnlyHint %v, want %v", want.Name, g.Annotations.ReadOnlyHint, want.ReadOnly)
		}
		var gotSchema, wantSchema any
		if err := json.Unmarshal(g.InputSchema, &gotSchema); err != nil {
			t.Fatalf("%s: schema: %v", want.Name, err)
		}
		_ = json.Unmarshal(want.InputSchema, &wantSchema)
		gotJSON, _ := json.Marshal(gotSchema)
		wantJSON, _ := json.Marshal(wantSchema)
		if !bytes.Equal(gotJSON, wantJSON) {
			t.Errorf("%s: input schema differs\n got %s\nwant %s", want.Name, gotJSON, wantJSON)
		}
	}
}

func TestToolCallRunsThroughTheRegistryAndLogsThePrincipal(t *testing.T) {
	f := newFixture(t)
	res := callTool(t, f, "get_spec_guide", nil)
	if res.IsError {
		t.Fatalf("get_spec_guide failed: %+v", res)
	}
	if guide, _ := res.StructuredContent["guide"].(string); guide == "" {
		t.Errorf("structured content has no guide: %v", res.StructuredContent)
	}
	if len(res.Content) != 1 || !strings.Contains(res.Content[0].Text, `"guide"`) {
		t.Errorf("text content does not mirror the result: %+v", res.Content)
	}
	logs := f.logs.String()
	for _, want := range []string{"mcp tool call", "tool=get_spec_guide", "principal_kind=service_account", "principal_id=sa-1", "is_error=false", "duration="} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %q: %s", want, logs)
		}
	}
}

func TestToolErrorIsReturnedAsToolResultAndArgumentsAreNotLogged(t *testing.T) {
	f := newFixture(t)
	res := callTool(t, f, "get_spec_guide", map[string]any{"leak_marker_7f3a": "value-9c1d"})
	if !res.IsError {
		t.Fatalf("unknown field was accepted: %+v", res)
	}
	if res.StructuredContent["code"] != "invalid_request" {
		t.Errorf("structured content %v", res.StructuredContent)
	}
	logs := f.logs.String()
	if strings.Contains(logs, "leak_marker_7f3a") || strings.Contains(logs, "value-9c1d") {
		t.Errorf("arguments reached the log: %s", logs)
	}
	if !strings.Contains(logs, "is_error=true") {
		t.Errorf("tool error was not logged as an error: %s", logs)
	}
}

func TestUnknownToolIsAToolError(t *testing.T) {
	f := newFixture(t)
	rec := f.post(rpc(1, "tools/call", map[string]any{"name": "drop_database"}), nil)
	resp := decodeRPC(t, rec)
	if resp.Error == nil && !strings.Contains(string(resp.Result), `"isError":true`) {
		t.Errorf("unknown tool succeeded: %s", rec.Body.String())
	}
}

func TestMalformedRequestsAreNotServerErrors(t *testing.T) {
	f := newFixture(t)
	rec := f.post("{not json", nil)
	if rec.Code >= 500 {
		t.Errorf("malformed JSON gave %d", rec.Code)
	}
	if resp := decodeRPC(t, rec); resp.Error == nil || resp.Error.Code != -32700 {
		t.Errorf("malformed JSON response: %s", rec.Body.String())
	}
	rec = f.post(rpc(1, "tools/list", nil), map[string]string{"Content-Type": "text/plain"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("text/plain gave %d", rec.Code)
	}
}

func TestGetDoesNotOpenAStreamAndNeedsAuthentication(t *testing.T) {
	f := newFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", goodKey)
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET gave %d: %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec = httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	assertErrorBody(t, rec, http.StatusUnauthorized, "unauthenticated")
}

func TestOversizedBodyIsRejected(t *testing.T) {
	f := newFixture(t)
	big := rpc(1, "tools/call", map[string]any{
		"name":      "get_spec_guide",
		"arguments": map[string]any{"padding": strings.Repeat("a", maxBodyBytes)},
	})
	rec := f.post(big, nil)
	assertErrorBody(t, rec, http.StatusRequestEntityTooLarge, "too_large")
}

func TestBodyJustUnderTheLimitIsServed(t *testing.T) {
	f := newFixture(t)
	pad := maxBodyBytes - 1024
	big := rpc(1, "tools/call", map[string]any{
		"name":      "get_spec_guide",
		"arguments": map[string]any{"padding": strings.Repeat("a", pad)},
	})
	if len(big) > maxBodyBytes {
		t.Fatalf("test body is %d bytes", len(big))
	}
	rec := f.post(big, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}
func TestInFlightLimitIsPerPrincipalAndReleased(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewServer(f.handler)
	t.Cleanup(srv.Close)

	post := func(key string) (int, string, string) {
		req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(rpc(1, "tools/list", nil)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Retry-After"), string(body)
	}
	waitForStatus := func(key string, want int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if status, _, _ := post(key); status == want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("status %d was never returned", want)
	}

	var stalled []net.Conn
	t.Cleanup(func() {
		for _, c := range stalled {
			_ = c.Close()
		}
	})
	addr := strings.TrimPrefix(srv.URL, "http://")
	for range maxInFlight {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		stalled = append(stalled, conn)
		_, err = io.WriteString(conn, "POST /mcp HTTP/1.1\r\nHost: "+addr+"\r\nAuthorization: "+goodKey+
			"\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{")
		if err != nil {
			t.Fatal(err)
		}
	}

	waitForStatus(goodKey, http.StatusTooManyRequests)
	status, retryAfter, body := post(goodKey)
	if status != http.StatusTooManyRequests || retryAfter != "1" || !strings.Contains(body, "too_many_requests") {
		t.Errorf("over-limit request: %d retry-after %q %s", status, retryAfter, body)
	}
	if status, _, _ := post(otherKey); status != http.StatusOK {
		t.Errorf("another principal was limited: %d", status)
	}

	for _, c := range stalled {
		_ = c.Close()
	}
	waitForStatus(goodKey, http.StatusOK)
}
