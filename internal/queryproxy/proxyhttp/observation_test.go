package proxyhttp

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type fakePanels struct {
	mu    sync.Mutex
	spec  *spec.Dashboard
	err   error
	calls int
}

func (f *fakePanels) PanelDashboard(context.Context, auth.Principal, pgtype.UUID) (*spec.Dashboard, time.Time, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.spec, time.Time{}, time.Time{}, f.err
}

func (f *fakePanels) lookups() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func blockDashboard(query string) *spec.Dashboard {
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "Checkout 5xx",
		Time:        spec.TimeRange{From: "2025-10-09T08:00:00Z", To: "2025-10-09T14:00:00Z"},
		Panels: []spec.Panel{
			{ID: observations.BlockPanelID, Type: spec.Timeseries, Title: "5xx", Datasource: "prom", GridPos: spec.GridPos{W: 12, H: 8}, Queries: []spec.Query{{Query: query}}},
		},
	}
}

func (hn *harness) observationRouter(p ObservationPanels) *gin.Engine {
	router := gin.New()
	api := router.Group("/api/v1", func(c *gin.Context) {
		principal := auth.Principal{Kind: auth.PrincipalKind(c.GetHeader("X-Kind")), ID: "alice", Email: "alice@example.com", Name: "alice", Role: auth.RoleViewer}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), principal))
	})
	NewObservationData(hn.h, p).Register(auth.NewRoutes(api))
	return router
}

func TestObservationBlockDataIsForUserSessions(t *testing.T) {
	hn := newHarness(t)
	f := &fakePanels{spec: blockDashboard("up")}
	router := hn.observationRouter(f)
	path := "/api/v1/blocks/" + hn.id.String() + "/data?" + window
	for _, kind := range []auth.PrincipalKind{auth.KindServiceAccount, ""} {
		expectError(t, findingGet(router, path, kind), http.StatusForbidden, "user_session_required")
	}
	if f.lookups() != 0 || hn.heldSlots() != 0 {
		t.Fatalf("refused calls read the block %d times and hold %d slots", f.lookups(), hn.heldSlots())
	}
	for range 2 {
		if body := decodeData(t, findingGet(router, path, auth.KindUser)); body.Meta.Cached || len(body.Frames) == 0 {
			t.Fatalf("block data %+v", body.Meta)
		}
	}
	if got := hn.ds.lastQuery().Text; got != "up" {
		t.Errorf("ran %q", got)
	}
}

func TestObservationBlockDataDefaultsToTheBlocksRange(t *testing.T) {
	hn := newHarness(t)
	router := hn.observationRouter(&fakePanels{spec: blockDashboard("up")})
	decodeData(t, findingGet(router, "/api/v1/blocks/"+hn.id.String()+"/data", auth.KindUser))
	want := time.Date(2025, 10, 9, 8, 0, 0, 0, time.UTC)
	hn.ds.mu.Lock()
	got := hn.ds.rangeRange
	hn.ds.mu.Unlock()
	if d := got.From.Sub(want); d < -time.Minute || d > time.Minute || got.To.Sub(got.From) < 5*time.Hour {
		t.Errorf("ran %v to %v", got.From, got.To)
	}
}

func TestObservationBlockDataRefusals(t *testing.T) {
	hn := newHarness(t)
	hidden := hn.observationRouter(&fakePanels{err: observations.ErrNotFound})
	visible := hn.observationRouter(&fakePanels{spec: blockDashboard("up")})
	cases := []struct {
		name   string
		router *gin.Engine
		path   string
		status int
		code   string
	}{
		{"an invisible or non-panel block", hidden, "/api/v1/blocks/" + hn.id.String() + "/data?" + window, http.StatusNotFound, "not_found"},
		{"a bad id", visible, "/api/v1/blocks/nope/data?" + window, http.StatusBadRequest, "invalid_request"},
		{"query text", visible, "/api/v1/blocks/" + hn.id.String() + "/data?query=up&" + window, http.StatusForbidden, "query_not_in_spec"},
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
