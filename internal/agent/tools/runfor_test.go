package tools

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	runForRequestID   = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	runForDashboardID = "bbbbbbbb-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	runForObservationID = "cccccccc-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	runForSubjectID     = "dddddddd-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

func TestRunForMapsEachSourceToItsMode(t *testing.T) {
	from := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	cases := map[string]struct {
		req  revisions.Request
		want Run
	}{
		"a new dashboard request builds a new dashboard": {
			req:  revisions.Request{ID: runForRequestID, Intent: revisions.IntentAsk, Source: revisions.SourceObserve, Prompt: "shop", Attempt: 2, ClaimEpoch: 5},
			want: Run{Mode: ModeCreate, Build: true, RequestID: runForRequestID, ClaimEpoch: 5, Prompt: "shop"},
		},
		"an investigate ask that lost its observation stays read-only": {
			req:  revisions.Request{ID: runForRequestID, Intent: revisions.IntentAsk, Source: revisions.SourceInvestigate, Prompt: "why", Attempt: 1, ClaimEpoch: 4},
			want: Run{Mode: ModeInvestigate, RequestID: runForRequestID, ClaimEpoch: 4, Prompt: "why"},
		},
		"an investigation into an observation keeps its window and panel": {
			req:  revisions.Request{ID: runForRequestID, Intent: revisions.IntentAsk, Source: revisions.SourceInvestigate, Investigation: true, ObservationID: runForObservationID, Prompt: "why", Attempt: 1, ClaimEpoch: 4, PanelID: "p_up", From: &from, To: &to},
			want: Run{Mode: ModeInvestigate, RequestID: runForRequestID, ClaimEpoch: 4, Prompt: "why", PanelID: "p_up", From: from, To: to, Investigation: true, Observations: []string{runForObservationID}},
		},
		"an ask is not about the observation its answer was saved into": {
			req:  revisions.Request{ID: runForRequestID, Intent: revisions.IntentAsk, Source: revisions.SourceInvestigate, ObservationID: runForObservationID, SubjectObservationID: runForSubjectID, Prompt: "why", Attempt: 2, ClaimEpoch: 5},
			want: Run{Mode: ModeInvestigate, RequestID: runForRequestID, ClaimEpoch: 5, Prompt: "why", Observations: []string{runForSubjectID}},
		},
		"a private ask about an observation with half a window has none": {
			req:  revisions.Request{ID: runForRequestID, Intent: revisions.IntentAsk, Source: revisions.SourceInvestigate, SubjectObservationID: runForObservationID, Prompt: "why", Attempt: 1, ClaimEpoch: 4, From: &from},
			want: Run{Mode: ModeInvestigate, RequestID: runForRequestID, ClaimEpoch: 4, Prompt: "why", Observations: []string{runForObservationID}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := RunFor(tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("run = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRunForMapsASuggestRunAndPinsNoVersion(t *testing.T) {
	got, err := RunFor(revisions.Request{ID: runForRequestID, Intent: revisions.IntentSuggest, DashboardID: runForDashboardID, PanelID: "p_up", BaseVersion: 4, Prompt: "p", Attempt: 2})
	if err != nil {
		t.Fatal(err)
	}
	want := Run{Mode: ModeSuggest, RequestID: runForRequestID, DashboardID: runForDashboardID, PanelID: "p_up"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("run = %+v, want %+v", got, want)
	}
}

func TestRunForNamesTheDashboardOfARevisionAndPinsNoVersion(t *testing.T) {
	for name, intent := range map[string]revisions.Intent{"revise": revisions.IntentApply, "investigate": revisions.IntentAsk} {
		t.Run(name, func(t *testing.T) {
			got, err := RunFor(revisions.Request{ID: runForRequestID, Intent: intent, DashboardID: runForDashboardID, Prompt: "p", Attempt: 3, ClaimEpoch: 6})
			if err != nil {
				t.Fatal(err)
			}
			wantMode, wantBuild := ModeRevise, true
			if intent == revisions.IntentAsk {
				wantMode, wantBuild = ModeInvestigate, false
			}
			if got.Mode != wantMode || got.Build != wantBuild || got.DashboardID != runForDashboardID || got.ParentVersion != 0 || got.ClaimEpoch != 6 || got.RequestID != runForRequestID {
				t.Fatalf("run = %+v", got)
			}
		})
	}
}

func TestRunForRefusesADashboardIDThatIsNotAUUID(t *testing.T) {
	_, err := RunFor(revisions.Request{ID: runForRequestID, Intent: revisions.IntentApply, DashboardID: "nope", Attempt: 1})
	if !errors.Is(err, ErrInvariant) {
		t.Fatalf("err = %v, want ErrInvariant", err)
	}
}

func TestRunForRefusesARunWithNoIntent(t *testing.T) {
	if _, err := RunFor(revisions.Request{ID: runForRequestID, Attempt: 1}); !errors.Is(err, ErrInvariant) {
		t.Fatalf("err = %v, want an invariant error", err)
	}
}
