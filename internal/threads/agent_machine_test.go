package threads

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var (
	builtin   = &revisions.Assignee{Kind: revisions.AssignBuiltin}
	noAgent   = threadAgent{}
	following = threadAgent{Agent: builtin}
	turnedOff = threadAgent{Agent: builtin, Off: true}
)

func TestTheThreadAgentMachineIsConsistent(t *testing.T) {
	if err := AgentMachine.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestAThreadAgentStateComesFromItsAgentAndTheOffFlag(t *testing.T) {
	cases := map[AgentState]threadAgent{AgentNone: noAgent, AgentFollowing: following, AgentOff: turnedOff}
	for want, agent := range cases {
		if got := agent.state(); got != want {
			t.Errorf("%+v: %s, want %s", agent, got, want)
		}
	}
}

func TestOnlyAFollowingAgentCanBeTurnedOff(t *testing.T) {
	if !following.allows(AgentTurnOff) {
		t.Error("a following agent cannot be turned off")
	}
	for _, agent := range []threadAgent{noAgent, turnedOff} {
		if agent.allows(AgentTurnOff) {
			t.Errorf("%s allows turn_agent_off", agent.state())
		}
		if !agent.allows(AgentSet) {
			t.Errorf("%s refuses set_agent", agent.state())
		}
	}
}

func TestAThreadReadCarriesTheAgentStateAndOnlyTheThreadsOwnMoves(t *testing.T) {
	thread := Thread{AllowedActions: []string{"reply"}, Agent: builtin, AgentOff: true, AgentState: AgentOff}
	body := newThreadJSON(thread)
	if !slices.Equal(body.AllowedActions, []string{"reply"}) {
		t.Errorf("allowed_actions %v", body.AllowedActions)
	}
	if body.Agent == nil || body.Agent.State != "off" || !body.AgentOff {
		t.Errorf("agent %+v, agent_off %v", body.Agent, body.AgentOff)
	}
	if none := newThreadJSON(Thread{AgentState: AgentNone}); none.Agent != nil {
		t.Errorf("a thread with no agent returns %+v", none.Agent)
	}
}

func TestAThreadRowBecomesItsAgentState(t *testing.T) {
	row := dbsqlc.Thread{AgentKind: pgtype.Text{String: "builtin", Valid: true}}
	if got := threadOf(row, nil, rights{Comment: true}); got.AgentState != AgentFollowing || slices.Contains(got.AllowedActions, "set_agent") {
		t.Errorf("state %s, allowed %v", got.AgentState, got.AllowedActions)
	}
	row.AgentOff = true
	if got := threadOf(row, nil, rights{}).AgentState; got != AgentOff {
		t.Errorf("state %s, want off", got)
	}
	if got := threadOf(dbsqlc.Thread{}, nil, rights{}).AgentState; got != AgentNone {
		t.Errorf("state %s, want none", got)
	}
}
