package public

import (
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type typed struct {
	datasource.Datasource
}

func (typed) Type() string { return "prometheus" }

func newAdminsOnlyRig(t *testing.T) rig {
	t.Helper()
	r := newRig(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: typed{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: typed{}}); err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(r.db.DB)
	dash := dashboards.New(r.db.DB, queryproxy.NewRunner(registry, logger), access, logger)
	r.service = New(r.db.DB, access, dash, registry, &url.URL{Scheme: "https", Host: "observe.example"}, logger)
	return r
}

func TestAFolderOnlyAdminsCanReadTakesNoPublicLink(t *testing.T) {
	r := newAdminsOnlyRig(t)
	locked := r.dashboard(t, plainSpec)
	foldertest.Move(t, r.db, locked, foldertest.AdminsOnly(t, r.db, "Payroll"))
	if _, err := r.service.Create(reading(t.Context()), admin, locked, CreateInput{Mode: ModeFrozen}); !errors.Is(err, ErrAdminsOnly) {
		t.Fatalf("err = %v, want %v", err, ErrAdminsOnly)
	}
}

func TestAPublicLinkStopsAnsweringOnceItsDashboardReadsAnAdminsOnlyDatasource(t *testing.T) {
	r := newAdminsOnlyRig(t)
	link := r.create(t, CreateInput{Mode: ModeFree})
	if _, err := r.service.View(reading(t.Context()), link.Token); err != nil {
		t.Fatalf("view before = %v", err)
	}
	r.version(t, r.shared, 2, strings.Replace(plainSpec, `"datasource":"prom"`, `"datasource":"audit"`, 1))
	if _, err := r.service.View(reading(t.Context()), link.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("view after = %v, want %v", err, ErrNotFound)
	}
}

func TestAnAdminsOnlyFolderHidesItsPublicLinksFromItsNonAdminCreator(t *testing.T) {
	r := newAdminsOnlyRig(t)
	locked := r.dashboard(t, plainSpec)
	foldertest.Move(t, r.db, locked, foldertest.AdminsOnlyMadeBy(t, r.db, "Payroll", editor))
	if _, err := r.service.List(reading(t.Context()), editor, locked); !errors.Is(err, folders.ErrNoAccess) {
		t.Fatalf("list = %v, want %v", err, folders.ErrNoAccess)
	}
	if _, err := r.service.Create(reading(t.Context()), editor, locked, CreateInput{Mode: ModeFrozen}); !errors.Is(err, folders.ErrNoAccess) {
		t.Fatalf("create = %v, want %v", err, folders.ErrNoAccess)
	}
}

func TestTheRefusalToLinkADashboardSaysWhereItCanGo(t *testing.T) {
	r := newAdminsOnlyRig(t)
	plain := r.dashboard(t, plainSpec)
	foldertest.Move(t, r.db, plain, foldertest.AdminsOnly(t, r.db, "Payroll"))
	auditSpec := strings.Replace(plainSpec, `"datasource":"prom"`, `"datasource":"audit"`, 1)
	audit := r.dashboard(t, auditSpec)
	foldertest.Move(t, r.db, audit, foldertest.AdminsOnly(t, r.db, "Salaries"))

	_, err := r.service.Create(reading(t.Context()), admin, plain, CreateInput{Mode: ModeFrozen})
	if !errors.Is(err, ErrAdminsOnly) || adminsOnlyDatasource(err) != "" || !strings.Contains(err.Error(), "not Admins only") {
		t.Errorf("a dashboard that can move: %v", err)
	}
	_, err = r.service.Create(reading(t.Context()), admin, audit, CreateInput{Mode: ModeFrozen})
	if !errors.Is(err, ErrAdminsOnly) || adminsOnlyDatasource(err) != "audit" ||
		!strings.Contains(err.Error(), `"audit"`) || strings.Contains(err.Error(), "move it") {
		t.Errorf("a dashboard that cannot move: %v", err)
	}
}

func TestTheRefusalToLinkAnRCASaysWhereItCanGo(t *testing.T) {
	r := newRCARig(t)
	id := r.published(t)
	r.db.Exec(t, `UPDATE folders SET admins_only = true WHERE id = $1`, r.folder)

	_, err := r.service.CreateRCALink(reading(t.Context()), admin, id, 1, CreateRCAInput{})
	if !errors.Is(err, ErrRCAAdminsOnly) || adminsOnlyDatasource(err) != "" || !strings.Contains(err.Error(), "not Admins only") {
		t.Errorf("an observation that can move: %v", err)
	}
	r.db.Exec(t, `INSERT INTO datasources (uid, type, name, config, audience) VALUES ('prom', 'prometheus', 'Prom', '{}', 'admins')`)
	_, err = r.service.CreateRCALink(reading(t.Context()), admin, id, 1, CreateRCAInput{})
	if !errors.Is(err, ErrRCAAdminsOnly) || adminsOnlyDatasource(err) != "prom" ||
		!strings.Contains(err.Error(), `"prom"`) || strings.Contains(err.Error(), "move it") {
		t.Errorf("an observation that cannot move: %v", err)
	}
}
