package activity

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r rig) get(t *testing.T, who auth.Principal, query string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	})
	NewHandler(r.store, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/activity?"+query, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return rec.Code, body
}

func seqsInBody(t *testing.T, body map[string]any) []int64 {
	t.Helper()
	list, ok := body["activity"].([]any)
	if !ok {
		t.Fatalf("no activity array in %v", body)
	}
	out := make([]int64, 0, len(list))
	for _, item := range list {
		out = append(out, int64(item.(map[string]any)["seq"].(float64)))
	}
	return out
}

func TestListServesPagesWithACursor(t *testing.T) {
	r := newRig(t)
	d := r.dashboardIn(t, pgtype.UUID{})
	first := r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: d, Data: map[string]any{"title": "Checkout"}})
	second := r.append(t, Entry{Type: TypeDashboardVersionCreated, DashboardID: d})
	third := r.append(t, Entry{Type: TypeDashboardMoved, DashboardID: d})

	code, page := r.get(t, plain, "limit=2")

	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, page)
	}
	if got := seqsInBody(t, page); !slices.Equal(got, []int64{third, second}) {
		t.Fatalf("first page %v, want [%d %d]", got, third, second)
	}
	cursor, ok := page["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("next_cursor = %v, want a cursor", page["next_cursor"])
	}

	code, last := r.get(t, plain, "limit=2&cursor="+url.QueryEscape(cursor))

	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, last)
	}
	if got := seqsInBody(t, last); !slices.Equal(got, []int64{first}) {
		t.Fatalf("second page %v, want [%d]", got, first)
	}
	if last["next_cursor"] != nil {
		t.Fatalf("next_cursor = %v, want null", last["next_cursor"])
	}
}

func TestListShapesAnEvent(t *testing.T) {
	r := newRig(t)
	d := r.dashboardIn(t, pgtype.UUID{})
	r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: d, TargetID: d, Data: map[string]any{"version": 1}})

	code, body := r.get(t, plain, "")

	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, body)
	}
	events := body["activity"].([]any)
	if len(events) != 1 {
		t.Fatalf("events %v", events)
	}
	event := events[0].(map[string]any)
	target := event["target"].(map[string]any)
	who := event["actor"].(map[string]any)
	data := event["data"].(map[string]any)
	switch {
	case event["type"] != TypeDashboardCreated,
		target["kind"] != "dashboard", target["id"] != d.String(),
		event["dashboard_id"] != d.String(), event["folder_id"] == nil,
		who["id"] != editorActor.ID, who["label"] != "Eve", who["name"] != "Eve",
		data["version"] != float64(1),
		event["created_at"] == "":
		t.Fatalf("event = %v", event)
	}
}

func TestListRefusesBadParameters(t *testing.T) {
	r := newRig(t)
	tests := []struct {
		name  string
		query string
	}{
		{"limit of zero", "limit=0"},
		{"limit above the ceiling", "limit=101"},
		{"limit that is not a number", "limit=many"},
		{"dashboard that is not a UUID", "dashboard_id=checkout"},
		{"folder that is not a UUID", "folder_id=general"},
		{"unknown type", "type=dashboard.renamed"},
		{"time that is not RFC 3339", "since=yesterday"},
		{"cursor that is not a cursor", "cursor=%21%21"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := r.get(t, plain, tt.query)

			if code != http.StatusBadRequest || body["code"] != "invalid_request" {
				t.Fatalf("status %d body %v, want 400 invalid_request", code, body)
			}
		})
	}
}

func TestListFiltersByTypeAndDashboard(t *testing.T) {
	r := newRig(t)
	a := r.dashboardIn(t, pgtype.UUID{})
	b := r.dashboardIn(t, pgtype.UUID{})
	r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: a})
	wanted := r.append(t, Entry{Type: TypeDashboardMoved, DashboardID: b})
	r.append(t, Entry{Type: TypeDashboardMoved, DashboardID: a})

	code, body := r.get(t, plain, "dashboard_id="+b.String()+"&type=dashboard.moved&type=dashboard.deleted")

	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, body)
	}
	if got := seqsInBody(t, body); !slices.Equal(got, []int64{wanted}) {
		t.Fatalf("got %v, want [%d]", got, wanted)
	}
}

func TestListAppliesTheAudience(t *testing.T) {
	r := newRig(t)
	open := r.dashboardIn(t, r.openFolder(t, "Open"))
	restricted := r.dashboardIn(t, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))
	inOpen := r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: open})
	inRestricted := r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: restricted})
	readersOwn := r.append(t, Entry{Type: TypeFindingCreated, Actor: editorActor})
	tests := []struct {
		name string
		who  auth.Principal
		want []int64
	}{
		{"a viewer", plain, []int64{inOpen}},
		{"a member of the restricted folder", payroll, []int64{inOpen, inRestricted}},
		{"an admin", admin, []int64{inOpen, inRestricted, readersOwn}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := r.get(t, tt.who, "")

			if code != http.StatusOK {
				t.Fatalf("status %d: %v", code, body)
			}
			if got := sorted(seqsInBody(t, body)); !slices.Equal(got, sorted(tt.want)) {
				t.Fatalf("got %v, want %v", got, sorted(tt.want))
			}
		})
	}
}

func TestListRefusesAPrincipalWithoutAUsableID(t *testing.T) {
	r := newRig(t)

	code, body := r.get(t, auth.Principal{Kind: auth.KindUser, ID: "not-a-uuid", Role: auth.RoleViewer}, "")

	if code != http.StatusForbidden || body["code"] != "forbidden_role" {
		t.Fatalf("status %d body %v, want 403 forbidden_role", code, body)
	}
}
