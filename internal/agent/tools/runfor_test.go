package tools

import (
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	runForRequestID   = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	runForDashboardID = "bbbbbbbb-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

func TestRunForMapsEachSourceToItsMode(t *testing.T) {
	from := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	cases := map[string]struct {
		req  revisions.Request
		want Run
	}{
		"a new dashboard request builds a new dashboard": {
			req:  revisions.Request{ID: runForRequestID, Source: revisions.SourceObserve, Prompt: "shop", Attempt: 2},
			want: Run{Mode: ModeCreate, Build: true, RequestID: runForRequestID, Attempt: 2, Prompt: "shop"},
		},
		"an investigation without a dashboard keeps its window and panel": {
			req:  revisions.Request{ID: runForRequestID, Source: revisions.SourceInvestigate, Prompt: "why", Attempt: 1, PanelID: "p_up", From: &from, To: &to},
			want: Run{Mode: ModeInvestigate, RequestID: runForRequestID, Attempt: 1, Prompt: "why", PanelID: "p_up", From: from, To: to},
		},
		"an investigation with half a window has none": {
			req:  revisions.Request{ID: runForRequestID, Source: revisions.SourceInvestigate, Prompt: "why", Attempt: 1, From: &from},
			want: Run{Mode: ModeInvestigate, RequestID: runForRequestID, Attempt: 1, Prompt: "why"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := RunFor(tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("run = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRunForMapsASuggestRunAndPinsNoVersion(t *testing.T) {
	got, err := RunFor(revisions.Request{ID: runForRequestID, Source: revisions.SourceSuggest, DashboardID: runForDashboardID, PanelID: "p_up", BaseVersion: 4, Prompt: "p", Attempt: 2})
	if err != nil {
		t.Fatal(err)
	}
	want := Run{Mode: ModeSuggest, RequestID: runForRequestID, DashboardID: runForDashboardID, PanelID: "p_up"}
	if got != want {
		t.Fatalf("run = %+v, want %+v", got, want)
	}
}

func TestRunForNamesTheDashboardOfARevisionAndPinsNoVersion(t *testing.T) {
	for name, source := range map[string]revisions.Source{"revise": revisions.SourceObserve, "investigate": revisions.SourceInvestigate} {
		t.Run(name, func(t *testing.T) {
			got, err := RunFor(revisions.Request{ID: runForRequestID, Source: source, DashboardID: runForDashboardID, Prompt: "p", Attempt: 3})
			if err != nil {
				t.Fatal(err)
			}
			wantMode, wantBuild := ModeRevise, true
			if source == revisions.SourceInvestigate {
				wantMode, wantBuild = ModeInvestigate, false
			}
			if got.Mode != wantMode || got.Build != wantBuild || got.DashboardID != runForDashboardID || got.ParentVersion != 0 || got.Attempt != 3 || got.RequestID != runForRequestID {
				t.Fatalf("run = %+v", got)
			}
		})
	}
}

func TestRunForRefusesADashboardIDThatIsNotAUUID(t *testing.T) {
	_, err := RunFor(revisions.Request{ID: runForRequestID, Source: revisions.SourceObserve, DashboardID: "nope", Attempt: 1})
	if !errors.Is(err, ErrInvariant) {
		t.Fatalf("err = %v, want ErrInvariant", err)
	}
}
