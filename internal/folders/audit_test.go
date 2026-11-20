package folders

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

type storedEvent struct {
	Action, Outcome, ActorKind, ActorID, ActorName, ActorRole string
	TargetKind, TargetID, TargetName, FolderID, RequestID     string
	Data                                                      map[string]any
}

func storedEvents(t *testing.T, d *dbtest.Database) []storedEvent {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(t.Context(), `SELECT action, outcome, actor_kind, coalesce(actor_id::text, ''), coalesce(actor_name, ''), coalesce(actor_role, ''),
  coalesce(target_kind, ''), coalesce(target_id, ''), coalesce(target_name, ''), coalesce(folder_id::text, ''), coalesce(request_id::text, ''), data::text
FROM audit_events ORDER BY occurred_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []storedEvent
	for rows.Next() {
		var e storedEvent
		var data string
		if err := rows.Scan(&e.Action, &e.Outcome, &e.ActorKind, &e.ActorID, &e.ActorName, &e.ActorRole, &e.TargetKind, &e.TargetID, &e.TargetName, &e.FolderID, &e.RequestID, &data); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func send(t *testing.T, engine http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func onlyEvent(t *testing.T, d *dbtest.Database, request *httptest.ResponseRecorder) storedEvent {
	t.Helper()
	return onlyEventAfter(t, d, 0, request)
}

func onlyEventAfter(t *testing.T, d *dbtest.Database, earlier int, request *httptest.ResponseRecorder) storedEvent {
	t.Helper()
	events := storedEvents(t, d)[earlier:]
	if len(events) != 1 {
		t.Fatalf("events = %+v, want exactly one", events)
	}
	if events[0].RequestID != request.Header().Get("X-Request-Id") {
		t.Fatalf("event request id = %q, want the response's %q", events[0].RequestID, request.Header().Get("X-Request-Id"))
	}
	return events[0]
}

func wantFolderEvent(t *testing.T, got storedEvent, action, folderID, title string, keys ...string) {
	t.Helper()
	if got.Action != action || got.Outcome != "success" || got.ActorKind != "service_account" || got.ActorID != testAdmin.ID || got.ActorName != "root" || got.ActorRole != "admin" {
		t.Fatalf("event = %+v, want a successful %s by %s", got, action, testAdmin.ID)
	}
	if got.TargetKind != "folder" || got.TargetID != folderID || got.TargetName != title || got.FolderID != folderID {
		t.Fatalf("target = %s %s %q in folder %s, want folder %s %q", got.TargetKind, got.TargetID, got.TargetName, got.FolderID, folderID, title)
	}
	if !slices.Equal(keysOf(got.Data), keys) {
		t.Fatalf("data keys = %v, want %v", keysOf(got.Data), keys)
	}
}

func TestCreatingAFolderRecordsFolderCreate(t *testing.T) {
	d := dbtest.New(t)
	rec := send(t, adminEngine(t, d), http.MethodPost, "/api/v1/folders", `{"title":"Payments"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var body struct{ ID string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	wantFolderEvent(t, onlyEvent(t, d, rec), "folder.create", body.ID, "Payments")
}

func TestRenamingAFolderRecordsTheOldAndNewTitle(t *testing.T) {
	d := dbtest.New(t)
	id := newFolder(t, d, "Before")
	rec := send(t, adminEngine(t, d), http.MethodPatch, "/api/v1/folders/"+id, `{"title":"After"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename = %d %s", rec.Code, rec.Body)
	}
	got := onlyEvent(t, d, rec)
	wantFolderEvent(t, got, "folder.rename", id, "After", "from_title", "title")
	if got.Data["from_title"] != "Before" || got.Data["title"] != "After" {
		t.Fatalf("data = %v, want Before to After", got.Data)
	}
}

func TestDeletingAFolderRecordsFolderDeleteAndARefusedDeleteRecordsNone(t *testing.T) {
	d := dbtest.New(t)
	held := newFolder(t, d, "Holds a search")
	d.Exec(t, `INSERT INTO saved_searches (folder_id, kind, title, datasource_uid, query, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 'logs', 't', 'loki', '{app="x"}', 'now-1h', 'now', 'user', gen_random_uuid(), 'Ada')`, held)
	if rec := deleteFolder(t, d, held); rec.Code != http.StatusConflict {
		t.Fatalf("delete of a held folder = %d", rec.Code)
	}
	refused := storedEvents(t, d)
	for _, e := range refused {
		if e.Outcome == "success" {
			t.Fatalf("a refused delete recorded a success: %+v", e)
		}
	}

	empty := newFolder(t, d, "Empty")
	rec := deleteFolder(t, d, empty)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}
	wantFolderEvent(t, onlyEventAfter(t, d, len(refused), rec), "folder.delete", empty, "Empty")
}

func TestSettingAFolderPermissionsRecordsWhatWasAddedRemovedAndChanged(t *testing.T) {
	d := dbtest.New(t)
	user, err := d.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "bo", Email: "bo@example.com", Name: "Bo"})
	if err != nil {
		t.Fatal(err)
	}
	id := newFolder(t, d, "Locked")
	engine := adminEngine(t, d)
	put := func(entries string) storedEvent {
		earlier := len(storedEvents(t, d))
		rec := send(t, engine, http.MethodPut, "/api/v1/folders/"+id+"/permissions", `{"entries":`+entries+`}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("set = %d %s", rec.Code, rec.Body)
		}
		got := onlyEventAfter(t, d, earlier, rec)
		wantFolderEvent(t, got, "folder.permissions_change", id, "Locked", "added", "changed", "count_after", "count_before", "removed", "truncated")
		return got
	}
	sre := map[string]any{"kind": "group", "name": "sre", "level": "edit"}
	bo := func(level string) map[string]any {
		return map[string]any{"kind": "user", "id": user.ID.String(), "name": "Bo", "level": level}
	}

	first := put(`[{"group":"sre","level":"edit"}]`)
	want := map[string]any{"added": []any{sre}, "removed": []any{}, "changed": []any{}, "count_before": 0.0, "count_after": 1.0, "truncated": false}
	if !reflect.DeepEqual(first.Data, want) {
		t.Fatalf("first = %v, want %v", first.Data, want)
	}

	second := put(`[{"user_id":"` + user.ID.String() + `","level":"view"}]`)
	want = map[string]any{"added": []any{bo("view")}, "removed": []any{sre}, "changed": []any{}, "count_before": 1.0, "count_after": 1.0, "truncated": false}
	if !reflect.DeepEqual(second.Data, want) {
		t.Fatalf("second = %v, want %v", second.Data, want)
	}

	third := put(`[{"user_id":"` + user.ID.String() + `","level":"edit"}]`)
	changed := bo("edit")
	changed["from_level"] = "view"
	want = map[string]any{"added": []any{}, "removed": []any{}, "changed": []any{changed}, "count_before": 1.0, "count_after": 1.0, "truncated": false}
	if !reflect.DeepEqual(third.Data, want) {
		t.Fatalf("third = %v, want %v", third.Data, want)
	}
	if raw, _ := json.Marshal(third.Data); strings.Contains(string(raw), "example.com") {
		t.Fatal("an email address reached the audit event")
	}
}

func TestAFailedPermissionChangeLeavesNoEventAndNoChange(t *testing.T) {
	d := dbtest.New(t)
	id := newFolder(t, d, "Rolls back")
	boom := errors.New("settling failed")
	h := NewHandler(d.DB, NewAccess(d.DB), slog.New(slog.DiscardHandler)).SettleSharingWith(func(context.Context, *dbsqlc.Queries, auth.Principal, pgtype.UUID) error { return boom })
	rec := send(t, adminEngineWith(t, d, h), http.MethodPut, "/api/v1/folders/"+id+"/permissions", `{"entries":[{"group":"sre","level":"edit"}]}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("set = %d %s, want 500", rec.Code, rec.Body)
	}
	for _, e := range storedEvents(t, d) {
		if e.Outcome == "success" {
			t.Fatalf("a rolled back change left a success event: %+v", e)
		}
	}
	var n int
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM folder_permissions WHERE folder_id = $1`, id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("permission rows = %d, %v; want 0", n, err)
	}
}

func TestCreatingAnImportFolderRecordsFolderCreate(t *testing.T) {
	d := dbtest.New(t)
	access := NewAccess(d.DB)
	editor := auth.Principal{Kind: auth.KindServiceAccount, ID: testAdmin.ID, Name: "root", Role: auth.RoleAdmin}
	out, err := access.ForImport(t.Context(), editor, "Imported")
	if err != nil || !out.Created {
		t.Fatalf("import folder = %+v, %v; want a created folder", out, err)
	}
	events := storedEvents(t, d)
	if len(events) != 1 || events[0].Action != "folder.create" || events[0].TargetName != "Imported" || events[0].TargetID != out.Folder.ID.String() {
		t.Fatalf("events = %+v, want one folder.create for Imported", events)
	}
}

func TestAGrantToAPersonWithoutANameCarriesTheIdAndNeverTheEmail(t *testing.T) {
	d := dbtest.New(t)
	user, err := d.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "nameless", Email: "nameless@example.com", Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	id := newFolder(t, d, "Locked")
	engine := adminEngine(t, d)
	earlier := len(storedEvents(t, d))

	rec := send(t, engine, http.MethodPut, "/api/v1/folders/"+id+"/permissions", `{"entries":[{"user_id":"`+user.ID.String()+`","level":"view"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	raw, err := json.Marshal(onlyEventAfter(t, d, earlier, rec).Data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), user.ID.String()) || strings.Contains(string(raw), "example.com") {
		t.Fatalf("data = %s, want the user id and no email", raw)
	}
}
