package revisions

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestBuildStateOf(t *testing.T) {
	cases := []struct {
		name  string
		facts buildFacts
		want  BuildState
	}{
		{"a run that never saved a build", buildFacts{run: StateFailed}, BuildNone},
		{"a finding run", buildFacts{run: StateSucceeded, result: ResultObservation}, BuildNone},
		{"a queued run that kept a continued build", buildFacts{run: StateQueued, held: true, panels: 3}, BuildBuilding},
		{"a running run", buildFacts{run: StateRunning, held: true}, BuildBuilding},
		{"a canceling run", buildFacts{run: StateCanceling, held: true, panels: 1}, BuildBuilding},
		{"a failed run with panels", buildFacts{run: StateFailed, held: true, panels: 2}, BuildStopped},
		{"a canceled run with panels", buildFacts{run: StateCanceled, held: true, panels: 1}, BuildStopped},
		{"a failed run with an empty build", buildFacts{run: StateFailed, held: true}, BuildStoppedEmpty},
		{"an empty build on a moved-on dashboard", buildFacts{run: StateFailed, held: true, stale: true}, BuildStoppedEmpty},
		{"a stopped build on a moved-on dashboard", buildFacts{run: StateFailed, held: true, panels: 2, stale: true}, BuildStale},
		{"a running build on a moved-on dashboard", buildFacts{run: StateRunning, held: true, panels: 2, stale: true}, BuildBuilding},
		{"a kept build", buildFacts{run: StateFailed, result: ResultPartialVersion, kept: true}, BuildKept},
		{"a continued run", buildFacts{run: StateCanceled, moved: true}, BuildMoved},
		{"a continued run that still holds a build", buildFacts{run: StateFailed, held: true, panels: 1, moved: true}, BuildStopped},
		{"a finished build", buildFacts{run: StateSucceeded, result: ResultVersion}, BuildCommitted},
		{"a proposed build", buildFacts{run: StateSucceeded, result: ResultProposal}, BuildProposed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildStateOf(c.facts)
			if got != c.want {
				t.Errorf("buildStateOf(%+v) = %s, want %s", c.facts, got, c.want)
			}
			if !BuildStates.Has(string(got)) {
				t.Errorf("%s is not a declared build state", got)
			}
		})
	}
}

func TestOnlyAStoppedBuildWithPanelsOffersKeepAndContinue(t *testing.T) {
	for _, held := range []bool{false, true} {
		for _, panels := range []int{0, 2} {
			for _, kept := range []bool{false, true} {
				facts := runFacts{state: StateFailed, source: SourcePrompt, onDashboard: true, requester: true, mayEdit: true, buildSaved: kept}
				if held {
					facts.buildPanels = panels
				}
				state := buildStateOf(buildFacts{run: StateFailed, held: held, panels: panels, kept: kept})
				offered := slices.Contains(allowedActions(facts), ActionKeepBuild)
				if offered != (state == BuildStopped) {
					t.Errorf("held=%v panels=%d kept=%v: keep_build offered=%v with build state %s", held, panels, kept, offered, state)
				}
			}
		}
	}
}

func TestNewBuildStateReportsStateAndExpiry(t *testing.T) {
	finished := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	held := dbsqlc.ListAgentRequestBuildStateRow{
		Saves:         pgtype.Int4{Int32: 2, Valid: true},
		Panels:        pgtype.Int4{Int32: 3, Valid: true},
		BaseVersion:   pgtype.Int4{Int32: 4, Valid: true},
		LatestVersion: 4,
		RunState:      string(StateFailed),
		FinishedAt:    pgtype.Timestamptz{Time: finished, Valid: true},
	}
	stopped := newBuildState(held)
	if stopped.derived() != BuildStopped || stopped.build.State != BuildStopped {
		t.Errorf("state = %s and build.state = %s, want stopped", stopped.derived(), stopped.build.State)
	}
	if stopped.build.ExpiresAt == nil || *stopped.build.ExpiresAt != "2026-03-08T10:00:00Z" {
		t.Errorf("expires_at = %v, want 7 days after the run finished", stopped.build.ExpiresAt)
	}

	held.LatestVersion = 5
	if got := newBuildState(held).derived(); got != BuildStale {
		t.Errorf("a build behind its dashboard = %s, want stale", got)
	}

	held.RunState, held.FinishedAt = string(StateRunning), pgtype.Timestamptz{}
	running := newBuildState(held)
	if running.derived() != BuildBuilding || running.build.ExpiresAt != nil {
		t.Errorf("a running build = %s with expires_at %v, want building and none", running.derived(), running.build.ExpiresAt)
	}

	gone := newBuildState(dbsqlc.ListAgentRequestBuildStateRow{RunState: string(StateFailed), ContinuedBy: pgtype.UUID{Valid: true}})
	if gone.build != nil || gone.derived() != BuildMoved {
		t.Errorf("a continued run = build %v and state %s, want no build and moved", gone.build, gone.derived())
	}
	if got := (buildState{}).derived(); got != BuildNone {
		t.Errorf("an unread build state = %s, want none", got)
	}
}

func TestBuildRefusalsCarryStateAllowedActionsAndReason(t *testing.T) {
	cases := []struct {
		err          error
		code, reason string
	}{
		{ErrBuildRunning, "build_running", "run_not_finished"},
		{ErrBuildEmpty, "build_empty", "no_panels"},
		{ErrBuildNotSavable, "build_not_savable", "no_stopped_build"},
	}
	for _, c := range cases {
		got, ok := buildRefusalFor(c.err, BuildKept, []Action{ActionRetry})
		if !ok {
			t.Fatalf("%v is not a build refusal", c.err)
		}
		if got.Code != c.code || got.Reason != c.reason || got.State != BuildKept || !slices.Equal(got.AllowedActions, []string{"retry"}) {
			t.Errorf("refusal for %v = %+v", c.err, got)
		}
		if got.Message == "" || got.Error != got.Message {
			t.Errorf("refusal for %v: error %q and message %q, want the same text", c.err, got.Error, got.Message)
		}
	}
	if got, _ := buildRefusalFor(ErrBuildEmpty, BuildStoppedEmpty, nil); got.AllowedActions == nil {
		t.Error("allowed_actions is null, want an empty list")
	}
	if _, ok := buildRefusalFor(errors.New("other"), BuildNone, nil); ok {
		t.Error("an unrelated error became a build refusal")
	}
}
