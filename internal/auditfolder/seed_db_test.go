package auditfolder

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/provision"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

var admin = auth.Principal{Kind: auth.KindUser, ID: "00000000-0000-4000-8000-0000000000ad", Name: "Ada", Role: auth.RoleAdmin}

type seedRig struct {
	db         *dbtest.Database
	dashboards *dashboards.Service
	access     *folders.Access
}

func newSeedRig(t *testing.T) seedRig {
	t.Helper()
	d := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	inst, err := auditsource.Instance(trail.NewReader(d.DB, logger), logger)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := datasource.NewRegistry(logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(inst); err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(d.DB)
	return seedRig{db: d, access: access, dashboards: dashboards.New(d.DB, queryproxy.NewRunner(registry, logger), access, logger)}
}

func (r seedRig) seeder() *Seeder {
	return New(r.db.DB, r.dashboards, r.access, provision.Principal, slog.New(slog.DiscardHandler))
}

func (r seedRig) seed(t *testing.T) {
	t.Helper()
	if err := r.seeder().Seed(t.Context()); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func (r seedRig) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r seedRig) auditFolder(t *testing.T) (id string, adminsOnly bool) {
	t.Helper()
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text, admins_only FROM folders WHERE title = 'Audit'`).Scan(&id, &adminsOnly); err != nil {
		t.Fatal(err)
	}
	return id, adminsOnly
}

func (r seedRig) wantSeeded(t *testing.T) string {
	t.Helper()
	if n := r.count(t, `SELECT count(*) FROM folders WHERE title = 'Audit'`); n != 1 {
		t.Fatalf("%d Audit folders, want 1", n)
	}
	id, adminsOnly := r.auditFolder(t)
	if !adminsOnly {
		t.Fatal("the Audit folder is readable by everyone, want Admins only")
	}
	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE folder_id = $1`, id); n != 4 {
		t.Fatalf("%d dashboards in the Audit folder, want 4", n)
	}
	if n := r.count(t, `SELECT count(*) FROM starter_content`); n != 5 {
		t.Fatalf("%d starter rows, want 5", n)
	}
	return id
}

func TestSeedingTwiceCreatesTheAuditFolderAndEachDashboardOnce(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	r.seed(t)
	r.wantSeeded(t)
	if n := r.count(t, `SELECT count(*) FROM audit_events WHERE action = 'folder.create'`); n != 1 {
		t.Fatalf("%d folder.create events, want 1", n)
	}
	if n := r.count(t, `SELECT count(*) FROM audit_events WHERE action = 'dashboard.create'`); n != 4 {
		t.Fatalf("%d dashboard.create events, want 4", n)
	}
}

func TestServersSeedingTogetherCreateEachStarterOnce(t *testing.T) {
	r := newSeedRig(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Go(func() {
			<-start
			errs[i] = r.seeder().Seed(t.Context())
		})
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("server %d: %v", i, err)
		}
	}
	r.wantSeeded(t)
}

func (r seedRig) starterID(t *testing.T, title string) string {
	t.Helper()
	var id string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(),
		`SELECT d.id::text FROM dashboards d JOIN starter_content s ON s.ref = d.id WHERE d.title = $1`, title).Scan(&id); err != nil {
		t.Fatalf("the starter dashboard %q: %v", title, err)
	}
	return id
}

func (r seedRig) latestVersion(t *testing.T, id string) int {
	t.Helper()
	return r.count(t, `SELECT latest_version FROM dashboards WHERE id = $1`, id)
}

func (r seedRig) changedRelease(t *testing.T, title string) func() ([]Starter, error) {
	t.Helper()
	starters, err := Starters()
	if err != nil {
		t.Fatal(err)
	}
	for i, st := range starters {
		if st.Title != title {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(st.Spec, &doc); err != nil {
			t.Fatal(err)
		}
		doc["description"] = "Reworded in a newer release"
		starters[i].Spec, err = json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return func() ([]Starter, error) { return starters, nil }
	}
	t.Fatalf("no starter dashboard %q", title)
	return nil
}

func (r seedRig) seedRelease(t *testing.T, release func() ([]Starter, error)) {
	t.Helper()
	s := r.seeder()
	s.shipped = release
	if err := s.Seed(t.Context()); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func (r seedRig) accessOf(t *testing.T, id string) folders.DashboardAccess {
	t.Helper()
	parsed, err := db.ParseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	access, err := r.access.Dashboard(t.Context(), admin, parsed)
	if err != nil {
		t.Fatal(err)
	}
	return access
}

func TestEveryStarterDashboardIsProvisionedAndReadOnlyFromTheFirstStart(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	for _, title := range []string{"Audit overview", "Sign-ins and access", "Agent and MCP activity", "Data access"} {
		access := r.accessOf(t, r.starterID(t, title))
		if !access.Provisioned || access.Shown().Edit || !access.Abilities.Comment {
			t.Errorf("%s: provisioned %v, edit shown %v, comment %v, want provisioned, read-only, still commentable", title, access.Provisioned, access.Shown().Edit, access.Abilities.Comment)
		}
	}
}

func TestARestartWithUnchangedStartersAddsNoVersionAndRecordsNothing(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	events := r.count(t, `SELECT count(*) FROM audit_events WHERE action = 'provision.apply'`)
	r.seed(t)
	r.seed(t)
	if n := r.count(t, `SELECT sum(latest_version)::int FROM dashboards`); n != 4 {
		t.Fatalf("latest versions add up to %d after restarts, want 4", n)
	}
	if n := r.count(t, `SELECT count(*) FROM dashboard_versions`); n != 4 {
		t.Fatalf("%d versions after restarts, want 4", n)
	}
	if n := r.count(t, `SELECT count(*) FROM audit_events WHERE action = 'provision.apply'`); n != events {
		t.Fatalf("%d provision events after restarts, want the %d of the first start", n, events)
	}
}

func TestAReleaseThatChangesOneStarterAddsOneVersionToIt(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	overview := r.starterID(t, "Audit overview")
	accessStarter := r.starterID(t, "Sign-ins and access")
	before := r.count(t, `SELECT count(*) FROM audit_events WHERE action = 'provision.apply'`)

	release := r.changedRelease(t, "Audit overview")
	r.seedRelease(t, release)
	r.seedRelease(t, release)

	if n := r.latestVersion(t, overview); n != 2 {
		t.Fatalf("the changed starter is at version %d, want 2", n)
	}
	if n := r.latestVersion(t, accessStarter); n != 1 {
		t.Fatalf("an unchanged starter is at version %d, want 1", n)
	}
	var summary, authorID string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT summary, author_id::text FROM dashboard_versions WHERE dashboard_id = $1 AND version = 2`, overview).Scan(&summary, &authorID); err != nil {
		t.Fatal(err)
	}
	if summary != releaseSummary || authorID != provision.Principal.ID {
		t.Fatalf("version 2 is %q by %s, want the release update by Provisioning", summary, authorID)
	}
	if n := r.count(t, `SELECT count(*) FROM audit_events WHERE action = 'provision.apply'`); n != before+1 {
		t.Fatalf("%d provision events, want %d", n, before+1)
	}
	if access := r.accessOf(t, overview); !access.Provisioned || access.Shown().Edit {
		t.Fatalf("the updated starter is provisioned %v, editable %v", access.Provisioned, access.Shown().Edit)
	}
	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE folder_id = $1`, mustFolder(t, r)); n != 4 {
		t.Fatalf("%d dashboards, want 4: an unedited starter gets no copy", n)
	}
}

func mustFolder(t *testing.T, r seedRig) string {
	t.Helper()
	id, _ := r.auditFolder(t)
	return id
}

func TestServersStartingTogetherOnANewReleaseUpdateEachStarterOnce(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	release := r.changedRelease(t, "Data access")
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Go(func() {
			s := r.seeder()
			s.shipped = release
			<-start
			errs[i] = s.Seed(t.Context())
		})
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("server %d: %v", i, err)
		}
	}
	if n := r.latestVersion(t, r.starterID(t, "Data access")); n != 2 {
		t.Fatalf("the starter is at version %d after three servers started, want 2", n)
	}
}

func TestADeletedStarterDashboardComesBackOnTheNextStart(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	folderID, _ := r.auditFolder(t)
	deleted := r.starterID(t, "Data access")
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `DELETE FROM dashboards WHERE id = $1`, deleted); err != nil {
		t.Fatal(err)
	}

	r.seed(t)

	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE folder_id = $1`, folderID); n != 4 {
		t.Fatalf("%d dashboards after a restart, want the 4 starters", n)
	}
	if back := r.starterID(t, "Data access"); back == deleted {
		t.Fatal("the starter row still points at the deleted dashboard")
	}
	if n := r.count(t, `SELECT count(*) FROM starter_content`); n != 5 {
		t.Fatalf("%d starter rows, want 5", n)
	}
}

func (r seedRig) editAsAdmin(t *testing.T, id, title string) {
	t.Helper()
	parsed, err := db.ParseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	ctx := datasource.WithAccess(t.Context(), datasource.Access{By: admin})
	_, ver, err := r.dashboards.GetVersion(ctx, parsed, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(ver.Spec)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["title"] = title
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.dashboards.UpdateProvisioned(ctx, admin, dashboards.ReleaseUpdate{DashboardID: parsed, Parent: 1, Spec: raw, Summary: "Rename", Home: r.folderID(t)}); err != nil {
		t.Fatalf("an Admin's edit made before starters were read-only: %v", err)
	}
}

func TestAnEditedSeededCopyIsKeptAsTheAdminsOwnDashboardWhenTheStarterTakesOver(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	folderID, _ := r.auditFolder(t)
	original := r.starterID(t, "Audit overview")
	r.editAsAdmin(t, original, "Our audit overview")
	r.editAsAdmin(t, r.starterID(t, "Agent and MCP activity"), "Agent and MCP activity")

	r.seed(t)
	r.seed(t)

	if got := r.starterID(t, "Audit overview"); got != original {
		t.Fatalf("the starter is now %s, want the same dashboard %s so its threads and pins stay", got, original)
	}
	if n := r.latestVersion(t, original); n != 3 {
		t.Fatalf("the starter is at version %d, want 3: the Admin's edit, then the shipped content", n)
	}
	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE folder_id = $1`, folderID); n != 5 {
		t.Fatalf("%d dashboards in the Audit folder, want the 4 starters and the Admin's copy", n)
	}
	var copyID string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM dashboards WHERE title = 'Our audit overview (your copy)' AND folder_id = $1`, folderID).Scan(&copyID); err != nil {
		t.Fatalf("the Admin's copy: %v", err)
	}
	if access := r.accessOf(t, copyID); access.Provisioned || !access.Shown().Edit {
		t.Fatalf("the copy is provisioned %v, editable %v, want an ordinary editable dashboard", access.Provisioned, access.Shown().Edit)
	}
	if n := r.count(t, `SELECT count(*) FROM starter_content WHERE ref = $1`, copyID); n != 0 {
		t.Fatal("the copy was recorded as a starter")
	}
}

func TestDuplicatingAStarterMakesAnEditableDashboardInTheAuditFolder(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	folderID, _ := r.auditFolder(t)
	starter, err := db.ParseUUID(r.starterID(t, "Data access"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := datasource.WithAccess(t.Context(), datasource.Access{By: admin})
	folder, err := db.ParseUUID(folderID)
	if err != nil {
		t.Fatal(err)
	}
	copied, ver, err := r.dashboards.Duplicate(ctx, admin, starter, folder, "My data access")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(copied.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.accessOf(t, copied.ID); got.Provisioned || !got.Shown().Edit || got.Folder.Title != Title {
		t.Fatalf("the copy: provisioned %v, editable %v, folder %q", got.Provisioned, got.Shown().Edit, got.Folder.Title)
	}
	raw, err := json.Marshal(ver.Spec)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "My data access", "Changed", 1))
	if _, err := r.dashboards.Save(ctx, admin, dashboards.SaveInput{DashboardID: id, Parent: 1, Spec: raw, Summary: "Rename"}); err != nil {
		t.Fatalf("saving the copy: %v", err)
	}
	if _, err := r.dashboards.Save(ctx, admin, dashboards.SaveInput{DashboardID: starter, Parent: 1, Spec: raw, Summary: "Rename"}); !errors.Is(err, folders.ErrProvisioned) {
		t.Fatalf("saving the starter: %v, want the provisioned refusal", err)
	}
}

func TestADeletedAuditFolderIsNotCreatedAgain(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	folderID, _ := r.auditFolder(t)
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `DELETE FROM dashboards WHERE folder_id = $1`, folderID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `DELETE FROM folders WHERE id = $1`, folderID); err != nil {
		t.Fatal(err)
	}
	r.seed(t)
	if n := r.count(t, `SELECT count(*) FROM folders WHERE title = 'Audit'`); n != 0 {
		t.Fatalf("%d Audit folders after the Admin deleted it, want 0", n)
	}
}

func TestAnAuditFolderAnAdminMadeIsUsedOnlyWhenItIsAdminsOnly(t *testing.T) {
	for _, adminsOnly := range []bool{true, false} {
		r := newSeedRig(t)
		var id string
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(),
			`INSERT INTO folders (title, created_by_kind, created_by_id, created_by_name, admins_only) VALUES ('Audit', 'user', $1, 'Ada', $2) RETURNING id::text`,
			admin.ID, adminsOnly).Scan(&id); err != nil {
			t.Fatal(err)
		}
		r.seed(t)
		want := 0
		if adminsOnly {
			want = 4
		}
		if n := r.count(t, `SELECT count(*) FROM dashboards WHERE folder_id = $1`, id); n != want {
			t.Errorf("admins only %v: %d dashboards in the Admin's Audit folder, want %d", adminsOnly, n, want)
		}
		if n := r.count(t, `SELECT count(*) FROM folders WHERE title = 'Audit'`); n != 1 {
			t.Errorf("admins only %v: %d Audit folders, want only the Admin's", adminsOnly, n)
		}
	}
}

func (r seedRig) folderID(t *testing.T) pgtype.UUID {
	t.Helper()
	id, _ := r.auditFolder(t)
	parsed, err := db.ParseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func (r seedRig) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestAReleaseThatResizesAPanelAddsOneVersionOverTwoStarts(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	overview := r.starterID(t, "Audit overview")
	starters, err := Starters()
	if err != nil {
		t.Fatal(err)
	}
	for i, st := range starters {
		if st.Title != "Audit overview" {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(st.Spec, &doc); err != nil {
			t.Fatal(err)
		}
		panels := doc["panels"].([]any)
		pos := panels[len(panels)-1].(map[string]any)["gridPos"].(map[string]any)
		pos["h"] = pos["h"].(float64) + 2
		if starters[i].Spec, err = json.Marshal(doc); err != nil {
			t.Fatal(err)
		}
	}
	release := func() ([]Starter, error) { return starters, nil }

	r.seedRelease(t, release)
	r.seedRelease(t, release)
	r.seedRelease(t, release)

	if n := r.latestVersion(t, overview); n != 2 {
		t.Fatalf("the resized starter is at version %d, want 2", n)
	}
}

func (r seedRig) moveToOpsAsEdited(t *testing.T, id string) string {
	t.Helper()
	var ops string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(),
		`INSERT INTO folders (title, created_by_kind, created_by_id, created_by_name) VALUES ('Ops', 'user', $1, 'Ada') RETURNING id::text`, admin.ID).Scan(&ops); err != nil {
		t.Fatal(err)
	}
	plain := `{"specVersion":"0","time":{"from":"now-15m","to":"now"},"title":"Audit overview","tags":[],"panels":[{"id":"n","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":6,"h":4},"content":"mine"}]}`
	r.exec(t, `UPDATE dashboard_versions SET author_kind = 'user', author_id = $2, spec = $3::jsonb WHERE dashboard_id = $1 AND version = 1`, id, admin.ID, plain)
	r.exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, id, ops)
	return ops
}

func TestAnEditedStarterMovedToAnotherFolderComesBackToAuditAndItsCopyStaysWhereItWas(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	folderID, _ := r.auditFolder(t)
	overview := r.starterID(t, "Audit overview")
	ops := r.moveToOpsAsEdited(t, overview)

	r.seed(t)
	r.seed(t)
	r.seed(t)

	if got := r.count(t, `SELECT count(*) FROM dashboards WHERE id = $1 AND folder_id = $2`, overview, folderID); got != 1 {
		t.Fatal("the starter is not back in the Audit folder")
	}
	if n := r.latestVersion(t, overview); n != 2 {
		t.Fatalf("the starter is at version %d, want 2", n)
	}
	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE title = 'Audit overview (your copy)'`); n != 1 {
		t.Fatalf("%d copies, want 1", n)
	}
	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE title = 'Audit overview (your copy)' AND folder_id = $1`, ops); n != 1 {
		t.Fatal("the copy is not in the folder the Admin had moved the starter to")
	}
}

func TestAnEditedStarterMovedToAnotherFolderWithAdminOnlyContentKeepsItsCopyInAudit(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	folderID, _ := r.auditFolder(t)
	overview := r.starterID(t, "Audit overview")
	ops := r.moveToOpsAsEdited(t, overview)
	r.exec(t, `UPDATE dashboard_versions SET spec = $2::jsonb WHERE dashboard_id = $1 AND version = 1`, overview,
		`{"specVersion":"0","time":{"from":"now-15m","to":"now"},"title":"Audit overview","tags":[],"panels":[{"id":"events","type":"stat","title":"Events","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"slashobserve-audit","queries":[{"query":"*"}]}]}`)

	r.seed(t)
	r.seed(t)

	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE title = 'Audit overview (your copy)' AND folder_id = $1`, folderID); n != 1 {
		t.Fatalf("%d copies in the Audit folder, want 1", n)
	}
	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE folder_id = $1`, ops); n != 0 {
		t.Fatalf("%d dashboards left in the other folder, want 0", n)
	}
}

func TestAnAdminsOwnDashboardWithAStarterTitleIsNotTakenOver(t *testing.T) {
	r := newSeedRig(t)
	var folderID string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(),
		`INSERT INTO folders (title, created_by_kind, created_by_id, created_by_name, admins_only) VALUES ('Audit', 'user', $1, 'Ada', true) RETURNING id::text`, admin.ID).Scan(&folderID); err != nil {
		t.Fatal(err)
	}
	parentFolder, err := db.ParseUUID(folderID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := datasource.WithAccess(t.Context(), datasource.Access{By: admin})
	created, _, err := r.dashboards.Create(ctx, admin, parentFolder, []byte(`{"specVersion":"0","time":{"from":"now-15m","to":"now"},"title":"Audit overview","tags":[],"panels":[{"id":"n","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":6,"h":4},"content":"mine"}]}`), "mine")
	if err != nil {
		t.Fatal(err)
	}
	own := created.ID

	r.seed(t)
	r.seed(t)

	if n := r.count(t, `SELECT count(*) FROM starter_content WHERE ref = $1`, own); n != 0 {
		t.Fatal("the Admin's own dashboard was taken over as a starter")
	}
	if n := r.latestVersion(t, own); n != 1 {
		t.Fatalf("the Admin's own dashboard is at version %d, want 1", n)
	}
	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE title = 'Audit overview'`); n != 2 {
		t.Fatalf("%d dashboards titled Audit overview, want the Admin's and the starter", n)
	}
	if n := r.count(t, `SELECT count(*) FROM starter_content`); n != 5 {
		t.Fatalf("%d starter rows, want 5", n)
	}
}
