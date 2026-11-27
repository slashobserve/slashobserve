package threads

import (
	"slices"
	"testing"

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

func TestAThreadOffersItsAgentMovesToSomeoneWhoMayComment(t *testing.T) {
	commenter, viewer := rights{Comment: true}, rights{}
	cases := []struct {
		state AgentState
		by    rights
		want  []string
	}{
		{AgentNone, commenter, []string{"set_agent"}},
		{AgentFollowing, commenter, []string{"set_agent", "turn_agent_off"}},
		{AgentOff, commenter, []string{"set_agent"}},
		{AgentFollowing, viewer, []string{}},
	}
	for _, c := range cases {
		if got := allowedAgentActions(c.state, c.by); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.state, got, c.want)
		}
	}
}

func TestAThreadReadCarriesTheAgentStateAndMovesBesideItsOwn(t *testing.T) {
	thread := Thread{AllowedActions: []string{"reply"}, Agent: builtin, AgentOff: true, AgentState: AgentOff, AgentActions: []string{"set_agent"}}
	body := newThreadJSON(thread)
	if !slices.Equal(body.AllowedActions, []string{"reply", "set_agent"}) {
		t.Errorf("allowed_actions %v", body.AllowedActions)
	}
	if body.Agent == nil || body.Agent.State != "off" || !body.AgentOff {
		t.Errorf("agent %+v, agent_off %v", body.Agent, body.AgentOff)
	}
	if !slices.Equal(thread.AllowedActions, []string{"reply"}) {
		t.Errorf("the thread's own moves changed to %v", thread.AllowedActions)
	}
	none := newThreadJSON(Thread{AgentState: AgentNone})
	if none.Agent != nil || none.AllowedActions == nil {
		t.Errorf("a thread with no agent: agent %+v, allowed_actions %v", none.Agent, none.AllowedActions)
	}
}
