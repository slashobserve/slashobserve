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
