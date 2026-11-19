package findings_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAFindingCarriesTheDashboardToAskOnWithItsTitle(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, pgtype.UUID{})
	saved := r.save(t, editor, findings.SaveInput{AgentRequestID: request, AskDashboardID: r.open})
	if saved.AskOnID != r.open.String() || saved.DashboardID != "" {
		t.Fatalf("saved finding: ask %q dashboard %q, want ask %s and no dashboard", saved.AskOnID, saved.DashboardID, r.open.String())
	}
	got, found, err := r.service.ForRequest(t.Context(), editor, request)
	if err != nil || !found {
		t.Fatalf("for request: found %v, %v", found, err)
	}
	if got.AskOnID != r.open.String() || got.AskOnTitle != "Checkout" {
		t.Errorf("ask on = %q %q, want %s Checkout", got.AskOnID, got.AskOnTitle, r.open.String())
	}
	json := findings.JSONOf(got, "")
	if json.AskOn == nil || json.AskOn.DashboardID != r.open.String() || json.AskOn.Title != "Checkout" {
		t.Errorf("json ask_on = %+v", json.AskOn)
	}
}

func TestAFindingWithoutAnAskTargetHasNone(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, pgtype.UUID{})
	r.save(t, editor, findings.SaveInput{AgentRequestID: request})
	got, _, err := r.service.ForRequest(t.Context(), editor, request)
	if err != nil || got.AskOnID != "" || findings.JSONOf(got, "").AskOn != nil {
		t.Fatalf("ask on = %q, json %+v, %v", got.AskOnID, findings.JSONOf(got, "").AskOn, err)
	}
}

func TestTheAskTargetIsDroppedForAReaderWhoCannotViewTheDashboard(t *testing.T) {
	r := newRig(t)
	hidden := r.dashboard(t, "Payroll")
	request := r.request(t, editor, pgtype.UUID{})
	r.save(t, editor, findings.SaveInput{AgentRequestID: request, AskDashboardID: hidden})
	foldertest.Move(t, r.db, hidden, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))
	got, found, err := r.service.ForRequest(t.Context(), editor, request)
	if err != nil || !found {
		t.Fatalf("for request: found %v, %v", found, err)
	}
	if got.AskOnID != "" || got.AskOnTitle != "" {
		t.Errorf("a dashboard the reader cannot view leaked: %q %q", got.AskOnID, got.AskOnTitle)
	}
}

func TestSavingWithAnAskTargetTheCallerCannotViewIsNotFound(t *testing.T) {
	r := newRig(t)
	hidden := r.dashboard(t, "Payroll")
	foldertest.Move(t, r.db, hidden, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))
	_, _, err := r.service.Save(t.Context(), editor, findings.SaveInput{AskDashboardID: hidden, Title: "t", Markdown: "m", From: windowTo.Add(-1), To: windowTo})
	if !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("save with a hidden ask target: %v", err)
	}
}

func TestAFindingAboutADashboardTakesNoAskTarget(t *testing.T) {
	r := newRig(t)
	other := r.dashboard(t, "Search")
	_, _, err := r.service.Save(t.Context(), editor, findings.SaveInput{DashboardID: r.open, AskDashboardID: other, Title: "t", Markdown: "m", From: windowTo.Add(-1), To: windowTo})
	if !errors.Is(err, findings.ErrAskPlacement) {
		t.Fatalf("save with a dashboard and an ask target: %v", err)
	}
}
