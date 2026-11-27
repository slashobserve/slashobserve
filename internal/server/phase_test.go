package server

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/logtest"
)

func servingPhases() *Phases {
	phases := StartPhases(slog.New(slog.DiscardHandler))
	for _, next := range []Phase{PhaseMigrating, PhaseProvisioning, PhaseReady} {
		phases.Enter(next)
	}
	return phases
}

func TestPhaseMachineIsWellFormed(t *testing.T) {
	if err := PhaseMachine.Check(); err != nil {
		t.Fatal(err)
	}
	if got := PhaseNames(); !slices.Equal(got, []string{"booting", "migrating", "provisioning", "ready", "stopping", "stopped"}) {
		t.Errorf("PhaseNames() = %v", got)
	}
}

func TestEachPhaseMoveWritesOneLogLineThatNamesThePhase(t *testing.T) {
	logger, logs := logtest.New()
	phases := StartPhases(logger, slog.String("version", "1.2.3"))
	if got := phases.Current(); got != PhaseBooting {
		t.Fatalf("a started server is %s, want booting", got)
	}
	phases.Enter(PhaseMigrating)
	phases.Enter(PhaseProvisioning)
	phases.Enter(PhaseReady, slog.String("addr", "127.0.0.1:8080"))
	phases.Enter(PhaseStopping)
	phases.Enter(PhaseStopped)
	if got := phases.Current(); got != PhaseStopped {
		t.Fatalf("after every move the server is %s, want stopped", got)
	}

	want := []struct{ msg, phase string }{
		{"starting slashobserve-server", "booting"},
		{"migrating database", "migrating"},
		{"provisioning", "provisioning"},
		{"listening", "ready"},
		{"shutting down", "stopping"},
		{"server stopped", "stopped"},
	}
	records := logs.Records(t)
	if len(records) != len(want) {
		t.Fatalf("%d log lines, want %d: %s", len(records), len(want), logs.String())
	}
	for i, w := range want {
		if records[i]["msg"] != w.msg || records[i]["phase"] != w.phase {
			t.Errorf("line %d = %v, want msg %q with phase %q", i, records[i], w.msg, w.phase)
		}
	}
	if records[0]["version"] != "1.2.3" || records[3]["addr"] != "127.0.0.1:8080" {
		t.Errorf("a move dropped its own attributes: %s", logs.String())
	}
}

func TestAMoveTheMachineLacksIsRefusedAndKeepsThePhase(t *testing.T) {
	cases := []struct {
		name string
		from func() *Phases
		to   Phase
		kept Phase
	}{
		{"ready straight from booting", func() *Phases { return StartPhases(slog.New(slog.DiscardHandler)) }, PhaseReady, PhaseBooting},
		{"stopping before it served", func() *Phases { return StartPhases(slog.New(slog.DiscardHandler)) }, PhaseStopping, PhaseBooting},
		{"booting again", servingPhases, PhaseBooting, PhaseReady},
		{"leaving stopped", func() *Phases {
			phases := servingPhases()
			phases.Enter(PhaseStopped)
			return phases
		}, PhaseReady, PhaseStopped},
	}
	for _, c := range cases {
		phases := c.from()
		logger, logs := logtest.New()
		phases.logger = logger
		phases.Enter(c.to)
		if got := phases.Current(); got != c.kept {
			t.Errorf("%s: phase = %s, want %s kept", c.name, got, c.kept)
		}
		record := logs.Find(t, "server phase move refused")
		if record["phase"] != string(c.kept) || record["refused_phase"] != string(c.to) {
			t.Errorf("%s: record = %v", c.name, record)
		}
	}
}

func TestAFailedBootExitsFromAnyPhase(t *testing.T) {
	for _, reached := range [][]Phase{{}, {PhaseMigrating}, {PhaseMigrating, PhaseProvisioning}, {PhaseMigrating, PhaseProvisioning, PhaseReady}} {
		phases := StartPhases(slog.New(slog.DiscardHandler))
		for _, next := range reached {
			phases.Enter(next)
		}
		phases.Enter(PhaseStopped)
		if got := phases.Current(); got != PhaseStopped {
			t.Errorf("after %v the exit left the server %s", reached, got)
		}
	}
}
