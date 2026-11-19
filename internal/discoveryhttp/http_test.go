package discoveryhttp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
)

type fakeReader struct {
	out    any
	err    error
	table  string
	offset *int
	limit  *int
	uid    string
	calls  int
}

func (f *fakeReader) DatasourceSchema(_ context.Context, uid, table string, offset *int) (any, error) {
	f.uid, f.table, f.offset = uid, table, offset
	f.calls++
	return f.out, f.err
}

func (f *fakeReader) DatasourceSample(_ context.Context, uid, table string, limit *int) (any, error) {
	f.uid, f.table, f.limit = uid, table, limit
	f.calls++
	return f.out, f.err
}

func get(t *testing.T, r Reader, target string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), auth.Principal{Kind: auth.KindUser, ID: "u1", Role: auth.RoleViewer}))
		c.Next()
	})
	NewHandler(r, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(group))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1"+target, nil))
	return rec
}

func TestSchemaPassesTableAndOffsetAndAnswersTheToolsBody(t *testing.T) {
	f := &fakeReader{out: map[string]any{"table": "otel_logs", "columns": []string{"Body"}}}
	rec := get(t, f, "/datasources/ch/schema?table=otel_logs&offset=3")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"columns":["Body"],"table":"otel_logs"}` {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	if f.uid != "ch" || f.table != "otel_logs" || f.offset == nil || *f.offset != 3 {
		t.Fatalf("got uid %q table %q offset %v", f.uid, f.table, f.offset)
	}
}

func TestSamplePassesTableAndLimit(t *testing.T) {
	f := &fakeReader{out: map[string]any{"rows": []any{}}}
	rec := get(t, f, "/datasources/ch/sample?table=otel_logs&limit=7")
	if rec.Code != http.StatusOK || f.limit == nil || *f.limit != 7 || f.table != "otel_logs" {
		t.Fatalf("status %d, limit %v, table %q", rec.Code, f.limit, f.table)
	}
}

func TestRequestsWithBadParametersAreRefusedBeforeTheRead(t *testing.T) {
	for _, target := range []string{
		"/datasources/ch/schema?bogus=1",
		"/datasources/ch/schema?limit=2",
		"/datasources/ch/schema?offset=x",
		"/datasources/ch/schema?offset=1&offset=2",
		"/datasources/ch/schema?table=a&table=b",
		"/datasources/ch/sample?offset=2",
		"/datasources/ch/sample?limit=1.5",
		"/datasources/ch/sample?limit=",
		"/datasources/ch/sample?%zz",
	} {
		f := &fakeReader{}
		rec := get(t, f, target)
		if rec.Code != http.StatusBadRequest || f.calls != 0 {
			t.Errorf("%s: status %d, calls %d, body %s", target, rec.Code, f.calls, rec.Body)
		}
	}
}

func TestEachToolRefusalMapsToItsStatus(t *testing.T) {
	tests := []struct {
		code   string
		status int
		want   string
	}{
		{"invalid_request", http.StatusBadRequest, "invalid_request"},
		{"unknown_datasource", http.StatusNotFound, "not_found"},
		{"query_timeout", http.StatusGatewayTimeout, "query_timeout"},
		{"datasource_down", http.StatusBadGateway, "datasource_down"},
		{"too_many_requests", http.StatusTooManyRequests, "too_many_requests"},
		{"query_invalid", http.StatusUnprocessableEntity, "query_invalid"},
	}
	for _, tt := range tests {
		rec := get(t, &fakeReader{err: &tools.ReadError{Code: tt.code, Message: "m"}}, "/datasources/ch/schema")
		want := `{"code":"` + tt.want + `","error":"m"}`
		if rec.Code != tt.status || rec.Body.String() != want {
			t.Errorf("%s: status %d, body %s", tt.code, rec.Code, rec.Body)
		}
	}
}
