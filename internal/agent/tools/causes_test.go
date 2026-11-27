package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/findings"
)

func causesRecord() *grounding.Record {
	r := grounding.NewRecord(grounding.Report)
	r.AddRead([]byte(`{"series":[{"labels":{"service":"cart","flag":"cartFailure"}}]}`), []string{"checkout"},
		[]grounding.Range{{From: findingWindowTo.Add(-2 * time.Hour), To: findingWindowTo}})
	at := findingWindowTo.Add(-30 * time.Minute).Format(time.RFC3339)
	r.IssueFacts("measure_window", nil, []byte(`{"stat":"max","from":"`+findingWindowTo.Add(-2*time.Hour).Format(time.RFC3339)+`","to":"`+findingWindowTo.Format(time.RFC3339)+`","query":"rate(errors[5m])","values":[{"labels":{"service":"cart"},"value":0.52,"at":"`+at+`"}]}`), nil)
	return r
}

func (r *findingRig) saveScored(t *testing.T, rec *grounding.Record, body string) (Result, findings.Finding) {
	t.Helper()
	run := investigateRun(findingWindowTo.Add(-2*time.Hour), findingWindowTo)
	ctx := WithRun(auth.WithPrincipal(t.Context(), editorCaller), run)
	if rec != nil {
		ctx = WithCauseSignals(WithFacts(ctx, rec.Ledger()), rec.CauseSignals(json.RawMessage(body)))
	}
	return r.saveIn(ctx, t, body)
}

func (r *findingRig) saveIn(ctx context.Context, t *testing.T, body string) (Result, findings.Finding) {
	t.Helper()
	res, err := r.registry.Call(ctx, nameSaveFinding, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok {
		return res, findings.Finding{}
	}
	id, err := db.ParseUUID(saved.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := r.findings.Get(reading(t.Context()), editorCaller, id)
	if err != nil {
		t.Fatal(err)
	}
	return res, f
}

func TestSaveFindingScoresEachCauseFromTheEvidenceAndRanksThem(t *testing.T) {
	r := newFindingRig(t)
	onset := findingWindowTo.Add(-31 * time.Minute).Format(time.RFC3339)
	body := `{"markdown":"Cart errors rose after the flag turned on.","causes":[
		{"component":{"kind":"service","name":"payments"},"kind":"unknown","mechanism":"Maybe payments slowed down.","estimate":"high"},
		{"component":{"kind":"service","name":"cart"},"kind":"config_or_flag","mechanism":"The flag made cart fail at {{f1}} per second.","onset":"` + onset + `",
		 "trigger":{"kind":"feature_flag","key":"cartFailure","value":"on"},"evidence":{"facts":["f1"]},"estimate":"high"},
		{"component":{"kind":"service","name":"checkout"},"kind":"dependency","mechanism":"Checkout failed on its cart calls.","knock_on_of":1,"evidence":{"facts":["f1"]},"onset":"` + onset + `"}]}`
	res, f := r.saveScored(t, causesRecord(), body)
	saved, ok := res.Content.(findingSavedOut)
	if !ok {
		t.Fatalf("save: %#v", res.Content)
	}
	causes := f.Structured.Causes
	if len(causes) != 3 || causes[0].Component.Name != "cart" || causes[1].Component.Name != "checkout" || causes[2].Component.Name != "payments" {
		t.Fatalf("order %+v", causes)
	}
	if causes[0].Band != findings.BandHigh || *causes[0].Score != 0.7 || !causes[0].Stated || causes[0].Mechanism != "The flag made cart fail at 0.52 per second." {
		t.Errorf("cart %+v", causes[0])
	}
	if causes[0].Evidence.Queries[0] != "rate(errors[5m])" {
		t.Errorf("queries %+v", causes[0].Evidence)
	}
	if causes[1].KnockOnOf == nil || *causes[1].KnockOnOf != 0 || !causes[1].Stated {
		t.Errorf("checkout %+v", causes[1])
	}
	if causes[2].Band != findings.BandLow || causes[2].Stated {
		t.Errorf("payments %+v", causes[2])
	}
	if f.Structured.RootCause == nil || f.Structured.RootCause.Component != "cart" {
		t.Errorf("root cause %+v", f.Structured.RootCause)
	}
	if !strings.Contains(saved.Note, "causes[0] (payments): you estimated high, the evidence scores low") || !strings.Contains(saved.Note, "listed as unconfirmed") {
		t.Errorf("note %q", saved.Note)
	}
	if len(saved.Causes) != 3 || saved.Causes[2].Stated || *saved.Causes[0].Band != findings.BandHigh {
		t.Errorf("result causes %+v", saved.Causes)
	}
}

func TestALegacyRootCauseIsSavedAsOneCause(t *testing.T) {
	r := newFindingRig(t)
	body := `{"markdown":"Cart errors peaked at {{f1}} per second.","onset":"` + findingWindowTo.Add(-30*time.Minute).Format(time.RFC3339) + `","confidence":"high",
		"root_cause":{"component":"cart","kind":"deploy","summary":"A deploy broke cart."}}`
	_, off := r.saveScored(t, nil, body)
	if c := off.Structured.Causes; len(c) != 1 || !c[0].Stated || c[0].Score != nil || c[0].Estimate != "high" || off.Structured.RootCause == nil {
		t.Errorf("without grounding: %+v, root cause %+v", c, off.Structured.RootCause)
	}
	r2 := newFindingRig(t)
	_, scored := r2.saveScored(t, causesRecord(), body)
	c := scored.Structured.Causes
	if len(c) != 1 || c[0].Evidence.Facts[0] != "f1" || c[0].Score == nil || *c[0].Score != 0.5 || !c[0].Stated {
		t.Fatalf("scored legacy cause %+v", c)
	}
	if scored.Structured.RootCause == nil || scored.Structured.RootCause.Summary != "A deploy broke cart." {
		t.Errorf("root cause %+v", scored.Structured.RootCause)
	}
}

func TestARootCauseThatDisagreesWithTheCausesIsRefused(t *testing.T) {
	r := newFindingRig(t)
	body := `{"markdown":"m","root_cause":{"component":"checkout","kind":"deploy","summary":"s"},
		"causes":[{"component":{"kind":"service","name":"cart"},"kind":"deploy","mechanism":"m"}]}`
	res, _ := r.saveScored(t, nil, body)
	if !res.IsError || !strings.Contains(string(mustJSON(res.Content)), "root_cause must describe causes[0]") {
		t.Errorf("result %#v", res.Content)
	}
}
