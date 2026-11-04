package revisions

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
)

func TestBuildStateMapsEveryField(t *testing.T) {
	at := time.Date(2026, 11, 3, 10, 30, 0, 0, time.UTC)
	next := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	state := newBuildState(dbsqlc.ListAgentRequestBuildStateRow{
		Saves:         pgtype.Int4{Int32: 3, Valid: true},
		Panels:        pgtype.Int4{Int32: 14, Valid: true},
		PlannedPanels: pgtype.Int4{Int32: 40, Valid: true},
		Title:         "Node Exporter",
		UpdatedAt:     pgtype.Timestamptz{Time: at, Valid: true},
		ContinuedBy:   next,
	})
	got, err := json.Marshal(state.build)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"saves":3,"panels":14,"planned_panels":40,"title":"Node Exporter","updated_at":"2026-11-03T10:30:00Z"}`
	if string(got) != want {
		t.Fatalf("progress = %s, want %s", got, want)
	}
	if state.continuedBy == nil || *state.continuedBy != next.String() {
		t.Fatalf("continuedBy = %v, want %s", state.continuedBy, next.String())
	}
}

func TestBuildStateWithoutABuildOrContinuationIsEmpty(t *testing.T) {
	state := newBuildState(dbsqlc.ListAgentRequestBuildStateRow{})
	if state.build != nil || state.continuedBy != nil {
		t.Fatalf("state = %+v, want no build and no continuation", state)
	}
}

func TestRequestAndRevisionJSONReportNullControlFields(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		fields []string
	}{
		{"request", agentRequestJSON{}, []string{"build", "build_saved_at", "queue_position", "cancel_requested_at", "canceled_by", "continues_id", "continue_note", "continued_by"}},
		{"summary", requestSummaryJSON{}, []string{"failure_code", "build", "build_saved_at", "continues_id", "continued_by", "continue_note"}},
		{"revision", revisionJSON{}, []string{"failure_code", "build", "build_saved_at", "continues_id", "continued_by", "continue_note"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(got, &fields); err != nil {
				t.Fatal(err)
			}
			for _, name := range tc.fields {
				if raw, ok := fields[name]; !ok || string(raw) != "null" {
					t.Fatalf("%s = %s (present %t), want null", name, raw, ok)
				}
			}
		})
	}
	got, err := json.Marshal(agentRequestJSON{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"can_cancel":false`) {
		t.Fatalf("can_cancel missing from %s", got)
	}
}

func TestBuildJSONNamesTheTargetOnlyForARevision(t *testing.T) {
	create := newBuildJSON(BuildView{RequestID: "r1", Saves: 2, Spec: json.RawMessage(`{"title":"x"}`), UpdatedAt: "2026-11-03T10:30:00Z"})
	if create.DashboardID != nil || create.BaseVersion != nil {
		t.Fatalf("a new-dashboard build names a target: %+v", create)
	}
	revise := newBuildJSON(BuildView{RequestID: "r2", DashboardID: "d1", BaseVersion: 4, Panels: 7, PlannedPanels: 9})
	if revise.DashboardID == nil || *revise.DashboardID != "d1" || revise.BaseVersion == nil || *revise.BaseVersion != 4 {
		t.Fatalf("a revision build lost its target: %+v", revise)
	}
	if revise.Panels != 7 || revise.PlannedPanels != 9 {
		t.Fatalf("counts = %+v", revise)
	}
}

func TestBuildErrorsMapToStatusesAndCodes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantLatest int
	}{
		{"no build", ErrBuildNotFound, http.StatusNotFound, "not_found", 0},
		{"request hidden", ErrRequestNotFound, http.StatusNotFound, "not_found", 0},
		{"dashboard gone", ErrDashboardNotFound, http.StatusNotFound, "not_found", 0},
		{"folder not found", folders.ErrNotFound, http.StatusNotFound, "not_found", 0},
		{"still running", ErrBuildRunning, http.StatusConflict, "build_running", 0},
		{"empty build", ErrBuildEmpty, http.StatusConflict, "build_empty", 0},
		{"not savable", ErrBuildNotSavable, http.StatusConflict, "build_not_savable", 0},
		{"not continuable", ErrNotContinuable, http.StatusConflict, "not_continuable", 0},
		{"request finished", ErrRequestFinished, http.StatusConflict, "request_finished", 0},
		{"not the requester", ErrNotRequester, http.StatusForbidden, "forbidden", 0},
		{"newer version", fmt.Errorf("save: %w", &BuildConflictError{Latest: 6}), http.StatusConflict, "version_conflict", 6},
		{"role too low", folders.ErrForbidden, http.StatusForbidden, forbidden.Code, 0},
		{"no folder access", folders.ErrNoAccess, http.StatusForbidden, forbidden.Code, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reply, ok := buildReplyFor(tc.err)
			if !ok || reply.status != tc.wantStatus || reply.code != tc.wantCode || reply.latest != tc.wantLatest {
				t.Fatalf("reply = %+v (handled %t), want status %d code %q latest %d", reply, ok, tc.wantStatus, tc.wantCode, tc.wantLatest)
			}
		})
	}
	if _, ok := buildReplyFor(errors.New("database down")); ok {
		t.Fatal("an unknown error must fall through to the service's 500 handling")
	}
}

func TestNoAccessAndRoleAreToldApart(t *testing.T) {
	access, _ := buildReplyFor(folders.ErrNoAccess)
	role, _ := buildReplyFor(folders.ErrForbidden)
	if access.message != forbidden.AccessMessage || role.message != forbidden.RoleMessage {
		t.Fatalf("messages = %q and %q", access.message, role.message)
	}
}

func TestBuildTitleIsHiddenFromEveryoneButTheRequester(t *testing.T) {
	requester := auth.Principal{Kind: auth.KindUser, ID: "u1"}
	state := newBuildState(dbsqlc.ListAgentRequestBuildStateRow{
		Saves:     pgtype.Int4{Int32: 1, Valid: true},
		Panels:    pgtype.Int4{Int32: 3, Valid: true},
		Title:     "Check small",
		UpdatedAt: pgtype.Timestamptz{Time: time.Date(2026, 11, 3, 10, 30, 0, 0, time.UTC), Valid: true},
	})
	tests := []struct {
		name   string
		caller auth.Principal
		want   string
	}{
		{"requester", requester, `{"saves":1,"panels":3,"planned_panels":0,"title":"Check small","updated_at":"2026-11-03T10:30:00Z"}`},
		{"another user", auth.Principal{Kind: auth.KindUser, ID: "u2"}, `{"saves":1,"panels":3,"planned_panels":0,"title":null,"updated_at":"2026-11-03T10:30:00Z"}`},
		{"key with the requester's id", auth.Principal{Kind: auth.KindServiceAccount, ID: "u1"}, `{"saves":1,"panels":3,"planned_panels":0,"title":null,"updated_at":"2026-11-03T10:30:00Z"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rev := newRevisionJSON(Revision{RequestedBy: requester}, state, tc.caller)
			got, err := json.Marshal(rev.Build)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("build = %s, want %s", got, tc.want)
			}
		})
	}
	if state.build.Title == nil || *state.build.Title != "Check small" {
		t.Fatal("hiding the title for one caller must not change the shared state")
	}
}

func TestProgressForAnAbsentBuildStaysAbsent(t *testing.T) {
	if got := (buildState{}).progressFor(auth.Principal{ID: "u1"}, auth.Principal{ID: "u2"}); got != nil {
		t.Fatalf("progress = %+v, want nil", got)
	}
}
