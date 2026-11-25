package dashboards

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

var placementAdmin = auth.Principal{Kind: auth.KindUser, ID: testUserID, Name: "Ada", Role: auth.RoleAdmin}

func newPlacementRig(t *testing.T) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: fakeDatasource{}}); err != nil {
		t.Fatal(err)
	}
	return &rig{db: database, service: New(database.DB, queryproxy.NewRunner(registry, logger), folders.NewAccess(database.DB), logger)}
}

func (r *rig) folder(t *testing.T, title string, adminsOnly bool) pgtype.UUID {
	t.Helper()
	var id string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(),
		`INSERT INTO folders (title, admins_only, created_by_kind, created_by_id, created_by_name) VALUES ($1, $2, 'user', $3, 'Ada') RETURNING id::text`,
		title, adminsOnly, testUserID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	folder, err := db.ParseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	return folder
}

func auditSpec(title string) []byte {
	return bytes.ReplaceAll(panelsSpec(title, "rps"), []byte(`"datasource":"prom"`), []byte(`"datasource":"audit"`))
}

func variableSpec(title string) []byte {
	return []byte(`{"specVersion":"0","title":"` + title + `","time":{"from":"now-15m","to":"now"},` +
		`"variables":[{"name":"ds","type":"datasource","datasourceType":"prometheus","default":"audit"}],` +
		`"panels":[{"id":"rps","type":"timeseries","title":"rps","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"${ds}","queries":[{"query":"up"}]}]}`)
}

func restricted(t *testing.T, err error, datasource string) {
	t.Helper()
	var refusal *folders.RestrictedContentError
	if !errors.As(err, &refusal) || refusal.Datasource != datasource {
		t.Fatalf("err = %v, want a restricted content refusal naming %q", err, datasource)
	}
}

func TestContentOnAnAdminsOnlyDatasourceIsCreatedOnlyInAnAdminsOnlyFolder(t *testing.T) {
	r := newPlacementRig(t)
	open, locked := r.folder(t, "Team", false), r.folder(t, "Payroll", true)
	ctx := reading(t.Context())

	for name, doc := range map[string][]byte{"a panel": auditSpec("Salaries"), "a datasource variable default": variableSpec("Picked")} {
		t.Run(name, func(t *testing.T) {
			_, _, err := r.service.Create(ctx, placementAdmin, open, doc, "create")
			restricted(t, err, "audit")
			if _, _, err := r.service.Create(ctx, placementAdmin, r.generalFolder(t), doc, "create"); err == nil {
				t.Fatal("General took content on an admins only datasource")
			}
			if _, _, err := r.service.Create(ctx, placementAdmin, locked, doc, "create"); err != nil {
				t.Fatalf("admins only folder = %v", err)
			}
		})
	}
	var count int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM dashboards WHERE folder_id <> $1`, locked.String()).Scan(&count); err != nil || count != 0 {
		t.Fatalf("dashboards outside the admins only folder = %d %v", count, err)
	}
}

func TestANewVersionOnAnAdminsOnlyDatasourceIsRefusedOutsideAnAdminsOnlyFolder(t *testing.T) {
	r := newPlacementRig(t)
	ctx := reading(t.Context())
	dash, _, err := r.service.Create(ctx, placementAdmin, r.folder(t, "Team", false), panelsSpec("Open", "rps"), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := db.ParseUUID(dash.ID)
	_, err = r.service.Save(ctx, placementAdmin, SaveInput{DashboardID: id, Parent: 1, Spec: auditSpec("Open"), Summary: "read audit"})
	restricted(t, err, "audit")
	if _, ver, err := r.service.Get(ctx, id); err != nil || ver.Number != 1 {
		t.Fatalf("latest after the refusal = %d %v", ver.Number, err)
	}
}

func TestContentOnAnAdminsOnlyDatasourceNeverLeavesTheAdminsOnlyFolder(t *testing.T) {
	r := newPlacementRig(t)
	ctx := reading(t.Context())
	locked, other := r.folder(t, "Payroll", true), r.folder(t, "Payroll archive", true)
	dash, _, err := r.service.Create(ctx, placementAdmin, locked, auditSpec("Salaries"), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := db.ParseUUID(dash.ID)
	if _, err := r.service.Save(ctx, placementAdmin, SaveInput{DashboardID: id, Parent: 1, Spec: panelsSpec("Salaries", "rps"), Summary: "drop audit"}); err != nil {
		t.Fatal(err)
	}

	_, _, err = r.service.Move(ctx, placementAdmin, id, r.generalFolder(t))
	restricted(t, err, "audit")
	if got := r.folderOfDashboard(t, id); got != locked {
		t.Fatalf("folder after the refusal = %v", got)
	}
	if _, _, err := r.service.Move(ctx, placementAdmin, id, other); err != nil {
		t.Fatalf("move between admins only folders = %v", err)
	}

	editor := auth.Principal{Kind: auth.KindUser, ID: testUserID, Name: "Ada", Role: auth.RoleEditor}
	if _, err := folders.NewAccess(r.db.DB).Dashboard(ctx, editor, id); !errors.Is(err, folders.ErrNoAccess) && !errors.Is(err, folders.ErrNotFound) {
		t.Fatalf("a non-Admin creator reads the dashboard: %v", err)
	}
}

func TestADuplicateWithoutAFolderGoesToTheSourcesFolderWhenTheCallerCanCreateThere(t *testing.T) {
	r := newPlacementRig(t)
	ctx := reading(t.Context())
	locked, other := r.folder(t, "Payroll", true), r.folder(t, "Team", false)
	dash, _, err := r.service.Create(ctx, placementAdmin, locked, auditSpec("Salaries"), "create")
	if err != nil {
		t.Fatal(err)
	}
	source, _ := db.ParseUUID(dash.ID)

	copied, _, err := r.service.Duplicate(ctx, placementAdmin, source, pgtype.UUID{}, "Salaries copy")
	if err != nil {
		t.Fatalf("duplicate without a folder: %v", err)
	}
	if got := r.folderOfDashboard(t, mustUUID(t, copied.ID)); got != locked {
		t.Fatalf("the copy is in folder %v, want the source's folder %v", got, locked)
	}

	_, _, err = r.service.Duplicate(ctx, placementAdmin, source, other, "Salaries elsewhere")
	restricted(t, err, "audit")
}

func TestADuplicateWithoutAFolderGoesToMyDashboardsWhenTheCallerOnlyViewsTheSourcesFolderAndToTheSourcesFolderWhenTheyCanEditIt(t *testing.T) {
	for _, c := range []struct {
		name    string
		grant   string
		wantOwn bool
	}{
		{"a View grant", "view", true},
		{"an Edit grant", "edit", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newPlacementRig(t)
			ctx := reading(t.Context())
			shared := r.folder(t, "Team", false)
			dash, _, err := r.service.Create(ctx, placementAdmin, shared, panelsSpec("Latency", "rps"), "create")
			if err != nil {
				t.Fatal(err)
			}
			source, _ := db.ParseUUID(dash.ID)
			person := r.addPerson(t, "vera")
			viewer := person.principal
			viewer.Role = auth.RoleViewer
			if _, err := r.db.SQLDB(t).ExecContext(t.Context(),
				`INSERT INTO folder_permissions (folder_id, user_id, level) VALUES ($1, $2, $3)`, shared, viewer.ID, c.grant); err != nil {
				t.Fatal(err)
			}

			copied, _, err := r.service.Duplicate(ctx, viewer, source, pgtype.UUID{}, "Latency copy")
			if err != nil {
				t.Fatalf("duplicate without a folder: %v", err)
			}

			want := shared
			if c.wantOwn {
				want = person.personal
			}
			if got := r.folderOfDashboard(t, mustUUID(t, copied.ID)); got != want {
				t.Fatalf("the copy is in folder %v, want %v", got, want)
			}
		})
	}
}

func TestARunThatReadAdminOnlyDataMovesNoDashboardWhereOthersRead(t *testing.T) {
	r := newPlacementRig(t)
	const run = "99999999-9999-4999-8999-999999999999"
	r.db.Exec(t, `INSERT INTO agent_requests (id, intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, read_restricted) VALUES ($1::uuid, 'ask', 'investigate', 'who changed it', 'user', $2::uuid, 'Ada', true)`, run, testUserID)
	locked, other := r.folder(t, "Payroll", true), r.folder(t, "Payroll archive", true)
	dash, _, err := r.service.Create(reading(t.Context()), placementAdmin, locked, panelsSpec("Latency", "rps"), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := db.ParseUUID(dash.ID)
	ctx := actor.AsConnectedAgent(actor.WithRun(reading(t.Context()), run))
	if _, _, err := r.service.Move(ctx, placementAdmin, id, r.generalFolder(t)); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("move where others read = %v, want ErrRestrictedAnswer", err)
	}
	if got := r.folderOfDashboard(t, id); got != locked {
		t.Fatalf("folder after the refusal = %v", got)
	}
	if _, _, err := r.service.Move(ctx, placementAdmin, id, other); err != nil {
		t.Fatalf("move where only Admins read = %v", err)
	}
}
