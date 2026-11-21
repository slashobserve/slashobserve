package auditfolder

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"

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

func TestAnAdminsEditAndDeleteOfAStarterDashboardOutliveARestart(t *testing.T) {
	r := newSeedRig(t)
	r.seed(t)
	folderID, _ := r.auditFolder(t)
	var edited, deleted string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM dashboards WHERE folder_id = $1 AND title = 'Audit overview'`, folderID).Scan(&edited); err != nil {
		t.Fatal(err)
	}
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM dashboards WHERE folder_id = $1 AND title = 'Data access'`, folderID).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	editedID, err := db.ParseUUID(edited)
	if err != nil {
		t.Fatal(err)
	}
	ctx := datasource.WithAccess(t.Context(), datasource.Access{By: admin})
	_, ver, err := r.dashboards.GetVersion(ctx, editedID, 1)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	raw, err := json.Marshal(ver.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["title"] = "Our audit overview"
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := r.dashboards.AddVersion(ctx, admin, editedID, 1, raw, "Rename"); err != nil {
		t.Fatalf("an Admin's edit: %v", err)
	}
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `DELETE FROM dashboards WHERE id = $1`, deleted); err != nil {
		t.Fatal(err)
	}

	r.seed(t)

	if n := r.count(t, `SELECT latest_version FROM dashboards WHERE id = $1`, edited); n != 2 {
		t.Fatalf("the edited dashboard is at version %d, want the Admin's version 2", n)
	}
	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE folder_id = $1`, folderID); n != 3 {
		t.Fatalf("%d dashboards after a restart, want the 3 the Admin kept", n)
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
