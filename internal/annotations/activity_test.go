package annotations

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func (r rig) activityFor(t *testing.T, by auth.Principal) []activity.Event {
	t.Helper()
	store := activity.NewStore(r.db.DB, folders.NewAccess(r.db.DB))
	audience, err := store.Audience(t.Context(), by)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := store.Next(t.Context(), audience, activity.Filter{}, activity.Position{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (r rig) folderQuery(t *testing.T, query string, args ...any) string {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func (r rig) general(t *testing.T) string {
	t.Helper()
	return r.folderQuery(t, `SELECT general_folder_id()::text`)
}

func (r rig) folderOf(t *testing.T, dashboard pgtype.UUID) string {
	t.Helper()
	return r.folderQuery(t, `SELECT folder_id::text FROM dashboards WHERE id = $1`, dashboard)
}

func typesOf(events []activity.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func decodeJSON(t *testing.T, raw []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func sameJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	g, _ := json.Marshal(decodeJSON(t, got))
	w, _ := json.Marshal(decodeJSON(t, []byte(want)))
	if string(g) != string(w) {
		t.Errorf("data = %s, want %s", got, want)
	}
}

func mustParse(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestADashboardAnnotationLandsInTheDashboardsFolder(t *testing.T) {
	r := newRig(t)
	to := at(30)
	a := r.create(t, admin, CreateInput{DashboardID: r.hidden, PanelID: "p1", From: at(10), To: &to, Text: "secret payroll deploy", Tags: []string{"deploy"}})

	got := r.activityFor(t, payroll)
	if len(got) != 1 {
		t.Fatalf("payroll member sees %d events, want 1: %+v", len(got), got)
	}
	e := got[0]
	if e.Type != activity.TypeAnnotationCreated || e.TargetKind != "annotation" || e.TargetID != a.ID || e.DashboardID != r.hidden.String() {
		t.Errorf("event = %+v", e)
	}
	if want := r.folderOf(t, r.hidden); e.FolderID != want {
		t.Errorf("folder = %q, want the restricted folder %q", e.FolderID, want)
	}
	if e.Actor.ID != admin.ID || e.Actor.Name != "Root" {
		t.Errorf("actor = %+v", e.Actor)
	}
	sameJSON(t, e.Data, `{"panel_id":"p1","tags":["deploy"],"time_from":"2026-11-01T10:10:00Z","time_to":"2026-11-01T10:30:00Z"}`)
	if len(r.activityFor(t, viewer)) != 0 {
		t.Error("a viewer outside the restricted folder sees the annotation")
	}
}

func TestTheDataOfAnAnnotationOmitsUnsetFieldsAndNeverHoldsTheText(t *testing.T) {
	r := newRig(t)
	r.create(t, editor, CreateInput{DashboardID: r.open, From: at(0), Text: "a distinctive annotation body"})
	got := r.activityFor(t, viewer)
	if len(got) != 1 {
		t.Fatalf("events = %+v", got)
	}
	sameJSON(t, got[0].Data, `{"tags":[],"time_from":"2026-11-01T10:00:00Z"}`)
}

func TestAGlobalAnnotationLandsInGeneralAndIsVisibleToAViewer(t *testing.T) {
	r := newRig(t)
	a := r.create(t, editor, CreateInput{From: at(0), Text: "release train"})
	for _, by := range []auth.Principal{viewer, payroll, admin} {
		got := r.activityFor(t, by)
		if len(got) != 1 || got[0].TargetID != a.ID || got[0].DashboardID != "" {
			t.Fatalf("%s sees %+v", by.Name, got)
		}
		if want := r.general(t); got[0].FolderID != want {
			t.Errorf("%s: folder = %q, want General %q", by.Name, got[0].FolderID, want)
		}
	}
}

func TestDeletingAnAnnotationAppendsOneRowWithItsDashboard(t *testing.T) {
	r := newRig(t)
	onDashboard := r.create(t, admin, CreateInput{DashboardID: r.hidden, From: at(0), Text: "one"})
	global := r.create(t, editor, CreateInput{From: at(1), Text: "two"})
	if err := r.service.Delete(t.Context(), admin, mustParse(t, onDashboard.ID)); err != nil {
		t.Fatal(err)
	}
	if err := r.service.Delete(t.Context(), editor, mustParse(t, global.ID)); err != nil {
		t.Fatal(err)
	}

	got := r.activityFor(t, admin)
	want := []string{activity.TypeAnnotationCreated, activity.TypeAnnotationCreated, activity.TypeAnnotationDeleted, activity.TypeAnnotationDeleted}
	if !slices.Equal(typesOf(got), want) {
		t.Fatalf("admin sees %v, want %v", typesOf(got), want)
	}
	deleted := got[2]
	if deleted.TargetID != onDashboard.ID || deleted.DashboardID != r.hidden.String() || deleted.FolderID != r.folderOf(t, r.hidden) {
		t.Errorf("dashboard annotation delete = %+v", deleted)
	}
	if string(deleted.Data) != "{}" || deleted.Actor.ID != admin.ID {
		t.Errorf("delete data %s actor %+v", deleted.Data, deleted.Actor)
	}
	deletedGlobal := got[3]
	if deletedGlobal.TargetID != global.ID || deletedGlobal.FolderID != r.general(t) {
		t.Errorf("global annotation delete = %+v", deletedGlobal)
	}
}

func TestADeleteThatFindsNothingAppendsNothing(t *testing.T) {
	r := newRig(t)
	r.create(t, editor, CreateInput{From: at(0), Text: "kept"})
	if err := r.service.Delete(t.Context(), editor, mustParse(t, "99999999-9999-4999-8999-999999999999")); err == nil {
		t.Fatal("deleting a missing annotation succeeded")
	}
	if got := typesOf(r.activityFor(t, admin)); !slices.Equal(got, []string{activity.TypeAnnotationCreated}) {
		t.Errorf("activity = %v", got)
	}
}

func TestAnAnnotationOnAMissingDashboardAppendsNothing(t *testing.T) {
	r := newRig(t)
	_, err := r.service.Create(t.Context(), admin, CreateInput{DashboardID: mustParse(t, "99999999-9999-4999-8999-999999999999"), From: at(0), Text: "lost"})
	if err == nil {
		t.Fatal("annotating a missing dashboard succeeded")
	}
	if got := r.activityFor(t, admin); len(got) != 0 {
		t.Errorf("activity = %v", typesOf(got))
	}
}
