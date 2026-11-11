package observations_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const hiddenGroup = "payroll-team"

var (
	editor      = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve", Role: auth.RoleEditor}
	otherEditor = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Ed", Role: auth.RoleEditor}
	viewer      = auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Name: "Vic", Role: auth.RoleViewer}
	commenter   = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Cam", Role: auth.RoleCommenter}
	otherCommen = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Cy", Role: auth.RoleCommenter}
	admin       = auth.Principal{Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Root", Role: auth.RoleAdmin}
	key         = auth.Principal{Kind: auth.KindServiceAccount, ID: "77777777-7777-4777-8777-777777777777", Name: "ci", Role: auth.RoleEditor}
)

const frozenData = `{"frames":[],"meta":{"kind":"range","from":"2026-11-01T07:00:00Z","to":"2026-11-01T09:00:00Z","step_ms":15000,"queries":[{"ref":"A","kind":"range","step_ms":15000}],"cached":false,"executed_at":"2026-11-01T09:00:01Z","truncated":false,"partial":false,"rows":0}}`

var windowTo = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type fakeFreezer struct {
	data       json.RawMessage
	userScoped bool
	err        error
	calls      int
	panelID    string
	title      string
	from, to   time.Time
}

func (f *fakeFreezer) Freeze(_ context.Context, _ auth.Principal, d *spec.Dashboard, panelID string, from, to time.Time) (observations.Frozen, error) {
	f.calls++
	f.panelID, f.title, f.from, f.to = panelID, d.Title, from, to
	if f.err != nil {
		return observations.Frozen{}, f.err
	}
	data := f.data
	if data == nil {
		data = json.RawMessage(frozenData)
	}
	return observations.Frozen{Data: data, UserScoped: f.userScoped}, nil
}

type rig struct {
	db      *dbtest.Database
	service *observations.Service
	freezer *fakeFreezer
	shared  pgtype.UUID
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	freezer := &fakeFreezer{}
	r := rig{db: d, freezer: freezer, service: observations.New(d.DB, folders.NewAccess(d.DB), freezer, nil, slog.New(slog.DiscardHandler))}
	for _, p := range []auth.Principal{editor, otherEditor, viewer, commenter, otherCommen, admin} {
		d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $2 || '@example.com', $3)`, p.ID, p.Name, p.Name)
	}
	d.Exec(t, `INSERT INTO service_accounts (id, name) VALUES ($1::uuid, $2)`, key.ID, key.Name)
	r.shared = r.folder(t, "Incidents")
	return r
}

func (r rig) folder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title) VALUES ($1) RETURNING id::text`, title).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, raw)
}

func (r rig) hiddenFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	return foldertest.Restricted(t, r.db, "Payroll", hiddenGroup)
}

func (r rig) personalFolder(t *testing.T, p auth.Principal) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM folders WHERE personal AND owner_id = $1::uuid`, p.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, raw)
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r rig) create(t *testing.T, by auth.Principal, folder pgtype.UUID) observations.Observation {
	t.Helper()
	o, err := r.service.Create(t.Context(), by, observations.CreateInput{FolderID: folder, Title: "Checkout 5xx"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return o
}

func textBody(markdown string) json.RawMessage {
	raw, err := json.Marshal(map[string]string{"markdown": markdown})
	if err != nil {
		panic(err)
	}
	return raw
}

func panelBody() json.RawMessage {
	return json.RawMessage(`{"panel":{"type":"timeseries","title":"Rate","datasource":"prom","queries":[{"ref":"A","query":"sum(rate(http_requests_total[5m]))"}]}}`)
}

func (r rig) addText(t *testing.T, by auth.Principal, o observations.Observation, markdown string) observations.Block {
	t.Helper()
	b, err := r.service.AddBlock(t.Context(), by, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindText, Body: textBody(markdown)})
	if err != nil {
		t.Fatalf("add text block: %v", err)
	}
	return b
}

func (r rig) addPanel(t *testing.T, by auth.Principal, o observations.Observation) observations.Block {
	t.Helper()
	b, err := r.service.AddBlock(t.Context(), by, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindPanel, Body: panelBody()})
	if err != nil {
		t.Fatalf("add panel block: %v", err)
	}
	return b
}

func (r rig) positions(t *testing.T, by auth.Principal, o observations.Observation) []string {
	t.Helper()
	got, err := r.service.Get(t.Context(), by, mustUUID(t, o.ID))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(got.Blocks))
	for i, b := range got.Blocks {
		if b.Position != i {
			t.Fatalf("block %d has position %d", i, b.Position)
		}
		var body struct {
			Markdown string `json:"markdown"`
		}
		_ = json.Unmarshal(b.Body, &body)
		out[i] = strings.TrimSpace(body.Markdown)
	}
	return out
}

func (r rig) activityTypes(t *testing.T, o observations.Observation) []string {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT type FROM activity WHERE observation_id = $1::uuid AND target_kind = 'observation' ORDER BY seq`, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatal(err)
		}
		out = append(out, typ)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
