package revisions

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestStatusIsComputedFromEveryState(t *testing.T) {
	want := map[State]Status{
		StateQueued:    StatusQueued,
		StateRunning:   StatusRunning,
		StateCanceling: StatusRunning,
		StateSucceeded: StatusSucceeded,
		StateFailed:    StatusFailed,
		StateCanceled:  StatusFailed,
	}
	for _, state := range RunMachine.States {
		if got := state.Status(); got == "" || got != want[state] {
			t.Errorf("status of %s = %q, want %q", state, got, want[state])
		}
	}
}

func TestEveryRunReadTakesItsStatusFromTheState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &Service{logger: slog.New(slog.DiscardHandler)}
	desk := NewRunDesk(service, ObserveOptions{})
	caller := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111"}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	for _, state := range RunMachine.States {
		row := dbsqlc.AgentRequest{State: string(state), Status: "stale", Steps: []byte("[]")}
		listed := dbsqlc.ListAgentRequestsByRow{State: string(state), Status: "stale"}
		reads := map[string]any{
			"run":        desk.view(t.Context(), row, caller, buildState{}, nil, nil),
			"summary":    desk.summary(listed, caller, buildState{}, nil),
			"revision":   newRevisionJSON(revisionFromRow(row), buildState{}, nil, caller),
			"suggestion": service.newSuggestionJSON(c, row),
		}
		for name, read := range reads {
			raw, err := json.Marshal(read)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Status Status `json:"status"`
				State  State  `json:"state"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got.State != state || got.Status != state.Status() {
				t.Errorf("%s of a %s run: state %q status %q, want %q and %q", name, state, got.State, got.Status, state, state.Status())
			}
		}
		if ev, ok := revisionEvent(dbsqlc.AgentRequest{State: string(state), Status: "stale", Intent: string(IntentApply), DashboardID: pgtype.UUID{Bytes: [16]byte{7}, Valid: true}}); !ok || ev.Status != string(state.Status()) {
			t.Errorf("revision event of a %s run: status %q (sent %t), want %q", state, ev.Status, ok, state.Status())
		}
	}
}
