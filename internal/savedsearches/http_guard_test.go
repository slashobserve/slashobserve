package savedsearches_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

type scriptedGuard struct {
	asked    int
	existing *savedsearches.SavedSearch
	refusal  error
}

func (g *scriptedGuard) GuardSavedSearch(context.Context, auth.Principal, savedsearches.CreateInput) (*savedsearches.SavedSearch, error) {
	g.asked++
	return g.existing, g.refusal
}

func TestCreateRunsTheSaveGuardForKeysOnly(t *testing.T) {
	const body = `{"kind":"logs","title":"Errors","datasource_uid":"loki","query":"{app=\"x\"}"}`
	t.Run("a refusal of the guard answers 422 and saves nothing", func(t *testing.T) {
		guard := &scriptedGuard{refusal: &savedsearches.InvalidInputError{Code: savedsearches.CodeQueryInvalid, Message: "the query does not pass the lint check"}}
		d := newDocumented(t, savedsearches.WithSaveGuard(guard))
		rec := d.send(t, pipeline, http.MethodPost, "/api/v1/saved-searches", body, http.StatusUnprocessableEntity)
		if code := errorCode(t, rec); code != savedsearches.CodeQueryInvalid {
			t.Fatalf("code %q", code)
		}
		list := d.send(t, pipeline, http.MethodGet, "/api/v1/saved-searches", "", http.StatusOK)
		if strings.Contains(list.Body.String(), "Errors") {
			t.Fatalf("the refused search was saved: %s", list.Body)
		}
	})
	t.Run("an existing search answers 200 with replayed and no second row", func(t *testing.T) {
		guard := &scriptedGuard{}
		d := newDocumented(t, savedsearches.WithSaveGuard(guard))
		d.send(t, editor, http.MethodPost, "/api/v1/saved-searches", body, http.StatusCreated)
		saved, _, err := d.service.List(t.Context(), editor, savedsearches.ListFilter{}, 10, "")
		if err != nil || len(saved) != 1 {
			t.Fatalf("saved %v, %v", saved, err)
		}
		guard.existing = &saved[0]
		rec := d.send(t, pipeline, http.MethodPost, "/api/v1/saved-searches", body, http.StatusOK)
		if !strings.Contains(rec.Body.String(), `"replayed":true`) || !strings.Contains(rec.Body.String(), saved[0].ID) {
			t.Fatalf("replay %s", rec.Body)
		}
		if again, _, err := d.service.List(t.Context(), editor, savedsearches.ListFilter{}, 10, ""); err != nil || len(again) != 1 {
			t.Fatalf("rows after the replay %v, %v", again, err)
		}
	})
	t.Run("a person's session does not ask the guard", func(t *testing.T) {
		guard := &scriptedGuard{refusal: &savedsearches.InvalidInputError{Code: savedsearches.CodeQueryInvalid, Message: "no"}}
		d := newDocumented(t, savedsearches.WithSaveGuard(guard))
		d.send(t, editor, http.MethodPost, "/api/v1/saved-searches", body, http.StatusCreated)
		if guard.asked != 0 {
			t.Fatalf("the guard was asked %d times", guard.asked)
		}
	})
}
