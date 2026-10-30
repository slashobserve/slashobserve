package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func init() { gin.SetMode(gin.TestMode) }

const (
	dashboardID   = "0a1b2c3d-0000-4000-8000-000000000001"
	sentinelValue = "sekret-value-42"
)

type fakeSpecs struct {
	latest    int
	latestErr error
	dash      *spec.Dashboard
}

func (f *fakeSpecs) LatestVersion(context.Context, pgtype.UUID) (int, error) {
	return f.latest, f.latestErr
}

func (f *fakeSpecs) VersionSpec(context.Context, pgtype.UUID, int) (*spec.Dashboard, error) {
	return f.dash, nil
}

type fakeSender struct {
	mu      sync.Mutex
	sent    []egress.SendRequest
	result  egress.SendResult
	err     error
	entered chan struct{}
	release chan struct{}
}

func (f *fakeSender) Send(_ context.Context, req egress.SendRequest) (egress.SendResult, error) {
	f.mu.Lock()
	f.sent = append(f.sent, req)
	f.mu.Unlock()
	if f.entered != nil {
		f.entered <- struct{}{}
		<-f.release
	}
	return f.result, f.err
}

func (f *fakeSender) calls() []egress.SendRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]egress.SendRequest(nil), f.sent...)
}

func canvasDashboard() *spec.Dashboard {
	return &spec.Dashboard{
		Variables: []spec.Variable{{Name: "env", Type: spec.CustomVariable}},
		Panels: []spec.Panel{{
			ID:   "orders-flow",
			Type: spec.Canvas,
			Display: &spec.Display{Elements: []spec.CanvasElement{{
				ID: "restart",
				Actions: []spec.Action{{
					Title: "Restart",
					URL:   "https://deploy.example.com/api/${env}/restart",
					Body:  `{"env":"${env}","who":"${__user.email}"}`,
				}},
			}}},
		}},
	}
}

type rig struct {
	specs  *fakeSpecs
	sender *fakeSender
	logs   *logtest.Capture
	engine *gin.Engine
}

func newRig(t *testing.T) *rig {
	t.Helper()
	logger, logs := logtest.New()
	specs := &fakeSpecs{latest: 7, dash: canvasDashboard()}
	sender := &fakeSender{result: egress.SendResult{Status: http.StatusNoContent}}
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		if kind := c.GetHeader("X-Test-Kind"); kind != "" {
			p := auth.Principal{
				Kind: auth.PrincipalKind(kind), ID: c.GetHeader("X-Test-User"), Email: "ada@example.com",
				Name: "Ada", Role: auth.RoleEditor,
			}
			c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
		}
	})
	requireEdit := func(c *gin.Context) {
		if c.GetHeader("X-Test-Edit") != "yes" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "your role does not allow this", "code": "forbidden_role"})
			return
		}
		c.Next()
	}
	NewHandler(specs, sender, requireEdit, logger).Register(auth.NewRoutes(api))
	return &rig{specs: specs, sender: sender, logs: logs, engine: engine}
}

func (r *rig) post(body any, headers map[string]string) *httptest.ResponseRecorder {
	raw, ok := body.(string)
	if !ok {
		encoded, _ := json.Marshal(body)
		raw = string(encoded)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/actions", bytes.NewBufferString(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

var editor = map[string]string{"X-Test-Kind": "user", "X-Test-User": "user-1", "X-Test-Edit": "yes"}

func validBody() map[string]any {
	return map[string]any{
		"version": 7, "panel": "orders-flow", "element": "restart", "action": 0,
		"variables": map[string][]string{"env": {sentinelValue}},
		"from":      "2026-10-30T10:00:00Z", "to": "2026-10-30T11:00:00Z",
	}
}

func with(body map[string]any, key string, value any) map[string]any {
	out := map[string]any{}
	for k, v := range body {
		out[k] = v
	}
	if value == nil {
		delete(out, key)
	} else {
		out[key] = value
	}
	return out
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	Host  string `json:"host"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return body
}

func TestRunSendsTheSavedActionWithExpandedValuesAndAnswersWithTheStatus(t *testing.T) {
	r := newRig(t)
	rec := r.post(validBody(), editor)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":204,"ok":true}` {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	calls := r.sender.calls()
	if len(calls) != 1 {
		t.Fatalf("sender called %d times", len(calls))
	}
	got := calls[0]
	if got.Method != "POST" || got.URL != "https://deploy.example.com/api/"+sentinelValue+"/restart" {
		t.Errorf("sent %s %s", got.Method, got.URL)
	}
	if want := `{"env":"` + sentinelValue + `","who":"ada@example.com"}`; string(got.Body) != want {
		t.Errorf("body = %s, want %s", got.Body, want)
	}
	if got.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", got.Header.Get("Content-Type"))
	}
}

func TestRunWritesOneAuditRecordWithoutURLsBodiesOrValues(t *testing.T) {
	r := newRig(t)
	if rec := r.post(validBody(), editor); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	record := r.logs.Find(t, "canvas action run")
	want := map[string]any{
		"actor_id": "user-1", "actor_role": "editor", "dashboard_id": dashboardID, "version": float64(7),
		"panel": "orders-flow", "element": "restart", "action": float64(0), "method": "POST",
		"host": "deploy.example.com", "status": float64(204), "outcome": "ok",
	}
	for key, value := range want {
		if record[key] != value {
			t.Errorf("%s = %v, want %v", key, record[key], value)
		}
	}
	if _, ok := record["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v", record["duration_ms"])
	}
	for _, key := range []string{"url", "body", "headers", "query", "variables", "path"} {
		if _, ok := record[key]; ok {
			t.Errorf("record has a %q key: %v", key, record)
		}
	}
	r.logs.AssertAbsent(t, sentinelValue, "ada@example.com", "/api/", "/restart")
}

func TestRunReportsARemoteErrorStatusAsOkFalse(t *testing.T) {
	r := newRig(t)
	r.sender.result = egress.SendResult{Status: http.StatusInternalServerError}
	rec := r.post(validBody(), editor)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":500,"ok":false}` {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	if outcome := r.logs.Find(t, "canvas action run")["outcome"]; outcome != "remote_error" {
		t.Errorf("outcome = %v", outcome)
	}
}

func TestRunRefusesBeforeTheNetworkAndLogsNothing(t *testing.T) {
	cases := []struct {
		name     string
		body     any
		prepare  func(*rig)
		wantCode int
		wantErr  string
	}{
		{"stale version", with(validBody(), "version", 6), nil, http.StatusConflict, "stale_version"},
		{"newer version than saved", with(validBody(), "version", 8), nil, http.StatusConflict, "stale_version"},
		{"unknown panel", with(validBody(), "panel", "nope"), nil, http.StatusNotFound, "action_not_found"},
		{"unknown element", with(validBody(), "element", "nope"), nil, http.StatusNotFound, "action_not_found"},
		{"action index past the list", with(validBody(), "action", 1), nil, http.StatusNotFound, "action_not_found"},
		{"dashboard deleted meanwhile", validBody(), func(r *rig) { r.specs.latestErr = dashboards.ErrNotFound }, http.StatusNotFound, "not_found"},
		{"panel that is not a canvas", validBody(), func(r *rig) { r.specs.dash.Panels[0].Type = spec.Text }, http.StatusNotFound, "action_not_found"},
		{"variable the dashboard does not declare", with(validBody(), "variables", map[string][]string{"env": {"a"}, "other": {"b"}}), nil, http.StatusBadRequest, "invalid_request"},
		{"too many values", with(validBody(), "variables", map[string][]string{"env": make([]string, 101)}), nil, http.StatusBadRequest, "invalid_request"},
		{"value over 1000 bytes", with(validBody(), "variables", map[string][]string{"env": {string(make([]byte, 1001))}}), nil, http.StatusBadRequest, "invalid_request"},
		{"too many fields", with(validBody(), "fields", manyFields(51)), nil, http.StatusBadRequest, "invalid_request"},
		{"field value over 1000 bytes", with(validBody(), "fields", map[string]string{"a": string(make([]byte, 1001))}), nil, http.StatusBadRequest, "invalid_request"},
		{"missing version", with(validBody(), "version", nil), nil, http.StatusBadRequest, "invalid_request"},
		{"missing action index", with(validBody(), "action", nil), nil, http.StatusBadRequest, "invalid_request"},
		{"negative action index", with(validBody(), "action", -1), nil, http.StatusBadRequest, "invalid_request"},
		{"missing from", with(validBody(), "from", nil), nil, http.StatusBadRequest, "invalid_request"},
		{"time that is not RFC 3339", with(validBody(), "to", "yesterday"), nil, http.StatusBadRequest, "invalid_request"},
		{"a field the caller may not choose", with(validBody(), "url", "https://evil.example/"), nil, http.StatusBadRequest, "invalid_request"},
		{"not JSON", "not json", nil, http.StatusBadRequest, "invalid_request"},
		{"declared variable with no value", with(validBody(), "variables", nil), nil, http.StatusBadRequest, "action_invalid"},
		{"expansion that breaks the JSON body", with(validBody(), "variables", map[string][]string{"env": {"x"}}), func(r *rig) {
			r.specs.dash.Panels[0].Display.Elements[0].Actions[0].Body = `{"n":${env}}`
		}, http.StatusBadRequest, "action_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			if tc.prepare != nil {
				tc.prepare(r)
			}
			rec := r.post(tc.body, editor)
			if rec.Code != tc.wantCode || decodeError(t, rec).Code != tc.wantErr {
				t.Fatalf("response = %d %s, want %d %s", rec.Code, rec.Body, tc.wantCode, tc.wantErr)
			}
			if len(r.sender.calls()) != 0 {
				t.Error("sender was called")
			}
			r.logs.AssertAbsent(t, "canvas action run")
		})
	}
}

func manyFields(n int) map[string]string {
	fields := map[string]string{}
	for i := range n {
		fields[string(rune('a'+i%26))+string(rune('a'+i/26))] = "v"
	}
	return fields
}

func TestRunMapsSenderErrorsAndStillAudits(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantCode    int
		wantErr     string
		wantHost    string
		wantOutcome string
	}{
		{"domain not allowed", &egress.HostError{Host: "deploy.example.com", Err: egress.ErrNotAllowed}, http.StatusForbidden, "outbound_domain_not_allowed", "deploy.example.com", "refused"},
		{"unavailable", &egress.HostError{Host: "deploy.example.com", Err: egress.ErrUnavailable}, http.StatusBadGateway, "action_failed", "deploy.example.com", "failed"},
		{"blocked address", &egress.HostError{Host: "deploy.example.com", Err: egress.ErrBlockedAddress}, http.StatusBadGateway, "action_failed", "deploy.example.com", "refused"},
		{"timeout waiting for a slot", context.DeadlineExceeded, http.StatusBadGateway, "action_failed", "deploy.example.com", "failed"},
		{"invalid address after expansion", &egress.HostError{Host: "deploy.example.com", Err: egress.ErrInvalidURL}, http.StatusBadRequest, "action_invalid", "", "refused"},
		{"body too large", &egress.HostError{Host: "deploy.example.com", Err: egress.ErrTooLarge}, http.StatusBadRequest, "action_invalid", "", "refused"},
		{"allowlist unreadable", errors.New("read outbound allowlist: connection refused"), http.StatusInternalServerError, "internal", "", "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.sender.err = tc.err
			rec := r.post(validBody(), editor)
			got := decodeError(t, rec)
			if rec.Code != tc.wantCode || got.Code != tc.wantErr || got.Host != tc.wantHost {
				t.Fatalf("response = %d %s, want %d %s host %q", rec.Code, rec.Body, tc.wantCode, tc.wantErr, tc.wantHost)
			}
			record := r.logs.Find(t, "canvas action run")
			if record["outcome"] != tc.wantOutcome || record["status"] != float64(0) {
				t.Errorf("audit = %v, want outcome %s status 0", record, tc.wantOutcome)
			}
			r.logs.AssertAbsent(t, sentinelValue, "connection refused")
		})
	}
}

func TestRunAllowsTwoInFlightPerPersonAndRefusesAThird(t *testing.T) {
	r := newRig(t)
	r.sender.entered = make(chan struct{}, 4)
	r.sender.release = make(chan struct{})

	var wg sync.WaitGroup
	codes := make(chan int, 3)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- r.post(validBody(), editor).Code
		}()
	}
	<-r.sender.entered
	<-r.sender.entered

	rec := r.post(validBody(), editor)
	if rec.Code != http.StatusTooManyRequests || decodeError(t, rec).Code != "too_many_actions" {
		t.Fatalf("third request = %d %s", rec.Code, rec.Body)
	}
	other := map[string]string{"X-Test-Kind": "user", "X-Test-User": "user-2", "X-Test-Edit": "yes"}
	otherDone := make(chan int)
	go func() { otherDone <- r.post(validBody(), other).Code }()
	<-r.sender.entered

	close(r.sender.release)
	wg.Wait()
	if code := <-otherDone; code != http.StatusOK {
		t.Errorf("another person's request = %d, want 200", code)
	}
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Errorf("in-flight request = %d, want 200", code)
		}
	}
	if rec := r.post(validBody(), editor); rec.Code != http.StatusOK {
		t.Errorf("request after the others finished = %d, want 200", rec.Code)
	}
}

func TestRunNeedsAPersonWithEditOnTheDashboard(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		code    string
	}{
		{"person without edit", map[string]string{"X-Test-Kind": "user", "X-Test-User": "user-1"}, "forbidden_role"},
		{"service account key", map[string]string{"X-Test-Kind": "service_account", "X-Test-User": "key-1", "X-Test-Edit": "yes"}, "forbidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			rec := r.post(validBody(), tc.headers)
			if rec.Code != http.StatusForbidden || decodeError(t, rec).Code != tc.code {
				t.Fatalf("response = %d %s", rec.Code, rec.Body)
			}
			if len(r.sender.calls()) != 0 {
				t.Error("sender was called")
			}
			r.logs.AssertAbsent(t, "canvas action run")
		})
	}
}
