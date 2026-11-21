package revisions

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
)

func TestRunAllowed(t *testing.T) {
	withEdit := folders.Abilities{View: true, Comment: true, Edit: true}
	withoutEdit := folders.Abilities{View: true, Comment: true}
	viewOnly := folders.Abilities{View: true}
	dash := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	obs := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	apply := RunTarget{Intent: IntentApply, DashboardID: dash}
	suggest := RunTarget{Intent: IntentSuggest, DashboardID: dash}
	create := RunTarget{Intent: IntentAsk}
	files := RunTarget{Intent: IntentAsk, Files: true}
	filesOnDashboard := RunTarget{Intent: IntentAsk, DashboardID: dash, Files: true}
	onObservation := RunTarget{Intent: IntentAsk, ObservationID: obs}
	onDashboard := RunTarget{Intent: IntentAsk, DashboardID: dash}

	cases := []struct {
		name string
		rule runRule
		want bool
	}{
		{"apply with Edit", runRule{target: apply, role: auth.RoleEditor, dashboard: withEdit}, true},
		{"apply with Comment only", runRule{target: apply, role: auth.RoleCommenter, dashboard: withoutEdit}, true},
		{"apply with View only", runRule{target: apply, role: auth.RoleEditor, dashboard: viewOnly}, false},
		{"apply without a dashboard", runRule{target: RunTarget{Intent: IntentApply}, role: auth.RoleEditor, canCreate: true}, false},
		{"suggest with Edit", runRule{target: suggest, role: auth.RoleEditor, dashboard: withEdit}, true},
		{"suggest without Edit", runRule{target: suggest, role: auth.RoleEditor, dashboard: withoutEdit}, false},
		{"a new-dashboard ask when the requester can create", runRule{target: create, role: auth.RoleEditor, canCreate: true}, true},
		{"a new-dashboard ask when the requester cannot create", runRule{target: create, role: auth.RoleEditor}, false},
		{"Edit abilities do not help a new-dashboard ask", runRule{target: create, role: auth.RoleEditor, dashboard: withEdit}, false},
		{"an investigation by a Viewer", runRule{target: files, role: auth.RoleViewer, canCreate: true}, false},
		{"an investigation by an Editor", runRule{target: files, role: auth.RoleEditor}, true},
		{"an investigation by a Commenter of the dashboard", runRule{target: filesOnDashboard, role: auth.RoleCommenter, dashboard: withoutEdit}, false},
		{"an investigation by an Editor who can view the dashboard", runRule{target: filesOnDashboard, role: auth.RoleEditor, dashboard: viewOnly}, true},
		{"an investigation by an Editor with no access to the dashboard", runRule{target: filesOnDashboard, role: auth.RoleEditor}, false},
		{"an ask on an observation by a Commenter", runRule{target: onObservation, role: auth.RoleCommenter, observation: withoutEdit}, true},
		{"an ask on an observation by an Editor who may only view it", runRule{target: onObservation, role: auth.RoleEditor, observation: viewOnly}, false},
		{"an ask on a dashboard by a Commenter", runRule{target: onDashboard, role: auth.RoleCommenter, dashboard: withoutEdit}, true},
		{"an ask on a dashboard with View only", runRule{target: onDashboard, role: auth.RoleEditor, dashboard: viewOnly}, false},
		{"a run with no intent", runRule{target: RunTarget{DashboardID: dash}, role: auth.RoleAdmin, dashboard: withEdit, canCreate: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runAllowed(tc.rule); got != tc.want {
				t.Fatalf("runAllowed(%+v) = %v, want %v", tc.rule, got, tc.want)
			}
		})
	}
}

func TestRefusalNamesWhatWouldLetTheRequestThrough(t *testing.T) {
	dash := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	obs := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	cases := []struct {
		name string
		rule runRule
		want forbidden.Body
	}{
		{"an investigation by a Viewer", runRule{target: RunTarget{Intent: IntentAsk, Files: true}, role: auth.RoleViewer}, forbidden.NeedsRole("editor")},
		{"an investigation by a Commenter on a dashboard", runRule{target: RunTarget{Intent: IntentAsk, DashboardID: dash, Files: true}, role: auth.RoleCommenter}, forbidden.NeedsRole("editor")},
		{"an ask on an observation with View only", runRule{target: RunTarget{Intent: IntentAsk, ObservationID: obs}, role: auth.RoleViewer}, forbidden.NeedsAccess("comment", "Payments")},
		{"an ask on a dashboard with View only", runRule{target: RunTarget{Intent: IntentAsk, DashboardID: dash}, role: auth.RoleViewer}, forbidden.NeedsAccess("comment", "Payments")},
		{"suggest without Edit", runRule{target: RunTarget{Intent: IntentSuggest, DashboardID: dash}, role: auth.RoleCommenter}, forbidden.NeedsAccess("edit", "Payments")},
		{"apply with View only", runRule{target: RunTarget{Intent: IntentApply, DashboardID: dash}, role: auth.RoleViewer}, forbidden.NeedsAccess("comment", "Payments")},
		{"a new-dashboard ask without a folder to create in", runRule{target: RunTarget{Intent: IntentAsk}, role: auth.RoleViewer}, forbidden.NeedsAccess("edit", "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := refusal(tc.rule, "Payments")
			if !errors.Is(err, ErrForbidden) || forbidden.BodyOf(err) != tc.want {
				t.Fatalf("refusal = %#v, want a forbidden error with %+v", err, tc.want)
			}
		})
	}
}

func TestFailAnswersAForbiddenRunWithItsNeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	(&Service{logger: slog.New(slog.DiscardHandler)}).fail(c, "enqueue", refusal(runRule{target: RunTarget{Intent: IntentAsk, Files: true}, role: auth.RoleViewer}, ""))
	want := `{"error":"your role does not allow this","code":"forbidden_role","needed_role":"editor"}`
	if rec.Code != http.StatusForbidden || rec.Body.String() != want {
		t.Fatalf("got %d %s, want 403 %s", rec.Code, rec.Body.String(), want)
	}
}
