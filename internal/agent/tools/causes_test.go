package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/findings"
)

func (r *findingRig) saveInvestigation(t *testing.T, body string) (Result, findings.Finding) {
	t.Helper()
	run := investigateRun(findingWindowTo.Add(-2*time.Hour), findingWindowTo)
	return r.saveIn(WithRun(auth.WithPrincipal(t.Context(), editorCaller), run), t, body)
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

func TestALegacyRootCauseIsSavedAsOneCause(t *testing.T) {
	r := newFindingRig(t)
	body := `{"markdown":"Cart errors peaked at {{f1}} per second.","onset":"` + findingWindowTo.Add(-30*time.Minute).Format(time.RFC3339) + `","confidence":"high",
		"root_cause":{"component":"cart","kind":"deploy","summary":"A deploy broke cart."}}`
	_, saved := r.saveInvestigation(t, body)
	if c := saved.Structured.Causes; len(c) != 1 || !c[0].Stated || c[0].Score != nil || c[0].Estimate != "high" {
		t.Errorf("legacy cause %+v", c)
	}
	if saved.Structured.RootCause == nil || saved.Structured.RootCause.Summary != "A deploy broke cart." {
		t.Errorf("root cause %+v", saved.Structured.RootCause)
	}
}

func TestARootCauseThatDisagreesWithTheCausesIsRefused(t *testing.T) {
	r := newFindingRig(t)
	body := `{"markdown":"m","root_cause":{"component":"checkout","kind":"deploy","summary":"s"},
		"causes":[{"component":{"kind":"service","name":"cart"},"kind":"deploy","mechanism":"m"}]}`
	res, _ := r.saveInvestigation(t, body)
	if !res.IsError || !strings.Contains(string(mustJSON(res.Content)), "root_cause must describe causes[0]") {
		t.Errorf("result %#v", res.Content)
	}
}
