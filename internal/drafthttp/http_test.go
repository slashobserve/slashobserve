package drafthttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
)

type fakeDrafter struct {
	draft tools.Draft
	err   error
	got   tools.DraftInput
	calls int
}

func (f *fakeDrafter) DraftQuery(_ context.Context, in tools.DraftInput) (tools.Draft, error) {
	f.got = in
	f.calls++
	return f.draft, f.err
}

func serve(t *testing.T, d Drafter, principal auth.Principal, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), principal))
		c.Next()
	})
	NewHandler(d, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(group))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/loki/draft-query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	return rec
}

var viewer = auth.Principal{Kind: auth.KindUser, ID: "u1", Role: auth.RoleViewer}

func TestDraftAnswersALogQLDraft(t *testing.T) {
	d := &fakeDrafter{draft: tools.Draft{Query: `{service_name="checkout"}`, Language: "logql", Explanation: "Checkout lines.", Warnings: []string{}}}
	rec := serve(t, d, viewer, `{"question":"checkout lines","from":"now-6h","to":"now","signal":"logs"}`)
	want := `{"query":"{service_name=\"checkout\"}","language":"logql","trace_filters":null,"explanation":"Checkout lines.","warnings":[]}`
	if rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	if d.got != (tools.DraftInput{DatasourceUID: "loki", Question: "checkout lines", Signal: "logs", From: "now-6h", To: "now"}) {
		t.Fatalf("input %+v", d.got)
	}
}

func TestDraftAnswersOnlyTheSetTraceFilters(t *testing.T) {
	d := &fakeDrafter{draft: tools.Draft{Language: "filters", TraceFilters: &tools.TraceFilters{Service: "checkout", Min: "500ms"}, Explanation: "Slow checkout traces.", Warnings: []string{"No matches."}}}
	rec := serve(t, d, viewer, `{"question":"slow checkout"}`)
	want := `{"query":"","language":"filters","trace_filters":{"service":"checkout","min":"500ms"},"explanation":"Slow checkout traces.","warnings":["No matches."]}`
	if rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
}

func TestDraftAnswersCannotTranslateWithTheReason(t *testing.T) {
	d := &fakeDrafter{err: &tools.CannotTranslate{Reason: "This Loki datasource holds logs, not CPU metrics."}}
	rec := serve(t, d, viewer, `{"question":"cpu"}`)
	want := `{"error":"This Loki datasource holds logs, not CPU metrics.","code":"cannot_translate","reason":"This Loki datasource holds logs, not CPU metrics."}`
	if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != want {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
}

func TestDraftMapsEachErrorToItsStatus(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{tools.ErrDraftUnavailable, http.StatusServiceUnavailable, "agent_unavailable"},
		{tools.ErrDraftInvalid, http.StatusBadRequest, "invalid_request"},
		{tools.ErrDraftNotFound, http.StatusNotFound, "not_found"},
		{tools.ErrDraftTooMany, http.StatusTooManyRequests, "too_many_requests"},
		{tools.ErrDraftBusy, http.StatusTooManyRequests, "agent_busy"},
		{fmt.Errorf("%w: rate limited", tools.ErrDraftProvider), http.StatusBadGateway, "provider_error"},
		{tools.ErrDraftTimeout, http.StatusGatewayTimeout, "draft_timeout"},
		{&tools.DatasourceError{Code: "datasource_unavailable", Message: "connection refused"}, http.StatusBadGateway, "datasource_unavailable"},
		{&tools.DatasourceError{Code: "query_timeout", Message: "query timed out"}, http.StatusBadGateway, "query_timeout"},
		{errors.New("boom"), http.StatusInternalServerError, "internal"},
	}
	for _, c := range cases {
		rec := serve(t, &fakeDrafter{err: c.err}, viewer, `{"question":"q"}`)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), `"code":"`+c.code+`"`) {
			t.Errorf("%v: status %d, body %s", c.err, rec.Code, rec.Body)
		}
	}
	rec := serve(t, &fakeDrafter{err: tools.ErrDraftUnavailable}, viewer, `{"question":"q"}`)
	if !strings.Contains(rec.Body.String(), "No model is configured on this server, so Ask is off.") {
		t.Errorf("unavailable body %s", rec.Body)
	}
}

func TestDraftRefusesABadBody(t *testing.T) {
	d := &fakeDrafter{}
	if rec := serve(t, d, viewer, `{"question":"q","limit":5}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
		t.Errorf("unknown field: status %d, body %s", rec.Code, rec.Body)
	}
	big := `{"question":"` + strings.Repeat("q", maxBodyBytes) + `"}`
	if rec := serve(t, d, viewer, big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("large body: status %d", rec.Code)
	}
	if d.calls != 0 {
		t.Errorf("drafted %d times", d.calls)
	}
}

func TestDraftAcceptsAViewerKey(t *testing.T) {
	d := &fakeDrafter{draft: tools.Draft{Language: "logql", Query: "{a=\"b\"}", Explanation: "A.", Warnings: []string{}}}
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "k1", Role: auth.RoleViewer}
	if rec := serve(t, d, key, `{"question":"q"}`); rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	if rec := serve(t, d, auth.Principal{}, `{"question":"q"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("no role: status %d", rec.Code)
	}
}
