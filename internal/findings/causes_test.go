package findings

import (
	"strings"
	"testing"
	"time"
)

func cause(name, kind string) CauseInput {
	return CauseInput{Component: ComponentInput{Kind: "service", Name: name}, Kind: kind, Mechanism: name + " failed"}
}

func knockOn(c CauseInput, of int) CauseInput {
	c.KnockOnOf = &of
	return c
}

func TestCausesAreValidatedWithAMessageThatNamesTheField(t *testing.T) {
	many := make([]CauseInput, MaxCauses+1)
	for i := range many {
		many[i] = cause("svc", "deploy")
	}
	noName := cause("", "deploy")
	badComponentKind := cause("cart", "deploy")
	badComponentKind.Component.Kind = "cluster"
	noMechanism := cause("cart", "deploy")
	noMechanism.Mechanism = " "
	keyless := cause("cart", "config_or_flag")
	keyless.Trigger = &TriggerInput{Kind: "feature_flag"}
	badTrigger := cause("cart", "config_or_flag")
	badTrigger.Trigger = &TriggerInput{Kind: "flag", Key: "cartFailure"}
	badFact := cause("cart", "deploy")
	badFact.Evidence.Facts = []string{"fact one"}
	badTrace := cause("cart", "deploy")
	badTrace.Evidence.TraceIDs = []string{"xyz"}
	badEstimate := cause("cart", "deploy")
	badEstimate.Estimate = "certain"
	badOnset := cause("cart", "deploy")
	badOnset.Onset = "now-5m"
	cases := map[string]struct {
		in   StructuredInput
		want string
	}{
		"too many causes":        {StructuredInput{Causes: many}, "causes takes at most 6"},
		"no incident and causes": {StructuredInput{NoIncident: true, Causes: []CauseInput{cause("cart", "deploy")}}, "no_incident is true, so causes"},
		"knock-on of itself":     {StructuredInput{Causes: []CauseInput{cause("cart", "deploy"), knockOn(cause("checkout", "dependency"), 1)}}, "causes[1].knock_on_of must be the index of another listed cause, from 0 to 1, not 1"},
		"knock-on out of range":  {StructuredInput{Causes: []CauseInput{cause("cart", "deploy"), knockOn(cause("checkout", "dependency"), 2)}}, "causes[1].knock_on_of must be the index"},
		"negative knock-on":      {StructuredInput{Causes: []CauseInput{knockOn(cause("cart", "deploy"), -1)}}, "causes[0].knock_on_of must be the index"},
		"knock-on of a knock-on": {StructuredInput{Causes: []CauseInput{cause("cart", "deploy"), knockOn(cause("checkout", "dependency"), 0), knockOn(cause("frontend", "dependency"), 1)}}, "causes[2].knock_on_of must name a cause that is not itself a knock-on"},
		"no component name":      {StructuredInput{Causes: []CauseInput{noName}}, "causes[0].component.name is required"},
		"unknown component kind": {StructuredInput{Causes: []CauseInput{badComponentKind}}, "causes[0].component.kind must be one of"},
		"no mechanism":           {StructuredInput{Causes: []CauseInput{noMechanism}}, "causes[0].mechanism is required"},
		"unknown cause kind":     {StructuredInput{Causes: []CauseInput{cause("cart", "bug")}}, "causes[0].kind must be one of"},
		"trigger without key":    {StructuredInput{Causes: []CauseInput{keyless}}, "causes[0].trigger.key is required"},
		"unknown trigger kind":   {StructuredInput{Causes: []CauseInput{badTrigger}}, "causes[0].trigger.kind must be one of"},
		"malformed fact id":      {StructuredInput{Causes: []CauseInput{badFact}}, "causes[0].evidence.facts takes fact ids"},
		"malformed trace id":     {StructuredInput{Causes: []CauseInput{badTrace}}, "causes[0].evidence.trace_ids takes 16 or 32"},
		"unknown estimate":       {StructuredInput{Causes: []CauseInput{badEstimate}}, "causes[0].estimate must be one of"},
		"relative onset":         {StructuredInput{Causes: []CauseInput{badOnset}}, "causes[0].onset must be an absolute"},
		"root cause disagrees": {StructuredInput{
			RootCause: &RootCauseInput{Component: "checkout", Kind: "deploy", Summary: "x"},
			Causes:    []CauseInput{cause("cart", "deploy")},
		}, "root_cause must describe causes[0]"},
		"root cause kind disagrees": {StructuredInput{
			RootCause: &RootCauseInput{Component: "cart", Kind: "resource", Summary: "x"},
			Causes:    []CauseInput{cause("cart", "deploy")},
		}, "root_cause must describe causes[0]"},
	}
	for name, c := range cases {
		_, err := c.in.Check()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
	}
}

func TestSeveralCausesAreKeptInOrderAndTheFirstIndependentOneIsTheRootCause(t *testing.T) {
	first := knockOn(cause("frontend", "dependency"), 1)
	second := cause("cart", "config_or_flag")
	second.Component.Namespace = "shop"
	second.Trigger = &TriggerInput{Kind: "feature_flag", Key: " cartFailure ", Value: "on"}
	second.Onset = "2026-11-20T10:05:00+01:00"
	second.Affected = []ComponentInput{{Kind: "service", Name: "checkout"}}
	second.Evidence = EvidenceInput{Facts: []string{"f2", "f2", "f9"}, TraceIDs: []string{"ABCDEF0123456789"}}
	second.Estimate = "high"
	got, err := StructuredInput{Causes: []CauseInput{first, second}}.Check()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Causes) != 2 || got.Causes[0].Relation() != RelationKnockOn || got.Causes[1].Relation() != RelationIndependent {
		t.Fatalf("causes %+v", got.Causes)
	}
	c := got.Causes[1]
	if c.Trigger.Key != "cartFailure" || c.Onset == nil || !c.Onset.Equal(time.Date(2026, 11, 20, 9, 5, 0, 0, time.UTC)) {
		t.Errorf("trigger or onset %+v", c)
	}
	if strings.Join(c.Evidence.Facts, ",") != "f2,f9" || c.Evidence.TraceIDs[0] != "abcdef0123456789" || !c.Stated {
		t.Errorf("evidence or stated %+v", c)
	}
	if got.RootCause == nil || got.RootCause.Component != "shop/cart" || got.RootCause.Kind != "config_or_flag" || got.RootCause.Summary != "cart failed" {
		t.Errorf("root cause %+v, want the first independent cause", got.RootCause)
	}
}

func TestARootCauseAloneIsStoredAsAOneCauseList(t *testing.T) {
	got, err := StructuredInput{
		RootCause:  &RootCauseInput{Component: "checkout", Kind: "deploy", Summary: "Version 42 broke the cart call."},
		Onset:      "2026-11-20T10:05:00Z",
		Confidence: "medium",
	}.Check()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Causes) != 1 {
		t.Fatalf("causes %+v", got.Causes)
	}
	c := got.Causes[0]
	if c.Component != (Component{Kind: "other", Name: "checkout"}) || c.Mechanism != "Version 42 broke the cart call." || c.Onset == nil || c.Estimate != "medium" || !c.Stated {
		t.Errorf("cause %+v", c)
	}
	if got.RootCause == nil || *got.RootCause != (RootCause{Component: "checkout", Kind: "deploy", Summary: "Version 42 broke the cart call."}) {
		t.Errorf("root cause %+v", got.RootCause)
	}
}

func TestARootCauseThatAgreesWithTheFirstCauseIsAccepted(t *testing.T) {
	c := cause("cart", "deploy")
	c.Component.Namespace = "shop"
	for _, component := range []string{"cart", "Shop/Cart"} {
		got, err := StructuredInput{RootCause: &RootCauseInput{Component: component, Kind: "deploy", Summary: "other words"}, Causes: []CauseInput{c}}.Check()
		if err != nil {
			t.Fatalf("%s: %v", component, err)
		}
		if got.RootCause.Summary != "cart failed" {
			t.Errorf("%s: root cause %+v, want it filled from the cause", component, got.RootCause)
		}
	}
}

func TestARowStoredBeforeCausesReadsAsOneStatedCause(t *testing.T) {
	onset := time.Date(2026, 11, 20, 9, 5, 0, 0, time.UTC)
	legacy := &RootCause{Component: "checkout", Kind: "deploy", Summary: "Version 42 broke it."}
	got := causesOf(nil, legacy, &onset, "high")
	if len(got) != 1 || got[0].Component.Name != "checkout" || got[0].Mechanism != "Version 42 broke it." || got[0].Onset != &onset || got[0].Estimate != "high" || !got[0].Stated || got[0].Score != nil {
		t.Fatalf("got %+v", got)
	}
	if causesOf(nil, nil, nil, "") != nil {
		t.Error("a row without a root cause has causes")
	}
	stored, err := encodeCauses([]Cause{{Component: Component{Kind: "service", Name: "cart"}, Kind: "deploy", Mechanism: "m"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := causesOf(stored, legacy, nil, ""); len(got) != 1 || got[0].Component.Name != "cart" || got[0].Stated {
		t.Errorf("stored causes %+v, want the stored list over the root cause", got)
	}
}

func TestTheConclusionListsStatedAndUnconfirmedCausesApart(t *testing.T) {
	onset := time.Date(2026, 11, 20, 9, 5, 0, 0, time.UTC)
	high, low := 0.85, 0.2
	causes := []Cause{
		{Component: Component{Kind: "flag", Name: "cartFailure"}, Kind: "config_or_flag", Mechanism: "The flag made cart return errors",
			Trigger: &Trigger{Kind: "feature_flag", Key: "cartFailure", Value: "on"}, Onset: &onset, Score: &high, Band: "high", Stated: true},
		{Component: Component{Kind: "service", Name: "checkout", Namespace: "shop"}, Kind: "dependency", Mechanism: "Checkout failed on cart calls.", KnockOnOf: new(0), Estimate: "medium", Stated: true},
		{Component: Component{Kind: "other", Name: "db_<primary>"}, Kind: "resource", Mechanism: "Maybe the disk filled.", Score: &low, Band: "low"},
	}
	want := "### Causes\n\n" +
		"1. **cartFailure** · flag · high confidence (0.85): The flag made cart return errors. Trigger: feature flag cartFailure = on. Started 2026-11-20 09:05 UTC.\n" +
		"2. **shop/checkout** · service · knock-on of cartFailure · estimated medium, not scored: Checkout failed on cart calls.\n\n" +
		"### Unconfirmed causes\n\n" +
		"- **db\\_\\<primary\\>** · low confidence (0.20): Maybe the disk filled."
	if got := CausesMarkdown(causes); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := WithCauses("Answer.", nil); got != "Answer." {
		t.Errorf("no causes changed the markdown: %q", got)
	}
}
