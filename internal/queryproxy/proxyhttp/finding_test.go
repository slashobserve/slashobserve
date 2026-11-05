package proxyhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type fakeFindings struct {
	mu    sync.Mutex
	spec  *spec.Dashboard
	err   error
	calls int
}

func (f *fakeFindings) EvidenceDashboard(context.Context, auth.Principal, pgtype.UUID) (*spec.Dashboard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.spec, f.err
}

func (f *fakeFindings) lookups() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (hn *harness) findingRouter(f FindingSpecs) *gin.Engine {
	router := gin.New()
	api := router.Group("/api/v1", func(c *gin.Context) {
		principal := auth.Principal{Kind: auth.PrincipalKind(c.GetHeader("X-Kind")), ID: "alice", Email: "alice@example.com", Name: "alice", Role: auth.RoleViewer}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), principal))
	})
	NewFindingData(hn.h, f).Register(auth.NewRoutes(api))
	return router
}

func findingGet(router *gin.Engine, path string, kind auth.PrincipalKind) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Kind", string(kind))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestFindingEvidenceDataIsForUserSessions(t *testing.T) {
	hn := newHarness(t)
	f := &fakeFindings{spec: testDashboard()}
	router := hn.findingRouter(f)
	path := "/api/v1/findings/" + hn.id.String() + "/panels/p_ts/data?" + window
	for _, kind := range []auth.PrincipalKind{auth.KindServiceAccount, ""} {
		expectError(t, findingGet(router, path, kind), http.StatusForbidden, "user_session_required")
	}
	if f.lookups() != 0 || hn.heldSlots() != 0 {
		t.Fatalf("refused calls read the finding %d times and hold %d slots", f.lookups(), hn.heldSlots())
	}
	for range 2 {
		if body := decodeData(t, findingGet(router, path, auth.KindUser)); body.Meta.Cached {
			t.Fatal("evidence data came from the cache")
		}
	}
}

func TestFindingEvidenceDataRefusals(t *testing.T) {
	hn := newHarness(t)
	hidden := hn.findingRouter(&fakeFindings{err: findings.ErrNotFound})
	visible := hn.findingRouter(&fakeFindings{spec: testDashboard()})
	cases := []struct {
		name   string
		router *gin.Engine
		path   string
		status int
		code   string
	}{
		{"a hidden finding", hidden, "/api/v1/findings/" + hn.id.String() + "/panels/p_ts/data?" + window, http.StatusNotFound, "not_found"},
		{"an unknown panel", visible, "/api/v1/findings/" + hn.id.String() + "/panels/missing/data?" + window, http.StatusNotFound, "panel_not_found"},
		{"a bad id", visible, "/api/v1/findings/nope/panels/p_ts/data?" + window, http.StatusBadRequest, "invalid_request"},
		{"query text", visible, "/api/v1/findings/" + hn.id.String() + "/panels/p_ts/data?query=up&" + window, http.StatusForbidden, "query_not_in_spec"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expectError(t, findingGet(c.router, c.path, auth.KindUser), c.status, c.code)
		})
	}
	if hn.heldSlots() != 0 {
		t.Fatalf("%d slots held after the refusals", hn.heldSlots())
	}
}
