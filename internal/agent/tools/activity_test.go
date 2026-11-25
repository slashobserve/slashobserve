package tools

import (
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/origin"
)

type activityRig struct {
	registry *Registry
	db       *dbtest.Database
}

func newActivityRig(t *testing.T) activityRig {
	t.Helper()
	database := dbtest.New(t)
	store := activity.NewStore(database.DB, folders.NewAccess(database.DB))
	return activityRig{
		registry: New(Deps{Activity: store, Logger: slog.New(slog.DiscardHandler)}),
		db:       database,
	}
}

func (r activityRig) dashboard(t *testing.T, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	var raw string
	err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ('Checkout', 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Valid {
		foldertest.Move(t, r.db, id, folder)
	}
	return id
}

func (r activityRig) observation(t *testing.T, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	var raw string
	err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 'Checkout latency', 'user', gen_random_uuid(), 'Seed') RETURNING id::text`, folder.String()).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r activityRig) appendOnObservation(t *testing.T, eventType string, observation pgtype.UUID) int64 {
	t.Helper()
	var seq int64
	err := r.db.InTx(reading(t.Context()), func(q *dbsqlc.Queries) error {
		var err error
		seq, err = activity.Append(reading(t.Context()), q, activity.Entry{
			Type:     eventType,
			TargetID: observation,
			Owner:    activity.OnObservation(observation),
			Actor:    actor.Of(editorCaller, origin.Origin{Kind: origin.MCP, Client: "claude-code"}, ""),
			Data:     map[string]any{"title": "Checkout latency"},
		}, nil)
		return err
	})
	if err != nil {
		t.Fatalf("append %s: %v", eventType, err)
	}
	return seq
}

func (r activityRig) append(t *testing.T, eventType string, dashboard pgtype.UUID) int64 {
	t.Helper()
	var seq int64
	err := r.db.InTx(reading(t.Context()), func(q *dbsqlc.Queries) error {
		var err error
		seq, err = activity.Append(reading(t.Context()), q, activity.Entry{
			Type:     eventType,
			TargetID: dashboard,
			Owner:    activity.OnDashboard(dashboard),
			Actor:    actor.Of(editorCaller, origin.Origin{Kind: origin.MCP, Client: "claude-code"}, ""),
			Data:     map[string]any{"title": "Checkout"},
		}, nil)
		return err
	})
	if err != nil {
		t.Fatalf("append %s: %v", eventType, err)
	}
	return seq
}

func (r activityRig) list(t *testing.T, by auth.Principal, input string) (activityListOut, Result) {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), by), nameListActivity, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		return activityListOut{}, res
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var out activityListOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("content %s: %v", raw, err)
	}
	return out, res
}

func seqsOfActivity(out activityListOut) []int64 {
	seqs := make([]int64, len(out.Activity))
	for i, e := range out.Activity {
		seqs[i] = e.Seq
	}
	return seqs
}

func TestListActivityPagesNewestFirstWithACursor(t *testing.T) {
	r := newActivityRig(t)
	d := r.dashboard(t, pgtype.UUID{})
	first := r.append(t, activity.TypeDashboardCreated, d)
	second := r.append(t, activity.TypeDashboardVersionCreated, d)
	third := r.append(t, activity.TypeDashboardMoved, d)

	page, _ := r.list(t, viewerCaller, `{"limit":2}`)

	if got := seqsOfActivity(page); !slices.Equal(got, []int64{third, second}) {
		t.Fatalf("first page %v, want [%d %d]", got, third, second)
	}
	if page.NextCursor == nil || *page.NextCursor == "" {
		t.Fatalf("next_cursor = %v, want a cursor", page.NextCursor)
	}
	if label := page.Activity[0].Actor.Label; label == "" {
		t.Errorf("the event has no actor label: %+v", page.Activity[0].Actor)
	}

	cursor, err := json.Marshal(*page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	last, _ := r.list(t, viewerCaller, `{"limit":2,"cursor":`+string(cursor)+`}`)

	if got := seqsOfActivity(last); !slices.Equal(got, []int64{first}) {
		t.Fatalf("second page %v, want [%d]", got, first)
	}
	if last.NextCursor != nil {
		t.Fatalf("last page next_cursor = %v, want null", *last.NextCursor)
	}
}

func TestListActivityReturnsAnEmptyArrayWhenNothingHappened(t *testing.T) {
	r := newActivityRig(t)

	_, res := r.list(t, viewerCaller, `{}`)

	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"activity":[],"next_cursor":null}` {
		t.Fatalf("empty list = %s", raw)
	}
}

func TestListActivityNarrowsByTypeAndDashboard(t *testing.T) {
	r := newActivityRig(t)
	one := r.dashboard(t, pgtype.UUID{})
	other := r.dashboard(t, pgtype.UUID{})
	created := r.append(t, activity.TypeDashboardCreated, one)
	r.append(t, activity.TypeDashboardMoved, one)
	r.append(t, activity.TypeDashboardCreated, other)

	byType, _ := r.list(t, viewerCaller, `{"dashboard_id":"`+one.String()+`","types":["dashboard.created"]}`)

	if got := seqsOfActivity(byType); !slices.Equal(got, []int64{created}) {
		t.Fatalf("narrowed list %v, want [%d]", got, created)
	}
}

func TestListActivityNarrowsToOneObservation(t *testing.T) {
	r := newActivityRig(t)
	folder, err := r.db.Queries().CreateFolder(reading(t.Context()), dbsqlc.CreateFolderParams{Title: "Incidents"})
	if err != nil {
		t.Fatal(err)
	}
	one, other := r.observation(t, folder.ID), r.observation(t, folder.ID)
	created := r.appendOnObservation(t, activity.TypeObservationCreated, one)
	renamed := r.appendOnObservation(t, activity.TypeObservationUpdated, one)
	r.appendOnObservation(t, activity.TypeObservationCreated, other)

	got, _ := r.list(t, viewerCaller, `{"observation_id":"`+one.String()+`"}`)

	if ids := seqsOfActivity(got); !slices.Equal(ids, []int64{renamed, created}) {
		t.Fatalf("narrowed list %v, want [%d %d]", ids, renamed, created)
	}
}

func TestListActivityHidesWhatTheKeyMayNotSee(t *testing.T) {
	r := newActivityRig(t)
	restricted := foldertest.Restricted(t, r.db, "Payroll", privateGroup)
	hidden := r.dashboard(t, restricted)
	open := r.dashboard(t, pgtype.UUID{})
	r.append(t, activity.TypeDashboardCreated, hidden)
	visible := r.append(t, activity.TypeDashboardCreated, open)

	viewer, _ := r.list(t, viewerCaller, `{}`)
	member, _ := r.list(t, groupCaller, `{}`)

	if got := seqsOfActivity(viewer); !slices.Equal(got, []int64{visible}) {
		t.Errorf("viewer sees %v, want only [%d]", got, visible)
	}
	if len(member.Activity) != 2 {
		t.Errorf("group member sees %d events, want 2", len(member.Activity))
	}
}

func (r activityRig) appendAdminChange(t *testing.T, eventType string) int64 {
	t.Helper()
	var seq int64
	err := r.db.InTx(reading(t.Context()), func(q *dbsqlc.Queries) error {
		var err error
		seq, err = activity.Append(reading(t.Context()), q, activity.Entry{
			Type:     eventType,
			TargetID: pgtype.UUID{Bytes: [16]byte{15: 1}, Valid: true},
			Actor:    actor.Of(editorCaller, origin.Origin{Kind: origin.MCP, Client: "claude-code"}, ""),
			Data:     map[string]any{"name": "SIEM"},
		}, nil)
		return err
	})
	if err != nil {
		t.Fatalf("append %s: %v", eventType, err)
	}
	return seq
}

func TestListActivityShowsAdminChangesToAdminsOnly(t *testing.T) {
	r := newActivityRig(t)
	changed := r.appendAdminChange(t, activity.TypeWebhookCreated)
	d := r.dashboard(t, pgtype.UUID{})
	workspace := r.append(t, activity.TypeDashboardCreated, d)

	asAdmin, _ := r.list(t, adminCaller, `{}`)
	byActor, _ := r.list(t, adminCaller, `{"types":["webhook.created"],"actor_id":"`+editorCaller.ID+`","until":"2999-01-01T00:00:00Z"}`)
	asEditor, _ := r.list(t, editorCaller, `{}`)
	_, scoped := r.list(t, adminCaller, `{"scope":"admin"}`)

	if got := seqsOfActivity(asAdmin); !slices.Equal(got, []int64{workspace, changed}) {
		t.Errorf("admin sees %v, want [%d %d]", got, workspace, changed)
	}
	if got := seqsOfActivity(byActor); !slices.Equal(got, []int64{changed}) {
		t.Errorf("actor filter lists %v, want [%d]", got, changed)
	}
	if got := seqsOfActivity(asEditor); !slices.Equal(got, []int64{workspace}) {
		t.Errorf("editor sees %v, want only [%d]", got, workspace)
	}
	body, _ := scoped.Content.(errorBody)
	if !scoped.IsError || body.Code != codeInvalidRequest || !strings.HasPrefix(body.Message, `unknown field "scope"`) {
		t.Errorf("scope admin: error %v, %+v", scoped.IsError, body)
	}
}

func TestListActivityRefusesBadInput(t *testing.T) {
	r := newActivityRig(t)
	cases := []struct {
		name    string
		input   string
		message string
	}{
		{"limit 0", `{"limit":0}`, "limit must be between 1 and 100"},
		{"limit 101", `{"limit":101}`, "limit must be between 1 and 100"},
		{"dashboard not a uuid", `{"dashboard_id":"shop"}`, "dashboard_id must be a UUID"},
		{"folder not a uuid", `{"folder_id":"shop"}`, "folder_id must be a UUID"},
		{"observation not a uuid", `{"observation_id":"shop"}`, "observation_id must be a UUID"},
		{"unknown type", `{"types":["dashboard.exploded"]}`, `type "dashboard.exploded" is not an activity type`},
		{"since not RFC 3339", `{"since":"yesterday"}`, "since must be an RFC 3339 time"},
		{"actor not a uuid", `{"actor_id":"ada"}`, "actor_id must be a UUID"},
		{"until not RFC 3339", `{"until":"tomorrow"}`, "until must be an RFC 3339 time"},
		{"cursor not a cursor", `{"cursor":"nope"}`, "cursor is not valid"},
		{"cursor too long", `{"cursor":"` + strings.Repeat("c", maxNameBytes+1) + `"}`, "cursor must be at most 256 bytes"},
	}
	for _, c := range cases {
		_, res := r.list(t, viewerCaller, c.input)
		body, _ := res.Content.(errorBody)
		if !res.IsError || body.Code != codeInvalidRequest || body.Message != c.message {
			t.Errorf("%s: error %v, %+v", c.name, res.IsError, body)
		}
	}
}

func TestListActivityIsOfferedInEveryRunAndNeedsAViewer(t *testing.T) {
	var tool Tool
	for _, candidate := range New(Deps{}).List() {
		if candidate.Name == nameListActivity {
			tool = candidate
		}
	}
	if tool.MCPOnly || !tool.ReadOnly || tool.MinRole != auth.RoleViewer {
		t.Fatalf("list_activity = %+v", tool)
	}
	for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest, ModeInvestigate} {
		if !tool.OfferedIn(mode) {
			t.Errorf("list_activity is not offered to the agent in %s", mode)
		}
	}
}
