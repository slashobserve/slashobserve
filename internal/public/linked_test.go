package public

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func (r rig) dashboardService(t *testing.T) *dashboards.Service {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger)
	if err != nil {
		t.Fatal(err)
	}
	return dashboards.New(r.db.DB, queryproxy.NewRunner(registry, logger), folders.NewAccess(r.db.DB), logger)
}

func (r rig) foldersAsAdmin() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), admin))
	})
	folders.NewHandler(r.db.DB, folders.NewAccess(r.db.DB), slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	return engine
}

func makeAdminsOnly(engine *gin.Engine, folder pgtype.UUID) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/folders/"+folder.String()+"/permissions", strings.NewReader(`{"entries":[],"admins_only":true}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	return rec
}

func TestADashboardWithALiveLinkCannotMoveIntoAnAdminsOnlyFolderUntilTheLinkIsRevoked(t *testing.T) {
	r := newRig(t)
	link := r.create(t, CreateInput{Mode: ModeFrozen})
	locked := foldertest.AdminsOnly(t, r.db, "Payroll")
	dash := r.dashboardService(t)

	if _, _, err := dash.Move(reading(t.Context()), admin, r.shared, locked); !errors.Is(err, folders.ErrLinkedMove) {
		t.Fatalf("move = %v, want %v", err, folders.ErrLinkedMove)
	}
	linkID, err := db.ParseUUID(link.Link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.service.Delete(reading(t.Context()), admin, r.shared, linkID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := dash.Move(reading(t.Context()), admin, r.shared, locked); err != nil {
		t.Fatalf("move after revoking = %v", err)
	}
}

func TestAFolderHoldingALinkedDashboardCannotBeMadeAdminsOnly(t *testing.T) {
	r := newRig(t)
	team := foldertest.Open(t, r.db, "Team")
	foldertest.Move(t, r.db, r.shared, team)
	r.create(t, CreateInput{Mode: ModeFrozen})

	got := makeAdminsOnly(r.foldersAsAdmin(), team)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), `"admins_only_folder"`) {
		t.Fatalf("admins only = %d %s, want 409 admins_only_folder", got.Code, got.Body)
	}
}

func TestALinkAndAnAdminsOnlySwitchRacingEachOtherNeverBothSucceed(t *testing.T) {
	r := newRig(t)
	engine := r.foldersAsAdmin()
	for round := range 12 {
		folder := foldertest.Open(t, r.db, fmt.Sprintf("Team %d", round))
		dashboard := r.dashboard(t, plainSpec)
		foldertest.Move(t, r.db, dashboard, folder)

		start := make(chan struct{})
		var wg sync.WaitGroup
		var linkErr error
		var switched *httptest.ResponseRecorder
		wg.Go(func() {
			<-start
			_, linkErr = r.service.Create(reading(t.Context()), admin, dashboard, CreateInput{Mode: ModeFrozen})
		})
		wg.Go(func() {
			<-start
			switched = makeAdminsOnly(engine, folder)
		})
		close(start)
		wg.Wait()

		switch {
		case linkErr == nil && switched.Code == http.StatusConflict && strings.Contains(switched.Body.String(), `"admins_only_folder"`):
		case errors.Is(linkErr, ErrAdminsOnly) && switched.Code == http.StatusOK:
		default:
			t.Fatalf("round %d: link = %v, switch = %d %s; want exactly one refused", round, linkErr, switched.Code, switched.Body)
		}
	}
}
