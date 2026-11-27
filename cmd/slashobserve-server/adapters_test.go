package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/threads"
)

type errorCase struct {
	name string
	in   error
	want error
}

func checkErrorMapping(t *testing.T, cases []errorCase, mapper func(error) error) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapper(tc.in); !errors.Is(got, tc.want) {
				t.Errorf("mapped to %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRevisionsBuildErrorMapsEachBuildFailureToItsRevisionsError(t *testing.T) {
	checkErrorMapping(t, []errorCase{
		{"no build", dashboards.ErrNoBuild, revisions.ErrNoBuild},
		{"build not found", dashboards.ErrBuildNotFound, revisions.ErrBuildNotFound},
		{"empty build", dashboards.ErrBuildEmpty, revisions.ErrBuildEmpty},
		{"build not savable", dashboards.ErrBuildNotSavable, revisions.ErrBuildNotSavable},
		{"dashboard not found", dashboards.ErrNotFound, revisions.ErrDashboardNotFound},
		{"wrapped", fmt.Errorf("save: %w", dashboards.ErrBuildEmpty), revisions.ErrBuildEmpty},
	}, revisionsBuildError)
}

func TestRevisionsBuildErrorKeepsTheLatestVersionOfAConflict(t *testing.T) {
	got := revisionsBuildError(fmt.Errorf("save: %w", &dashboards.ConflictError{Latest: 7}))
	var conflict *revisions.BuildConflictError
	if !errors.As(got, &conflict) || conflict.Latest != 7 {
		t.Fatalf("mapped to %v, want a BuildConflictError at version 7", got)
	}
}

func TestRevisionsBuildErrorPassesAnUnknownErrorThrough(t *testing.T) {
	other := errors.New("database down")
	if got := revisionsBuildError(other); !errors.Is(got, other) {
		t.Errorf("mapped to %v, want the same error", got)
	}
}

func TestSaveAnswerErrorHidesAnAnswerFromAnyoneButItsRequester(t *testing.T) {
	checkErrorMapping(t, []errorCase{
		{"not found", findings.ErrNotFound, revisions.ErrNoAnswer},
		{"requester only", findings.ErrRequesterOnly, revisions.ErrNotRequester},
		{"wrapped", fmt.Errorf("save: %w", findings.ErrRequesterOnly), revisions.ErrNotRequester},
	}, saveAnswerError)
	other := errors.New("database down")
	if got := saveAnswerError(other); !errors.Is(got, other) {
		t.Errorf("unknown error mapped to %v, want it passed through", got)
	}
	if got := saveAnswerError(nil); got != nil {
		t.Errorf("nil mapped to %v", got)
	}
}

func TestShareAnswerErrorMapsEachRefusal(t *testing.T) {
	checkErrorMapping(t, []errorCase{
		{"run not found", threads.ErrRunNotFound, revisions.ErrRequestNotFound},
		{"no answer", threads.ErrNoAnswer, revisions.ErrNoAnswerToShare},
		{"not shareable", threads.ErrNotShareable, revisions.ErrNotShareable},
		{"thread not found", threads.ErrNotFound, revisions.ErrShareThread},
		{"wrong thread", &threads.InvalidInputError{Code: threads.CodeWrongThread}, revisions.ErrShareThread},
		{"target not found", threads.ErrTargetNotFound, revisions.ErrRequestNotFound},
		{"wrapped", fmt.Errorf("share: %w", threads.ErrNotShareable), revisions.ErrNotShareable},
	}, shareAnswerError)
}

func TestShareAnswerErrorPassesOtherErrorsThrough(t *testing.T) {
	other := errors.New("database down")
	if got := shareAnswerError(other); !errors.Is(got, other) {
		t.Errorf("unknown error mapped to %v, want it passed through", got)
	}
	notWrongThread := &threads.InvalidInputError{Code: threads.CodeInvalidMention}
	if got := shareAnswerError(notWrongThread); errors.Is(got, revisions.ErrShareThread) {
		t.Errorf("an invalid input other than a wrong thread mapped to %v", got)
	}
}

func TestOpenObservationErrorMapsNotFoundOnly(t *testing.T) {
	checkErrorMapping(t, []errorCase{
		{"not found", observations.ErrNotFound, revisions.ErrObservationNotFound},
	}, openObservationError)
	other := errors.New("database down")
	if got := openObservationError(other); !errors.Is(got, other) {
		t.Errorf("unknown error mapped to %v, want it passed through", got)
	}
	if got := openObservationError(nil); got != nil {
		t.Errorf("nil mapped to %v", got)
	}
}

func TestAnswerOfOffersSavingOnlyToTheRequesterOfAnUnsavedAnswer(t *testing.T) {
	run := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	requester := auth.Principal{Kind: auth.KindUser, ID: "u1"}
	finding := findings.Finding{
		ID:              "f1",
		Title:           "Checkout is slow",
		Markdown:        "The cache is cold.",
		SuggestedChange: "warm the cache",
		RequestedBy:     findings.Author{Kind: string(auth.KindUser), ID: "u1"},
	}

	cases := []struct {
		name    string
		by      auth.Principal
		finding findings.Finding
		canSave bool
	}{
		{"requester", requester, finding, true},
		{"another person", auth.Principal{Kind: auth.KindUser, ID: "u2"}, finding, false},
		{"a key with the requester's id", auth.Principal{Kind: auth.KindServiceAccount, ID: "u1"}, finding, false},
		{"requester of a saved answer", requester, func() findings.Finding { f := finding; f.ObservationID = "o1"; return f }(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := answerOf(run, tc.finding, tc.by, "https://x/o/f1")
			if err != nil {
				t.Fatal(err)
			}
			if got.CanSave != tc.canSave {
				t.Errorf("CanSave = %v, want %v", got.CanSave, tc.canSave)
			}
			if got.RunID != run.String() || got.FindingID != "f1" || got.Title != tc.finding.Title || got.Markdown != tc.finding.Markdown ||
				got.SuggestedChange != "warm the cache" || got.ObservationID != tc.finding.ObservationID || got.EvidencePanels != 0 {
				t.Errorf("answer = %+v", got)
			}
		})
	}
}

func TestAnswerOfCarriesTheFindingJSONWithItsURL(t *testing.T) {
	run := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	got, err := answerOf(run, findings.Finding{ID: "f1", Title: "t"}, auth.Principal{}, "https://x/c/run")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(got.Finding, &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "f1" || body.URL != "https://x/c/run" {
		t.Errorf("finding JSON = %s", got.Finding)
	}
}
