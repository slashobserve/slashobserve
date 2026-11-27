package tools

import (
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/findings"
)

var factLedger = &grounding.Ledger{Facts: []grounding.Fact{
	{ID: "f1", Text: "0.5234123", Value: 0.5234123, Kind: grounding.FactNumber, Query: "histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))"},
	{ID: "f2", Text: "1284", Value: 1284, Kind: grounding.FactCount},
}}

func TestTheFindingRendersEveryCitedFactBeforeItIsChecked(t *testing.T) {
	in := findingInput{
		Title:    "Checkout at {{f1:ms:3}}",
		Markdown: "p99 peaked at {{f1:ms}} with {{f2}} errors, then {{f9}}.",
		StructuredInput: findings.StructuredInput{
			Impact:    "{{f2}} failed checkouts.",
			RootCause: &findings.RootCauseInput{Component: "cart", Kind: "dependency", Summary: "Redis at {{f1:s:2}}"},
			RuledOut:  []string{"not {{f2}} retries"},
		},
	}
	got := renderedFinding(WithFacts(t.Context(), factLedger), in)
	if got.Title != "Checkout at 523 ms" || got.Markdown != "p99 peaked at 523.4123 ms with 1284 errors, then "+grounding.UnreadMarker+"." ||
		got.Impact != "1284 failed checkouts." || got.RootCause.Summary != "Redis at 0.52 s" || !slices.Equal(got.RuledOut, []string{"not 1284 retries"}) {
		t.Errorf("rendered = %+v %+v", got, got.RootCause)
	}
	if in.RootCause.Summary != "Redis at {{f1:s:2}}" {
		t.Error("rendering changed the caller's input")
	}
	if same := renderedFinding(t.Context(), in); same.Markdown != in.Markdown {
		t.Errorf("without a ledger the markdown became %q", same.Markdown)
	}
}

func TestASavedFindingHoldsTheRenderedFiguresAndNoPlaceholder(t *testing.T) {
	r := newFindingRig(t)
	run := investigateRun(findingWindowTo.Add(-2*time.Hour), findingWindowTo)
	ctx := WithFacts(WithRun(auth.WithPrincipal(t.Context(), editorCaller), run), factLedger)
	res, err := r.registry.Call(ctx, nameSaveFinding, []byte(`{"markdown":"Checkout p99 peaked at {{f1:ms:4}}.","answers":[{"quantity":"peak p99","fact":"f1"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok {
		t.Fatalf("save: %#v", res.Content)
	}
	id, err := db.ParseUUID(saved.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := r.findings.Get(reading(t.Context()), editorCaller, id)
	if err != nil {
		t.Fatal(err)
	}
	if f.Markdown != "Checkout p99 peaked at 523.4 ms." {
		t.Errorf("stored markdown = %q", f.Markdown)
	}
}
