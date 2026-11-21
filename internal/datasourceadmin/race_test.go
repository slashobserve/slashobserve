package datasourceadmin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

func TestASaveAndAnAudienceChangeRacingEachOtherNeverBothSucceed(t *testing.T) {
	h := newHTTPRig(t)
	if created := h.call(t, http.MethodPost, "/api/v1/datasources", createBody("payroll", upstream(t, new(string)).URL)); created.Code != http.StatusCreated {
		t.Fatalf("create answered %d: %s", created.Code, created.Body)
	}
	general, err := db.ParseUUID(h.scalar(t, `SELECT general_folder_id()::text`))
	if err != nil {
		t.Fatal(err)
	}
	searches := savedsearches.New(h.db, h.access, h.registry, nil, slog.New(slog.DiscardHandler))
	ctx := datasource.WithAccess(auth.WithPrincipal(reading(t.Context()), h.admin), datasource.Access{By: h.admin})
	query := "up"

	for round := range 12 {
		if opened := h.call(t, http.MethodPatch, "/api/v1/datasources/payroll", `{"audience":"everyone"}`); opened.Code != http.StatusOK {
			t.Fatalf("round %d: reopen = %d %s", round, opened.Code, opened.Body)
		}
		h.database.Exec(t, `DELETE FROM saved_searches`)

		start := make(chan struct{})
		var wg sync.WaitGroup
		var saveErr error
		patch := httptest.NewRecorder()
		wg.Go(func() {
			<-start
			_, saveErr = searches.Create(ctx, h.admin, savedsearches.CreateInput{
				Kind: savedsearches.KindMetrics, Title: "Salaries", DatasourceUID: "payroll", Query: &query, From: "now-1h", To: "now", FolderID: general,
			})
		})
		wg.Go(func() {
			<-start
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/datasources/payroll", strings.NewReader(`{"audience":"admins"}`))
			req.Header.Set("Content-Type", "application/json")
			h.engine.ServeHTTP(patch, req)
		})
		close(start)
		wg.Wait()

		var restricted *folders.RestrictedContentError
		saved := saveErr == nil
		switch {
		case saved && patch.Code == http.StatusConflict && strings.Contains(patch.Body.String(), `"restricted_content"`):
		case errors.As(saveErr, &restricted) && patch.Code == http.StatusOK:
		default:
			t.Fatalf("round %d: save = %v, audience change = %d %s; want exactly one refused", round, saveErr, patch.Code, patch.Body)
		}
	}
}

func TestASaveAndTheCreateOfAnAdminsOnlyDatasourceItNamesNeverBothSucceed(t *testing.T) {
	h := newHTTPRig(t)
	general, err := db.ParseUUID(h.scalar(t, `SELECT general_folder_id()::text`))
	if err != nil {
		t.Fatal(err)
	}
	notes := observations.New(h.db, h.access, nil, nil, slog.New(slog.DiscardHandler))
	ctx := datasource.WithAccess(auth.WithPrincipal(reading(t.Context()), h.admin), datasource.Access{By: h.admin})
	address := upstream(t, new(string)).URL

	for round := range 12 {
		uid := fmt.Sprintf("payroll-%d", round)
		o, err := notes.Create(ctx, h.admin, observations.CreateInput{FolderID: general, Title: fmt.Sprintf("Salaries %d", round)})
		if err != nil {
			t.Fatal(err)
		}
		id, err := db.ParseUUID(o.ID)
		if err != nil {
			t.Fatal(err)
		}
		panel := json.RawMessage(fmt.Sprintf(`{"panel":{"type":"timeseries","title":"Salaries","datasource":%q,"queries":[{"ref":"A","query":"sum(payroll)"}]}}`, uid))
		body := strings.Replace(createBody(uid, address), `{`, `{"audience":"admins",`, 1)

		start := make(chan struct{})
		var wg sync.WaitGroup
		var saveErr error
		created := httptest.NewRecorder()
		wg.Go(func() {
			<-start
			_, saveErr = notes.AddBlock(ctx, h.admin, id, observations.BlockInput{Kind: observations.KindPanel, Body: panel})
		})
		wg.Go(func() {
			<-start
			req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			h.engine.ServeHTTP(created, req)
		})
		close(start)
		wg.Wait()

		var restricted *folders.RestrictedContentError
		switch {
		case saveErr == nil && created.Code == http.StatusConflict && strings.Contains(created.Body.String(), `"restricted_content"`):
		case errors.As(saveErr, &restricted) && created.Code == http.StatusCreated:
		default:
			t.Fatalf("round %d: save = %v, create = %d %s; want exactly one refused", round, saveErr, created.Code, created.Body)
		}
	}
}
