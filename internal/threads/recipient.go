package threads

import "github.com/slashobserve/slashobserve/internal/revisions"

type To string

const (
	ToAuto     To = ""
	ToAgent    To = "agent"
	ToEveryone To = "everyone"
)

type stickyChange int

const (
	stickyKeep stickyChange = iota
	stickyOff
	stickyFollow
)

type threadAgent struct {
	Agent *revisions.Assignee
	Off   bool
}

type Recipient struct {
	Agent       *revisions.Assignee
	Sticky      stickyChange
	Unavailable bool
}

var errTooManyAgents = &InvalidInputError{Code: CodeTooManyAgents, Message: "start a run for one agent at a time: mention only one of @observe, a connected agent or a service account"}

func recipientOf(thread threadAgent, body string, to To, builtinAgent bool) (Recipient, error) {
	mentions, err := ParseMentions(body)
	if err != nil {
		return Recipient{}, err
	}
	agents := mentions.Agents()
	switch {
	case len(agents) > 1:
		return Recipient{}, errTooManyAgents
	case to == ToEveryone:
		if thread.Agent != nil {
			return Recipient{Sticky: stickyOff}, nil
		}
	case len(agents) == 1:
		return asking(assigneeOf(agents[0]), stickyFollow, builtinAgent), nil
	case to == ToAgent:
		agent := revisions.Assignee{Kind: revisions.AssignBuiltin}
		if thread.Agent != nil {
			agent = *thread.Agent
		}
		return asking(agent, stickyFollow, builtinAgent), nil
	case len(mentions) > 0:
	case thread.Agent != nil && !thread.Off:
		return asking(*thread.Agent, stickyKeep, builtinAgent), nil
	}
	return Recipient{}, nil
}

func asking(agent revisions.Assignee, sticky stickyChange, builtinAgent bool) Recipient {
	if agent.Kind == revisions.AssignBuiltin && !builtinAgent {
		return Recipient{Unavailable: true}
	}
	return Recipient{Agent: &agent, Sticky: sticky}
}

func assigneeOf(agent Mention) revisions.Assignee {
	switch agent.Kind() {
	case mentionGrant:
		return revisions.Assignee{Kind: revisions.AssignGrant, ID: agent.GrantID}
	case mentionServiceAccount:
		return revisions.Assignee{Kind: revisions.AssignServiceAccount, ID: agent.ServiceAccountID}
	}
	return revisions.Assignee{Kind: revisions.AssignBuiltin}
}
