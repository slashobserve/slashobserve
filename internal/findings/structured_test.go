package findings

import (
	"strings"
	"testing"
	"time"
)

func TestAnEmptyInputIsAnEmptyStructuredFinding(t *testing.T) {
	got, err := StructuredInput{}.Check()
	if err != nil || !got.Empty() {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestAFullInputIsTrimmedAndKept(t *testing.T) {
	got, err := StructuredInput{
		RootCause:  &RootCauseInput{Component: " checkout ", Kind: "deploy", Summary: " Version 42 broke the cart call. "},
		Onset:      "2026-11-20T10:05:00+01:00",
		Impact:     " 12% of orders failed. ",
		Confidence: "high",
		RuledOut:   []string{" the database "},
		NextSteps:  []string{"Roll back checkout."},
	}.Check()
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 20, 9, 5, 0, 0, time.UTC)
	if got.RootCause.Component != "checkout" || got.RootCause.Summary != "Version 42 broke the cart call." || got.Onset == nil || !got.Onset.Equal(want) || got.Onset.Location() != time.UTC {
		t.Errorf("root cause or onset: %#v", got)
	}
	if got.Impact != "12% of orders failed." || got.RuledOut[0] != "the database" || got.Empty() {
		t.Errorf("text fields: %#v", got)
	}
}

func TestInvalidFieldsAreRefusedWithAMessageThatNamesTheField(t *testing.T) {
	cause := func(c, k, s string) *RootCauseInput { return &RootCauseInput{Component: c, Kind: k, Summary: s} }
	cases := map[string]struct {
		in   StructuredInput
		want string
	}{
		"missing kind":        {StructuredInput{RootCause: cause("checkout", "", "x")}, "component, kind and summary together"},
		"unknown kind":        {StructuredInput{RootCause: cause("checkout", "bug", "x")}, "root_cause.kind must be one of"},
		"long component":      {StructuredInput{RootCause: cause(strings.Repeat("a", 121), "deploy", "x")}, "root_cause.component takes at most 120"},
		"multiline summary":   {StructuredInput{RootCause: cause("checkout", "deploy", "a\nb")}, "root_cause.summary is one plain line"},
		"relative onset":      {StructuredInput{Onset: "now-1h"}, "onset must be an absolute RFC 3339 time"},
		"long impact":         {StructuredInput{Impact: strings.Repeat("a", 301)}, "impact takes at most 300"},
		"unknown confidence":  {StructuredInput{Confidence: "certain"}, "confidence must be one of high, medium, low"},
		"six ruled out":       {StructuredInput{RuledOut: make([]string, 6)}, "ruled_out takes at most 5"},
		"empty ruled out":     {StructuredInput{RuledOut: []string{" "}}, "ruled_out takes no empty lines"},
		"four next steps":     {StructuredInput{NextSteps: []string{"a", "b", "c", "d"}}, "next_steps takes at most 3"},
		"long next step":      {StructuredInput{NextSteps: []string{strings.Repeat("a", 201)}}, "next_steps takes at most 200"},
		"no incident + cause": {StructuredInput{NoIncident: true, RootCause: cause("checkout", "deploy", "x")}, "no_incident is true"},
	}
	for name, c := range cases {
		_, err := c.in.Check()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
	}
}

func TestNoIncidentAloneIsAFinding(t *testing.T) {
	got, err := StructuredInput{NoIncident: true}.Check()
	if err != nil || got.Empty() || !got.NoIncident {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestJSONOfAnEmptyFindingHasNoNulls(t *testing.T) {
	got := StructuredJSONOf(Structured{})
	if got.RootCause != nil || got.Onset != nil || got.Impact != nil || got.Confidence != nil || got.RuledOut == nil || got.NextSteps == nil || got.NoIncident {
		t.Fatalf("got %#v", got)
	}
}
