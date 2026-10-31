package egress_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actions"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func init() { gin.SetMode(gin.TestMode) }

const actionsDashboardID = "0a1b2c3d-0000-4000-8000-000000000002"

type savedDashboard struct{ dash *spec.Dashboard }

func (savedDashboard) LatestVersion(context.Context, pgtype.UUID) (int, error) { return 3, nil }

func (s savedDashboard) VersionSpec(context.Context, pgtype.UUID, int) (*spec.Dashboard, error) {
	return s.dash, nil
}

type listed []string

func (l listed) Allowed(context.Context) (domains.List, error) {
	var list domains.List
	for _, entry := range l {
		p, err := domains.ParsePattern(entry)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

type landed struct{ Host, Path, Body string }

type landings struct {
	mu   sync.Mutex
	seen []landed
}

func (l *landings) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	l.mu.Lock()
	l.seen = append(l.seen, landed{Host: r.Host, Path: r.URL.Path, Body: string(body)})
	l.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (l *landings) all() []landed {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]landed(nil), l.seen...)
}

func canvasWithTwoActions() *spec.Dashboard {
	return &spec.Dashboard{
		Variables: []spec.Variable{{Name: "env", Type: spec.CustomVariable}},
		Panels: []spec.Panel{{
			ID:   "flow",
			Type: spec.Canvas,
			Display: &spec.Display{Elements: []spec.CanvasElement{{
				ID: "restart",
				Actions: []spec.Action{
					{Title: "Restart", URL: "https://feed.test/restart/${env}", Body: `{"who":"${__user.email}"}`},
					{Title: "Restart elsewhere", URL: "https://nope.test/restart/${env}"},
				},
			}}},
		}},
	}
}

func postAction(t *testing.T, allow egress.Allowlist, action int) (*httptest.ResponseRecorder, *landings, func() int64, *logtest.Capture) {
	t.Helper()
	reached := &landings{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", reached.handler)
	client, hits := egress.NewTestClient(t, allow, mux)

	logger, logs := logtest.New()
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		p := auth.Principal{Kind: auth.KindUser, ID: "user-1", Email: "ada@example.com", Name: "Ada", Role: auth.RoleEditor}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
	})
	actions.NewHandler(savedDashboard{canvasWithTwoActions()}, client, func(c *gin.Context) { c.Next() }, logger).Register(auth.NewRoutes(api))

	body, err := json.Marshal(map[string]any{
		"version": 3, "panel": "flow", "element": "restart", "action": action,
		"variables": map[string][]string{"env": {"prod"}},
		"from":      "2026-11-01T10:00:00Z", "to": "2026-11-01T11:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/"+actionsDashboardID+"/actions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec, reached, hits, logs
}

func TestActionToAnAllowlistedHostIsSentByTheServer(t *testing.T) {
	rec, reached, hits, logs := postAction(t, listed{"feed.test"}, 0)

	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":204,"ok":true}` {
		t.Fatalf("response = %d %s, want 200 with status 204 and ok", rec.Code, rec.Body)
	}
	want := landed{Host: "feed.test", Path: "/restart/prod", Body: `{"who":"ada@example.com"}`}
	if got := reached.all(); len(got) != 1 || got[0] != want {
		t.Errorf("upstream saw %+v, want exactly %+v", got, want)
	}
	if hits() != 1 {
		t.Errorf("upstream connections = %d, want 1", hits())
	}
	record := logs.Find(t, "action run")
	if record["host"] != "feed.test" || record["outcome"] != "ok" || record["status"] != float64(204) {
		t.Errorf("audit = %v, want host feed.test, outcome ok, status 204", record)
	}
}

func TestActionToAHostOffTheAllowlistIsRefusedAndNothingIsSent(t *testing.T) {
	type refusal struct{ Error, Code, Host string }
	cases := []struct {
		name  string
		allow listed
		index int
		host  string
	}{
		{"host not on the list", listed{"feed.test"}, 1, "nope.test"},
		{"empty allowlist", nil, 0, "feed.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, reached, hits, logs := postAction(t, tc.allow, tc.index)

			var got refusal
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("body %q: %v", rec.Body, err)
			}
			want := refusal{tc.host + " is not on the outbound allowlist.", "outbound_domain_not_allowed", tc.host}
			if rec.Code != http.StatusForbidden || got != want {
				t.Fatalf("response = %d %+v, want 403 %+v", rec.Code, got, want)
			}
			if hits() != 0 || len(reached.all()) != 0 {
				t.Errorf("the upstream was reached: %d connections, %+v", hits(), reached.all())
			}
			record := logs.Find(t, "action run")
			if record["host"] != tc.host || record["outcome"] != "refused" || record["status"] != float64(0) {
				t.Errorf("audit = %v, want host %s, outcome refused, status 0", record, tc.host)
			}
		})
	}
}
