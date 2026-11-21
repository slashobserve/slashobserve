package placement

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

func (r holdersRig) personalFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	user := r.scalar(t, `INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', gen_random_uuid()::text, gen_random_uuid()::text || '@example.com', 'Ada') RETURNING id::text`)
	var id pgtype.UUID
	if err := r.d.SQLDB(t).QueryRowContext(t.Context(), `SELECT id FROM folders WHERE personal AND owner_id = $1::uuid`, user).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (r holdersRig) savedSearch(t *testing.T, folder pgtype.UUID, title string) {
	t.Helper()
	r.d.Exec(t, `INSERT INTO saved_searches (kind, title, datasource_uid, query, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name) VALUES ('metrics', $2, 'audit', 'up', 'now-1h', 'now', $1, 'user', gen_random_uuid(), 'Ada')`, folder, title)
}

func (r holdersRig) heldTwice(t *testing.T, folder pgtype.UUID) {
	t.Helper()
	id := r.observation(t, folder)
	r.d.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name) VALUES ($1::uuid, 'panel', 0, '{"panel":{"type":"stat","datasource":"audit","queries":[{"query":"up"}]}}', 'user', gen_random_uuid(), 'Ada')`, id)
	r.d.Exec(t, `INSERT INTO snapshots (observation_id, panel, query, frames, user_scoped, time_from, time_to, created_by_kind, created_by_id, created_by_name, datasource_uids)
VALUES ($1::uuid, '{"type":"stat","datasource":"${ds}"}', '[]', '{"frames":[]}', false, now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'Ada', '{audit}')`, id)
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
			c, err := g.FolderCensus(t.Context(), r.d.Queries(), folder)
			if err != nil || len(c.Listed) != 1 {
				t.Fatalf("census = %+v %v", c, err)
			}
			if h := c.Listed[0]; h.Kind != want || h.Datasource != "audit" || h.FolderID != folder.String() {
				t.Fatalf("holder = %+v", h)
			}
			if shared, err := SharedCensus(t.Context(), r.d.Queries(), "audit"); err != nil || shared.Held() {
				t.Fatalf("shared holders of content in an admins only folder = %+v %v", shared, err)
			}
		})
	}
}

func TestAFolderHoldsNothingWhenTheTextOnlyNamesTheDatasource(t *testing.T) {
	r := newHoldersRig(t)
	folder := foldertest.Open(t, r.d, "Team")
	r.dashboard(t, folder, namesAudit)
	if c, err := New(nil).FolderCensus(t.Context(), r.d.Queries(), folder); err != nil || c.Held() {
		t.Fatalf("census = %+v %v", c, err)
	}
	r.dashboard(t, folder, readsAudit)
	shared, err := SharedCensus(t.Context(), r.d.Queries(), "audit")
	if err != nil || shared.Shared != 1 || shared.Personal != 0 || len(shared.Listed) != 1 || shared.Listed[0].FolderTitle != "Team" {
		t.Fatalf("shared = %+v %v", shared, err)
	}
}

func TestTheSharedCensusListsOnlySharedItemsAndCountsEachItemOnce(t *testing.T) {
	r := newHoldersRig(t)
	mine := r.personalFolder(t)
	for i := range 3 {
		r.savedSearch(t, mine, fmt.Sprintf("Mine %d", i))
	}
	r.dashboard(t, mine, readsAudit)
	team := foldertest.Open(t, r.d, "Team")
	for range MaxListed + 2 {
		r.dashboard(t, team, readsAudit, readsProm, readsAudit)
	}
	r.dashboard(t, team, namesAudit)
	r.heldTwice(t, team)
	r.heldTwice(t, team)
	r.dashboard(t, foldertest.AdminsOnly(t, r.d, "Payroll"), readsAudit)

	c, err := SharedCensus(t.Context(), r.d.Queries(), "audit")
	if err != nil {
		t.Fatal(err)
	}
	if c.Shared != MaxListed+4 || c.Personal != 4 || len(c.Listed) != MaxListed || c.Unlisted() != 4 {
		t.Fatalf("census = %d shared, %d personal, %d listed, %d unlisted; want %d, 4, %d, 4", c.Shared, c.Personal, len(c.Listed), c.Unlisted(), MaxListed+4, MaxListed)
	}
	for _, h := range c.Listed {
		if h.FolderID != team.String() {
			t.Fatalf("listed %+v, want only items in the shared folder", h)
		}
	}
}

func TestTheBootRefusalNamesNoItemInAPersonalFolder(t *testing.T) {
	r := newHoldersRig(t)
	mine := r.personalFolder(t)
	r.savedSearch(t, mine, "My payroll search")
	r.savedSearch(t, foldertest.Open(t, r.d, "Team"), "Team payroll search")

	err := CheckFileAudiences(t.Context(), r.d.Queries(), []string{"audit"})
	var misplaced *MisplacedError
	if !errors.As(err, &misplaced) {
		t.Fatalf("check = %v, want a refusal", err)
	}
	text := err.Error()
	for _, want := range []string{`but 2 dashboards`, `"Team payroll search" in folder "Team"`, "1 in people's My Dashboards"} {
		if !strings.Contains(text, want) {
			t.Fatalf("refusal lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "My payroll search") || strings.Contains(text, mine.String()) {
		t.Fatalf("refusal names the personal item:\n%s", text)
	}
}

func TestAFolderCensusListsTwentyHoldersAndCountsTheRest(t *testing.T) {
	r := newHoldersRig(t)
	folder := foldertest.AdminsOnly(t, r.d, "Payroll")
	for range MaxListed + 2 {
		r.d.Exec(t, `INSERT INTO saved_searches (kind, title, datasource_uid, query, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name) VALUES ('metrics', 'Pay', 'audit', 'up', 'now-1h', 'now', $1, 'user', gen_random_uuid(), 'Ada')`, folder)
	}
	c, err := New(nil).FolderCensus(t.Context(), r.d.Queries(), folder)
	if err != nil || len(c.Listed) != MaxListed || c.Unlisted() != 2 {
		t.Fatalf("census = %d listed, %d unlisted, %v", len(c.Listed), c.Unlisted(), err)
	}
}
