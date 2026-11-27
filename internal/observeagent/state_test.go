package observeagent

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSupervisorMachineIsConsistent(t *testing.T) {
	if err := SupervisorMachine.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorIsInBackoffWhileTheChildCannotStart(t *testing.T) {
	h := newHarness(t, "exit")
	h.s.cfg.Path = filepath.Join(t.TempDir(), "missing", "observe-agent")
	var waiting []State
	h.s.sleep = func(context.Context, time.Duration) {
		waiting = append(waiting, h.s.State())
		if len(waiting) == 2 {
			h.cancel()
		}
	}
	if got := h.s.State(); got != "" {
		t.Fatalf("state before Run = %q, want none", got)
	}
	h.start()
	h.wait(t)

	if want := []State{StateBackoff, StateBackoff}; !slices.Equal(waiting, want) {
		t.Fatalf("states while waiting to restart = %v, want %v", waiting, want)
	}
	if got := h.s.State(); got != StateStopping {
		t.Fatalf("state after Run returned = %s, want stopping", got)
	}
	backoff := h.rec.next(t, "observe-agent waiting to restart")
	if backoff.attrs[stateLogKey] != string(StateBackoff) {
		t.Fatalf("record = %+v, want %s=%s", backoff, stateLogKey, StateBackoff)
	}
}

func TestSupervisorIsRunningWhileTheChildRunsAndStoppingAfter(t *testing.T) {
	h := newHarness(t, "serve")
	h.start()
	h.rec.next(t, "child started")
	if got := h.s.State(); got != StateRunning {
		t.Fatalf("state with a child running = %s, want running", got)
	}

	h.stop(t)

	if got := h.s.State(); got != StateStopping {
		t.Fatalf("state after stop = %s, want stopping", got)
	}
}

func TestSupervisorIgnoresAMoveItsMachineDoesNotDeclare(t *testing.T) {
	h := newHarness(t, "serve")
	h.s.move(ActionStart, StateStarting)
	h.s.move(ActionStop, StateStopping)
	h.s.move(ActionRun, StateRunning)
	if got := h.s.State(); got != StateStopping {
		t.Fatalf("state = %s, want stopping: nothing leaves it", got)
	}
}
