package search

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
	"github.com/slashobserve/slashobserve/internal/threads"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const payrollGroup = "payroll-team"

var (
	admin   = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Root", Role: auth.RoleAdmin}
	editor  = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve", Role: auth.RoleEditor}
	payroll = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Pat", Role: auth.RoleViewer, Groups: []string{payrollGroup}}
	ada     = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Ada", Role: auth.RoleEditor}
	keyS    = auth.Principal{Kind: auth.KindServiceAccount, ID: "66666666-6666-4666-8666-666666666666", Name: "pipeline", Role: auth.RoleEditor}
	keyT    = auth.Principal{Kind: auth.KindServiceAccount, ID: "77777777-7777-4777-8777-777777777777", Name: "reader", Role: auth.RoleViewer}
)

type rig struct {
	db          *dbtest.Database
	service     *Service
	access      *folders.Access
	findings    *findings.Service
	annotations *annotations.Service
	threads     *threads.Service
	saved       *savedsearches.Service
}

func newRig(t *testing.T) *rig {
	t.Helper()
	d := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	access := folders.NewAccess(d.DB)
	annotationService := annotations.New(d.DB, access)
	dashboardService := dashboards.New(d.DB, nil, access, logger)
	findingService := findings.New(d.DB, access, annotationService, dashboardService, logger)
	r := &rig{
		db:          d,
		service:     New(d.DB, access, &url.URL{Scheme: "https", Host: "observe.example.com"}),
		access:      access,
		findings:    findingService,
		annotations: annotationService,
		threads:     threads.NewService(d.DB, access, dashboardService, findingService, annotationService, observations.New(d.DB, access, nil, nil, logger)),
		saved:       savedsearches.New(d.DB, access, nil, nil, logger),
	}
	for _, p := range []auth.Principal{admin, editor, payroll, ada} {
		d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $2 || '@example.com', $2)`, p.ID, p.Name)
	}
	for _, p := range []auth.Principal{keyS, keyT} {
		d.Exec(t, `INSERT INTO service_accounts (id, name) VALUES ($1::uuid, $2)`, p.ID, p.Name)
	}
	return r
}

func (r *rig) text(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) folder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	return mustUUID(t, r.text(t, `INSERT INTO folders (title) VALUES ($1) RETURNING id::text`, title))
}

func (r *rig) restricted(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	return foldertest.Restricted(t, r.db, title, payrollGroup)
}

func (r *rig) personal(t *testing.T, p auth.Principal) pgtype.UUID {
	t.Helper()
	return mustUUID(t, r.text(t, `SELECT id::text FROM folders WHERE personal AND owner_id = $1::uuid`, p.ID))
}

func (r *rig) dashboard(t *testing.T, title string, folder pgtype.UUID, description string) string {
	t.Helper()
	id := r.text(t, `INSERT INTO dashboards (title, latest_version, folder_id, created_by_kind, created_by_id, created_by_name, created_via)
VALUES ($1, 1, coalesce($2::uuid, general_folder_id()), 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, title, folder)
	r.version(t, id, 1, title, description)
	return id
}

func (r *rig) version(t *testing.T, dashboard string, version int, title, description string) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, parent_version, spec, summary, author_kind, author_id, author_name)
VALUES ($1::uuid, $2, CASE WHEN $2 = 1 THEN NULL ELSE $2 - 1 END, jsonb_build_object('title', $3::text, 'description', $4::text), 'seed', 'user', gen_random_uuid(), 'seed')`,
		dashboard, version, title, description)
}

type findingIn struct {
	title     string
	markdown  string
	dashboard string
	folder    pgtype.UUID
	by        auth.Principal
	ageDays   int
}

func (r *rig) finding(t *testing.T, in findingIn) string {
	t.Helper()
	if in.by.ID == "" {
		in.by = editor
	}
	var dashboard any
	if in.dashboard != "" {
		dashboard = in.dashboard
	}
	return r.text(t, `INSERT INTO findings (dashboard_id, folder_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name, created_at)
VALUES ($1::uuid, $2::uuid, $3, $4, now() - interval '1 hour', now(), $5, $6::uuid, $7, now() - make_interval(days => $8))
RETURNING id::text`, dashboard, in.folder, in.title, in.markdown, string(in.by.Kind), in.by.ID, in.by.Name, in.ageDays)
}

func (r *rig) annotation(t *testing.T, dashboard, body string) string {
	t.Helper()
	var target any
	if dashboard != "" {
		target = dashboard
	}
	return r.text(t, `INSERT INTO annotations (dashboard_id, time_from, text, tags) VALUES ($1::uuid, now(), $2, ARRAY[]::text[]) RETURNING id::text`, target, body)
}

func (r *rig) comment(t *testing.T, kind Kind, target, body string) (thread, message string) {
	t.Helper()
	switch kind {
	case KindDashboard:
		thread = r.text(t, `INSERT INTO threads (dashboard_id, dashboard_version) VALUES ($1::uuid, 1) RETURNING id::text`, target)
	case KindFinding:
		thread = r.text(t, `INSERT INTO threads (finding_id) VALUES ($1::uuid) RETURNING id::text`, target)
	case KindObservation:
		thread = r.text(t, `INSERT INTO threads (observation_id) VALUES ($1::uuid) RETURNING id::text`, target)
	default:
		thread = r.text(t, `INSERT INTO threads (annotation_id) VALUES ($1::uuid) RETURNING id::text`, target)
	}
	message = r.text(t, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name)
VALUES ($1::uuid, $2, 'user', $3::uuid, 'Ada') RETURNING id::text`, thread, body, ada.ID)
	return thread, message
}

func (r *rig) savedSearch(t *testing.T, kind, title string, query *string, folder pgtype.UUID) string {
	t.Helper()
	return r.text(t, `INSERT INTO saved_searches (kind, title, datasource_uid, query, filters, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name)
VALUES ($1, $2, 'loki', $3, '{}', 'now-1h', 'now', coalesce($4::uuid, general_folder_id()), 'user', $5::uuid, 'Ada') RETURNING id::text`,
		kind, title, query, folder, ada.ID)
}

func (r *rig) search(t *testing.T, by auth.Principal, q string, kinds ...Kind) []Result {
	t.Helper()
	var all []Result
	cursor := ""
	for {
		page, err := r.service.Search(t.Context(), by, Request{Q: q, Kinds: kinds, Limit: 7, Cursor: cursor})
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		all = append(all, page.Results...)
		if page.Next == "" {
			return all
		}
		cursor = page.Next
	}
}

func titles(results []Result) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Title
	}
	return out
}

func ids(results []Result) map[string]bool {
	out := map[string]bool{}
	for _, r := range results {
		out[string(r.Kind)+":"+r.ID] = true
	}
	return out
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func isHidden(err error) bool {
	return errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) || errors.Is(err, folders.ErrForbidden) ||
		errors.Is(err, findings.ErrNotFound) || errors.Is(err, annotations.ErrNotFound) || errors.Is(err, threads.ErrTargetNotFound)
}
