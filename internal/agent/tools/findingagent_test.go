package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
)

const agentFindingBody = `{"markdown":"Cart errors rose to 0.52 per second after the flag turned on.",` +
	`"unverified":[{"kind":"figure","text":"0.52 per second"},{"kind":"made_up","text":"x"}],` +
	`"reads_refused":2,"unchecked":[{"datasource":"logs","code":"query_timeout"}],` +
	`"causes":[{"component":{"kind":"service","name":"payments"},"kind":"unknown","mechanism":"Payments slowed down.","score":0.2},` +
	`{"component":{"kind":"service","name":"cart"},"kind":"config_or_flag","mechanism":"The flag made cart fail.",` +
	`"signals":{"kinds":["metrics","logs"],"component":true,"trigger_read":true},"queries":["rate(errors[5m])"," rate(errors[5m]) "]}]}`

func (r *findingRig) saveAs(t *testing.T, builtIn bool, body string) (Result, findings.Finding) {
	t.Helper()
	run := investigateRun(findingWindowTo.Add(-2*time.Hour), findingWindowTo)
	run.BuiltIn = builtIn
	return r.saveIn(WithRun(auth.WithPrincipal(t.Context(), editorCaller), run), t, body)
}

func TestTheBuiltInAgentsFindingFieldsReplaceWhatTheServerWouldWorkOut(t *testing.T) {
	r := newFindingRig(t)
	res, f := r.saveAs(t, true, agentFindingBody)
	if res.IsError {
		t.Fatalf("save: %#v", res.Content)
	}
	if len(f.Unverified) != 1 || f.Unverified[0] != (findings.Unverified{Kind: "figure", Text: "0.52 per second"}) {
		t.Fatalf("unverified = %+v", f.Unverified)
	}
	if !strings.Contains(f.Markdown, "2 reads were refused") || !strings.Contains(f.Markdown, "Could not check logs (a read timed out)") {
		t.Fatalf("markdown = %q", f.Markdown)
	}
	if !strings.HasPrefix(f.Markdown, "Cart errors rose to 0.52 per second") {
		t.Fatalf("rendered markdown was changed: %q", f.Markdown)
	}
	causes := f.Structured.Causes
	if len(causes) != 2 || causes[0].Component.Name != "cart" || causes[1].Component.Name != "payments" {
		t.Fatalf("causes = %+v", causes)
	}
	if *causes[0].Score != 0.7 || causes[0].Band != findings.BandHigh || !causes[0].Stated || len(causes[0].Evidence.Queries) != 1 || causes[0].Evidence.Queries[0] != "rate(errors[5m])" {
		t.Errorf("cart = %+v", causes[0])
	}
	if *causes[1].Score != 0.2 || causes[1].Band != findings.BandLow || causes[1].Stated {
		t.Errorf("payments = %+v", causes[1])
	}
}

func TestAConnectedAgentsFindingFieldsAreIgnored(t *testing.T) {
	r := newFindingRig(t)
	res, f := r.saveAs(t, false, agentFindingBody)
	if res.IsError {
		t.Fatalf("save: %#v", res.Content)
	}
	if len(f.Unverified) != 0 || strings.Contains(f.Markdown, "refused") || strings.Contains(f.Markdown, "Could not check") {
		t.Fatalf("unverified %+v, markdown %q", f.Unverified, f.Markdown)
	}
	for _, c := range f.Structured.Causes {
		if c.Score != nil || len(c.Evidence.Queries) != 0 {
			t.Fatalf("cause scored from a connected agent's fields: %+v", c)
		}
	}
}

func TestTheBuiltInAgentsBandIsKeptAndFollowsItsScoreWhenItSendsNone(t *testing.T) {
	r := newFindingRig(t)
	body := `{"markdown":"Cart failed.","causes":[{"component":{"kind":"service","name":"cart"},"kind":"unknown","mechanism":"Cart failed.","score":0.9,"band":"low"},` +
		`{"component":{"kind":"service","name":"web"},"kind":"unknown","mechanism":"Web failed.","score":0.75}]}`
	_, f := r.saveAs(t, true, body)
	causes := f.Structured.Causes
	if len(causes) != 2 || causes[0].Component.Name != "web" || causes[0].Band != findings.BandHigh || causes[1].Band != findings.BandLow || !causes[1].Stated {
		t.Fatalf("causes = %+v", causes)
	}
}
