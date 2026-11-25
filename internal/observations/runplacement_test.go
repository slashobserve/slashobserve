package observations

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestAWriteOfContentIsPlacedForItsRunBeforeItRuns(t *testing.T) {
	var ran bool
	write := placedForRun(t.Context(), func(*dbsqlc.Queries, dbsqlc.Observation, folders.Abilities, actor.Actor) error {
		ran = true
		return nil
	})
	if err := write(nil, dbsqlc.Observation{}, folders.Abilities{}, actor.Actor{}); err != nil || !ran {
		t.Fatalf("a write outside a run: ran %v, err %v", ran, err)
	}
	ran = false
	if err := write(nil, dbsqlc.Observation{}, folders.Abilities{}, actor.Actor{RunID: "not-a-run"}); err == nil || ran {
		t.Fatalf("a write for a malformed run: ran %v, err %v", ran, err)
	}
}
