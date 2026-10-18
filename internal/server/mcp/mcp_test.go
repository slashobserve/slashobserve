package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	mcplib "github.com/mark3labs/mcp-go/mcp"

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
	if id, ok := strings.CutPrefix(header, "Bearer pool-"); ok {
		return auth.Principal{Kind: auth.KindServiceAccount, ID: "pool-" + id, Name: "pool"}, true, nil
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

func (f *fixture) waitHeld(t *testing.T, principalID string, want int) {
	t.Helper()
	s := &f.handler.(*handler).slots
	key := principalKey(auth.Principal{Kind: auth.KindServiceAccount, ID: principalID})
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		held := s.held[key]
		s.mu.Unlock()
		if held == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s holds %d slots, want %d", principalID, held, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
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
		{"http://observe.test:80", false},
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
				ReadOnlyHint    *bool `json:"readOnlyHint"`
				DestructiveHint *bool `json:"destructiveHint"`
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
		if !want.ReadOnly && (g.Annotations.DestructiveHint == nil || *g.Annotations.DestructiveHint) {
			t.Errorf("%s: destructiveHint %v, want false", want.Name, g.Annotations.DestructiveHint)
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

	f.waitHeld(t, "sa-1", maxInFlight)
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

func TestOriginNormalisesDefaultPorts(t *testing.T) {
	cases := []struct {
		public string
		origin string
		want   bool
	}{
		{"https://observe.test", "https://observe.test:443", true},
		{"https://observe.test:443", "https://observe.test", true},
		{"http://observe.test", "http://observe.test:80", true},
		{"http://observe.test:80", "http://OBSERVE.test", true},
		{"https://observe.test", "http://observe.test", false},
		{"https://observe.test", "https://observe.test:80", false},
		{"http://observe.test", "http://observe.test:443", false},
		{"http://[::1]:8080", "http://[::1]:8080", true},
		{"http://[::1]", "http://[::1]:80", true},
	}
	for _, tc := range cases {
		t.Run(tc.public+" "+tc.origin, func(t *testing.T) {
			base, err := url.Parse(tc.public)
			if err != nil {
				t.Fatal(err)
			}
			h := NewHandler(tools.New(tools.Deps{}), &fakeAuth{}, Options{PublicURL: base, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(rpc(1, "tools/list", nil)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Authorization", goodKey)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if tc.want && rec.Code != http.StatusOK {
				t.Fatalf("origin %s was refused for %s: %d %s", tc.origin, tc.public, rec.Code, rec.Body.String())
			}
			if !tc.want {
				assertErrorBody(t, rec, http.StatusForbidden, "forbidden_origin")
			}
		})
	}
}

func TestToolCallWorksWithTheLatestProtocolVersionAndBearerPrincipal(t *testing.T) {
	f := newFixture(t)
	rec := f.post(rpc(1, "tools/call", map[string]any{
		"name": "get_spec_guide",
		"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		},
	}), map[string]string{
		"MCP-Protocol-Version": "2026-07-28",
		"Mcp-Method":           "tools/call",
		"Mcp-Name":             "get_spec_guide",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeRPC(t, rec)
	if resp.Error != nil {
		t.Fatalf("rpc error: %s", rec.Body.String())
	}
	var res toolResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.StructuredContent["guide"] == nil {
		t.Fatalf("tool call failed: %+v", res)
	}
	logs := f.logs.String()
	for _, want := range []string{"principal_kind=service_account", "principal_id=sa-1"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %q: %s", want, logs)
		}
	}
}

func callDirect(ctx context.Context, f *fixture, call func(context.Context, string, json.RawMessage) (tools.Result, error)) *mcplib.CallToolResult {
	return callDirectWith(ctx, f, call, mcplib.CallToolRequest{})
}

func callDirectWith(ctx context.Context, f *fixture, call func(context.Context, string, json.RawMessage) (tools.Result, error), req mcplib.CallToolRequest) *mcplib.CallToolResult {
	h := f.handler.(*handler)
	h.call = call
	ctx = auth.WithPrincipal(ctx, auth.Principal{Kind: auth.KindServiceAccount, ID: "sa-1", Name: "ci"})
	res, _ := h.callTool("get_spec_guide")(ctx, req)
	return res
}

type unencodable struct{}

func (unencodable) MarshalJSON() ([]byte, error) { return nil, errors.New("secret-detail") }

func resultText(t *testing.T, res *mcplib.CallToolResult) string {
	t.Helper()
	if res == nil || !res.IsError || len(res.Content) != 1 {
		t.Fatalf("result %+v is not a single error content", res)
	}
	text, ok := res.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatalf("content %T is not text", res.Content[0])
	}
	return text.Text
}

func TestClientCancellationIsLoggedAtInfo(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := callDirect(ctx, f, func(ctx context.Context, _ string, _ json.RawMessage) (tools.Result, error) {
		return tools.Result{}, fmt.Errorf("upstream said secret-detail: %w", ctx.Err())
	})
	if got := resultText(t, res); got != internalErrorMsg {
		t.Errorf("result text %q", got)
	}
	logs := f.logs.String()
	for _, want := range []string{"level=INFO", "canceled by client", "tool=get_spec_guide", "principal_kind=service_account", "principal_id=sa-1", "error_code=canceled", "error_type=*errors.errorString"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %q: %s", want, logs)
		}
	}
	if strings.Contains(logs, "secret-detail") || strings.Contains(logs, " error=") {
		t.Errorf("error text reached the log: %s", logs)
	}
	if strings.Contains(logs, "level=ERROR") {
		t.Errorf("cancellation was logged as an error: %s", logs)
	}
}

func TestInternalToolFailureIsLoggedByCodeAndTypeOnly(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{"plain error", errors.New("upstream exploded with secret-detail"), []string{"error_code=tool_failed", "error_type=*errors.errorString"}},
		{
			"postgres error",
			fmt.Errorf("tool create_dashboard: %w", &pgconn.PgError{Code: "23505", Message: "duplicate key secret-detail", Detail: "Key (x)=(secret-detail)"}),
			[]string{"error_code=tool_failed", "error_type=*pgconn.PgError", "sqlstate=23505"},
		},
		{"oversize result", fmt.Errorf("tool x: %w: secret-detail", tools.ErrResultTooLarge), []string{"error_code=result_too_large"}},
		{"panic", fmt.Errorf("tool x: %w: secret-detail", tools.ErrPanicked), []string{"error_code=tool_panicked"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			res := callDirect(context.Background(), f, func(context.Context, string, json.RawMessage) (tools.Result, error) {
				return tools.Result{}, tc.err
			})
			if got := resultText(t, res); got != internalErrorMsg {
				t.Errorf("result text %q", got)
			}
			logs := f.logs.String()
			want := append([]string{"level=ERROR", "mcp tool call failed", "tool=get_spec_guide", "principal_kind=service_account", "principal_id=sa-1"}, tc.want...)
			for _, w := range want {
				if !strings.Contains(logs, w) {
					t.Errorf("log lacks %q: %s", w, logs)
				}
			}
			if strings.Contains(logs, "secret-detail") || strings.Contains(logs, "duplicate key") || strings.Contains(logs, " error=") {
				t.Errorf("error text reached the log: %s", logs)
			}
		})
	}
}

func TestEncodingFailuresAreLoggedByCodeAndTypeOnly(t *testing.T) {
	t.Run("arguments", func(t *testing.T) {
		f := newFixture(t)
		req := mcplib.CallToolRequest{}
		req.Params.Arguments = unencodable{}
		res := callDirectWith(context.Background(), f, func(context.Context, string, json.RawMessage) (tools.Result, error) {
			return tools.Result{}, nil
		}, req)
		if got := resultText(t, res); got != internalErrorMsg {
			t.Errorf("result text %q", got)
		}
		assertSafeFailureLog(t, f.logs.String(), "mcp tool arguments not encodable", "invalid_arguments")
	})
	t.Run("result", func(t *testing.T) {
		f := newFixture(t)
		res := callDirect(context.Background(), f, func(context.Context, string, json.RawMessage) (tools.Result, error) {
			return tools.Result{Content: unencodable{}}, nil
		})
		if got := resultText(t, res); got != internalErrorMsg {
			t.Errorf("result text %q", got)
		}
		assertSafeFailureLog(t, f.logs.String(), "mcp tool result not encodable", "result_not_encodable")
	})
}

func assertSafeFailureLog(t *testing.T, logs, msg, code string) {
	t.Helper()
	for _, w := range []string{"level=ERROR", msg, "tool=get_spec_guide", "principal_id=sa-1", "error_code=" + code, "error_type="} {
		if !strings.Contains(logs, w) {
			t.Errorf("log lacks %q: %s", w, logs)
		}
	}
	if strings.Contains(logs, "secret-detail") || strings.Contains(logs, " error=") {
		t.Errorf("error text reached the log: %s", logs)
	}
}

func TestPanicInToolGlueNeverReachesTheClient(t *testing.T) {
	f := newFixture(t)
	f.handler.(*handler).call = func(context.Context, string, json.RawMessage) (tools.Result, error) {
		panic("panic-secret-4d2e")
	}
	rec := f.post(rpc(1, "tools/call", map[string]any{"name": "get_spec_guide", "arguments": map[string]any{"arg_marker": "arg-9b7c"}}), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "panic-secret-4d2e") {
		t.Errorf("panic text reached the client: %s", rec.Body.String())
	}
	var res toolResult
	if err := json.Unmarshal(decodeRPC(t, rec).Result, &res); err != nil {
		t.Fatal(err)
	}
	if !res.IsError || len(res.Content) != 1 || res.Content[0].Text != internalErrorMsg {
		t.Errorf("result %+v", res)
	}
	logs := f.logs.String()
	for _, want := range []string{"level=ERROR", "tool=get_spec_guide", "panic_type=string"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %q: %s", want, logs)
		}
	}
	if strings.Contains(logs, "panic-secret-4d2e") || strings.Contains(logs, "arg-9b7c") {
		t.Errorf("panic value or arguments reached the log: %s", logs)
	}
}

func TestGlobalInFlightLimitCoversOnlyRunningRequests(t *testing.T) {
	f := newFixture(t)
	h := f.handler.(*handler)
	release := make(chan struct{})
	entered := make(chan struct{}, maxTotalInFlight)
	h.call = func(ctx context.Context, _ string, _ json.RawMessage) (tools.Result, error) {
		if p, _ := auth.FromContext(ctx); p.ID != "pool-free" {
			entered <- struct{}{}
			<-release
		}
		return tools.Result{Content: map[string]string{"ok": "true"}}, nil
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	post := func(key string) (int, string, string) {
		req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(rpc(1, "tools/call", map[string]any{"name": "get_spec_guide"})))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Retry-After"), string(body)
	}

	var running sync.WaitGroup
	statuses := make(chan int, maxTotalInFlight)
	for p := range maxTotalInFlight / maxInFlight {
		for range maxInFlight {
			running.Add(1)
			go func() {
				defer running.Done()
				status, _, _ := post("Bearer pool-" + strconv.Itoa(p))
				statuses <- status
			}()
		}
	}
	for range maxTotalInFlight {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("running requests never reached the tool")
		}
	}

	status, retryAfter, body := post("Bearer pool-extra")
	if status != http.StatusServiceUnavailable || retryAfter != "1" || !strings.Contains(body, `"code":"busy"`) {
		t.Errorf("over-capacity request: %d retry-after %q %s", status, retryAfter, body)
	}
	if status, _, body := post("Bearer pool-0"); status != http.StatusTooManyRequests || !strings.Contains(body, "too_many_requests") {
		t.Errorf("principal at its own limit: %d %s", status, body)
	}

	close(release)
	running.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Errorf("running request ended with %d", status)
		}
	}
	if status, _, body := post("Bearer pool-extra"); status != http.StatusOK {
		t.Errorf("slots were not released: %d %s", status, body)
	}
}

func TestSlowUploadsHoldOnlyThePrincipalSlot(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewServer(f.handler)
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")

	post := func(key string) (int, string) {
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
		return resp.StatusCode, string(body)
	}

	var stalled []net.Conn
	t.Cleanup(func() {
		for _, c := range stalled {
			_ = c.Close()
		}
	})
	principals := maxTotalInFlight/maxInFlight + 1
	for p := range principals {
		for range maxInFlight {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			stalled = append(stalled, conn)
			_, err = io.WriteString(conn, "POST /mcp HTTP/1.1\r\nHost: "+addr+"\r\nAuthorization: Bearer pool-"+strconv.Itoa(p)+
				"\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{")
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(stalled) <= maxTotalInFlight {
		t.Fatalf("only %d uploads are stalled, want more than the global cap %d", len(stalled), maxTotalInFlight)
	}
	for p := range principals {
		f.waitHeld(t, "pool-"+strconv.Itoa(p), maxInFlight)
	}

	for p := range principals {
		if status, body := post("Bearer pool-" + strconv.Itoa(p)); status != http.StatusTooManyRequests {
			t.Errorf("pool-%d: status %d %s, want 429 while its uploads hold its slots", p, status, body)
		}
	}
	if status, body := post("Bearer pool-free"); status != http.StatusOK {
		t.Errorf("stalled uploads took global slots: %d %s", status, body)
	}
}
