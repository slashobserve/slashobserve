package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const runID = "11111111-2222-3333-4444-555555555555"

type fakeRuns struct {
	raw     json.RawMessage
	err     error
	gotID   pgtype.UUID
	gotBy   auth.Principal
	gotMax  int
	invoked bool
}

func (f *fakeRuns) Run(_ context.Context, id pgtype.UUID, by auth.Principal, maxSteps int) (json.RawMessage, error) {
	f.invoked, f.gotID, f.gotBy, f.gotMax = true, id, by, maxSteps
	return f.raw, f.err
}

func callGetRun(t *testing.T, runs RunReader, role auth.Role, input string) (Result, error) {
	t.Helper()
	r := New(Deps{Runs: runs})
	by := auth.Principal{Kind: auth.KindUser, Name: "n", Role: role}
	return r.Call(auth.WithPrincipal(t.Context(), by), nameGetRun, []byte(input))
}

func TestGetRunRejectsABadInput(t *testing.T) {
	for name, input := range map[string]string{
		"missing id":  `{}`,
		"not a uuid":  `{"id":"nope"}`,
		"empty id":    `{"id":""}`,
		"unknown key": `{"id":"` + runID + `","extra":1}`,
		"wrong type":  `{"id":7}`,
	} {
		t.Run(name, func(t *testing.T) {
			runs := &fakeRuns{raw: json.RawMessage(`{}`)}
			res, err := callGetRun(t, runs, auth.RoleViewer, input)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != codeInvalidRequest {
				t.Fatalf("code %q", code)
			}
			if runs.invoked {
				t.Fatal("the reader was called with a bad input")
			}
		})
	}
}

func TestGetRunReturnsTheRawRunAndAsksForTwentySteps(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","state":"running","allowed_actions":["cancel"]}`)}
	res, err := callGetRun(t, runs, auth.RoleViewer, `{"id":"`+runID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("error result %+v", res)
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(runs.raw) {
		t.Fatalf("content %s, want %s", raw, runs.raw)
	}
	if runs.gotID.String() != runID || runs.gotMax != 20 || runs.gotBy.Role != auth.RoleViewer {
		t.Fatalf("reader got %v %v %d", runs.gotID, runs.gotBy, runs.gotMax)
	}
}

func TestGetRunMapsErrors(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{revisions.ErrRequestNotFound, codeNotFound},
		{revisions.ErrNotFound, codeNotFound},
		{revisions.ErrDashboardNotFound, codeNotFound},
		{folders.ErrNotFound, codeNotFound},
		{fmt.Errorf("wrapped: %w", revisions.ErrRequestNotFound), codeNotFound},
		{folders.ErrForbidden, codeForbiddenRole},
		{folders.ErrNoAccess, codeForbiddenRole},
		{revisions.ErrForbidden, codeForbiddenRole},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			res, err := callGetRun(t, &fakeRuns{err: tc.err}, auth.RoleViewer, `{"id":"`+runID+`"}`)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != tc.want {
				t.Fatalf("code %q, want %q", code, tc.want)
			}
		})
	}
}

func TestGetRunReportsAnUnknownFailureAsAToolFailure(t *testing.T) {
	res, err := callGetRun(t, &fakeRuns{err: errors.New("database is down")}, auth.RoleViewer, `{"id":"`+runID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != FailureToolFailed {
		t.Fatalf("code %q", code)
	}
}

func TestGetRunWithoutAReaderIsAToolFailure(t *testing.T) {
	res, err := callGetRun(t, nil, auth.RoleViewer, `{"id":"`+runID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != FailureToolFailed {
		t.Fatalf("code %q", code)
	}
}
