package placement

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type holdersRig struct {
	d *dbtest.Database
}

func (r holdersRig) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := r.d.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r holdersRig) dashboard(t *testing.T, folder pgtype.UUID, specs ...string) string {
	t.Helper()
	id := r.scalar(t, `INSERT INTO dashboards (title, latest_version, folder_id, created_by_kind, created_by_id, created_by_name, created_via) VALUES ('Salaries', $2, $1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, folder, len(specs))
	for i, s := range specs {
		r.d.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, spec, summary, author_kind, author_id, author_name) VALUES ($1::uuid, $2, $3::jsonb, 'seed', 'user', gen_random_uuid(), 'seed')`, id, i+1, s)
	}
	return id
}

func (r holdersRig) observation(t *testing.T, folder pgtype.UUID) string {
	t.Helper()
	return r.scalar(t, `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name) VALUES ($1, 'Payroll drift', 'user', gen_random_uuid(), 'Ada') RETURNING id::text`, folder)
}

const (
	readsAudit = `{"title":"Salaries","panels":[{"id":"p","type":"stat","datasource":"audit","queries":[{"query":"up"}]}]}`
	readsProm  = `{"title":"Salaries","panels":[{"id":"p","type":"stat","datasource":"prom","queries":[{"query":"up"}]}]}`
	namesAudit = `{"title":"Salaries","description":"not the \"audit\" datasource","panels":[{"id":"p","type":"stat","datasource":"prom","queries":[{"query":"up"}]}]}`
)

func newHoldersRig(t *testing.T) holdersRig {
	t.Helper()
	r := holdersRig{d: dbtest.New(t)}
	r.d.Exec(t, `INSERT INTO datasources (uid, type, name, config, audience) VALUES ('audit', 'prometheus', 'Audit', '{}', 'admins')`)
	return r
}

func TestAFolderHolderIsFoundInEveryKindOfContent(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	cases := map[string]func(folder pgtype.UUID) folders.ContentKind{
		"an older dashboard version": func(f pgtype.UUID) folders.ContentKind {
			r.dashboard(t, f, readsAudit, readsProm)
			return folders.ContentDashboard
		},
		"a proposal revision": func(f pgtype.UUID) folders.ContentKind {
			dash := r.dashboard(t, f, readsProm)
			proposal := r.scalar(t, `INSERT INTO proposals (dashboard_id, title, author_kind, author_id, author_name) VALUES ($1::uuid, 'Add salaries', 'user', gen_random_uuid(), 'Ada') RETURNING id::text`, dash)
			r.d.Exec(t, `INSERT INTO proposal_revisions (proposal_id, revision, base_version, spec, summary) VALUES ($1::uuid, 1, 1, $2::jsonb, 'add')`, proposal, readsAudit)
			return folders.ContentProposal
		},
		"a panel block": func(f pgtype.UUID) folders.ContentKind {
			r.d.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name) VALUES ($1::uuid, 'panel', 0, '{"panel":{"type":"stat","datasource":"audit","queries":[{"query":"up"}]}}', 'user', gen_random_uuid(), 'Ada')`, r.observation(t, f))
			return folders.ContentObservation
		},
		"a snapshot taken through a variable": func(f pgtype.UUID) folders.ContentKind {
			r.d.Exec(t, `INSERT INTO snapshots (observation_id, panel, query, frames, user_scoped, time_from, time_to, created_by_kind, created_by_id, created_by_name, datasource_uids)
VALUES ($1::uuid, '{"type":"stat","datasource":"${ds}"}', '[]', '{"frames":[]}', false, now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'Ada', '{audit}')`, r.observation(t, f))
			return folders.ContentObservation
		},
		"a saved search": func(f pgtype.UUID) folders.ContentKind {
			r.d.Exec(t, `INSERT INTO saved_searches (kind, title, datasource_uid, query, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name) VALUES ('metrics', 'Pay', 'audit', 'up', 'now-1h', 'now', $1, 'user', gen_random_uuid(), 'Ada')`, f)
			return folders.ContentSavedSearch
		},
	}
	for name, add := range cases {
		t.Run(name, func(t *testing.T) {
			folder := foldertest.AdminsOnly(t, r.d, name)
			want := add(folder)
			h, held, err := g.FolderHolder(t.Context(), r.d.Queries(), folder)
			if err != nil || !held || h.Kind != want || h.Datasource != "audit" || h.FolderID != folder.String() {
				t.Fatalf("holder = %+v %v %v", h, held, err)
			}
			if shared, err := SharedHolders(t.Context(), r.d.Queries(), "audit"); err != nil || len(shared) != 0 {
				t.Fatalf("shared holders of content in an admins only folder = %+v %v", shared, err)
			}
		})
	}
}

func TestAFolderHoldsNothingWhenTheTextOnlyNamesTheDatasource(t *testing.T) {
	r := newHoldersRig(t)
	folder := foldertest.Open(t, r.d, "Team")
	r.dashboard(t, folder, namesAudit)
	if h, held, err := New(nil).FolderHolder(t.Context(), r.d.Queries(), folder); err != nil || held {
		t.Fatalf("holder = %+v %v %v", h, held, err)
	}
	r.dashboard(t, folder, readsAudit)
	shared, err := SharedHolders(t.Context(), r.d.Queries(), "audit")
	if err != nil || len(shared) != 1 || shared[0].FolderTitle != "Team" || shared[0].Personal {
		t.Fatalf("shared = %+v %v", shared, err)
	}
}
