package brief

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestTheStructuredBriefHoldsTheRequestInUTCAndLeavesUnsetFieldsNull(t *testing.T) {
	from := time.Date(2026, 3, 1, 10, 0, 0, 0, time.FixedZone("x", 3600))
	to := from.Add(time.Hour)
	task := Task{
		Mode: tools.ModeInvestigate, Source: revisions.SourceInvestigate, Prompt: "why", PanelID: "a", From: &from, To: &to,
		Attempt: 2, Investigation: true, TimeZone: "Europe/Paris", Withheld: true, Pointer: Pointer{BlockID: "b1"},
	}
	statuses := []datasource.Status{{UID: "prom", Type: "prometheus", Healthy: true}, {UID: "logs", Type: "loki"}}
	got := Structured(statuses, func(uid string) bool { return uid == "prom" }, task, View{}, json.RawMessage(`{"id":"a"}`))
	raw, err := json.Marshal(got.Request)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"mode":"investigate","source":"investigate","prompt":"why","panel_id":"a","from":"2026-03-01T09:00:00Z","to":"2026-03-01T10:00:00Z",` +
		`"time_zone":"Europe/Paris","zone":{"label":"CET","offset_seconds":3600},"as_of":null,"continue_note":null,"thread_context":null,"continues":false,"attempt":2,"investigation":true,` +
		`"pointer":{"anchor":null,"block_id":"b1","observation_id":null,"thread_ask":false}}`
	if string(raw) != want {
		t.Fatalf("request = %s\nwant %s", raw, want)
	}
	if !got.Withheld || got.Current != nil || got.Build != nil || got.Draft != nil {
		t.Fatalf("brief = %+v", got)
	}
	if len(got.Datasources) != 2 || !got.Datasources[0].Everyone || got.Datasources[1].Everyone || got.Datasources[1].Type != "loki" {
		t.Fatalf("datasources = %+v", got.Datasources)
	}
	if got.Guide.Text != prompts.GuideFor(guideTypes(statuses)) || got.Guide.Text == prompts.GuideFor(nil) {
		t.Fatalf("guide = %.200s", got.Guide.Text)
	}
	if string(got.Guide.Schema) != string(prompts.Schema()) {
		t.Fatal("the guide schema is not the agent's schema")
	}
}

func TestTheStructuredBriefCarriesADraftOnlyForASuggestion(t *testing.T) {
	draft := json.RawMessage(`{"id":"a","type":"stat"}`)
	if got := Structured(nil, nil, Task{Mode: tools.ModeSuggest}, View{}, draft); string(got.Draft) != string(draft) {
		t.Fatalf("suggest draft = %s", got.Draft)
	}
	if got := Structured(nil, nil, Task{Mode: tools.ModeRevise}, View{}, draft); got.Draft != nil {
		t.Fatalf("revise draft = %s", got.Draft)
	}
}
