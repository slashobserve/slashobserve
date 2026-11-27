package findings_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/findings"
)

func TestSavedCausesComeBackInOrderAndTheirSectionIsInTheObservation(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, pgtype.UUID{})
	structured, err := findings.StructuredInput{Causes: []findings.CauseInput{
		{Component: findings.ComponentInput{Kind: "flag", Name: "cartFailure"}, Kind: "config_or_flag", Mechanism: "The flag made cart fail.",
			Trigger: &findings.TriggerInput{Kind: "feature_flag", Key: "cartFailure", Value: "on"}},
		{Component: findings.ComponentInput{Kind: "service", Name: "checkout"}, Kind: "dependency", Mechanism: "Checkout failed on cart calls.", KnockOnOf: new(0)},
	}}.Check()
	if err != nil {
		t.Fatal(err)
	}
	saved := r.save(t, editor, findings.SaveInput{AgentRequestID: request, Structured: structured})
	got, err := r.service.Get(reading(t.Context()), editor, mustUUID(t, saved.ID))
	if err != nil {
		t.Fatal(err)
	}
	causes := got.Structured.Causes
	if len(causes) != 2 || causes[0].Trigger == nil || causes[0].Trigger.Key != "cartFailure" || causes[1].KnockOnOf == nil || *causes[1].KnockOnOf != 0 {
		t.Fatalf("causes %+v", causes)
	}
	if got.Structured.RootCause == nil || got.Structured.RootCause.Component != "cartFailure" {
		t.Errorf("root cause %+v, want the first stated cause", got.Structured.RootCause)
	}
	asObservation, _, err := r.service.SaveAsObservation(reading(t.Context()), editor, request)
	if err != nil {
		t.Fatal(err)
	}
	o, err := r.observations.Get(reading(t.Context()), editor, mustUUID(t, asObservation.ObservationID))
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, b := range o.Blocks {
		if b.RunID == request.String() && b.Kind == "text" {
			text = string(b.Body)
		}
	}
	if !strings.Contains(text, "### Causes") || !strings.Contains(text, "knock-on of cartFailure") {
		t.Errorf("conclusion block %s, want the causes section", text)
	}
	if len(o.Findings) != 1 || len(o.Findings[0].Fields.Causes) != 2 {
		t.Errorf("observation finding fields %+v", o.Findings)
	}
}

func TestARowSavedBeforeCausesReadsItsRootCauseAsTheOnlyCause(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, findings.SaveInput{DashboardID: r.open})
	r.db.Exec(t, `UPDATE findings SET root_cause_component = 'checkout', root_cause_kind = 'deploy', root_cause_summary = 'Version 42 broke it.', confidence = 'high', causes = NULL WHERE id = $1::uuid`, saved.ID)
	got, err := r.service.Get(reading(t.Context()), editor, mustUUID(t, saved.ID))
	if err != nil {
		t.Fatal(err)
	}
	causes := got.Structured.Causes
	if len(causes) != 1 || causes[0].Component.Name != "checkout" || causes[0].Kind != "deploy" || causes[0].Mechanism != "Version 42 broke it." || !causes[0].Stated || causes[0].Score != nil || causes[0].Estimate != "high" {
		t.Fatalf("causes %+v, want the stored root cause as one stated, unscored cause", causes)
	}
	if got.Structured.RootCause == nil || got.Structured.RootCause.Component != "checkout" {
		t.Errorf("root cause %+v", got.Structured.RootCause)
	}
}
