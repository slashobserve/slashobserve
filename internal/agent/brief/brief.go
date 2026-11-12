package brief

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const threadLead = "The request was asked in this thread (earlier messages, oldest first):"

type Task struct {
	Mode          tools.Mode
	Source        revisions.Source
	Prompt        string
	PanelID       string
	From, To      *time.Time
	ContinueNote  string
	ThreadContext string
	Continues     bool
	Attempt       int
}

type View struct {
	Current *Revision
	Build   *dashboards.Build
}

type Message struct {
	System  string
	Context string
	User    string
}

func Render(statuses []datasource.Status, t Task, v View, r Reason, digest string) (Message, error) {
	sd := seed{build: v.Build, reason: r, note: t.ContinueNote}
	var m Message
	if t.Mode == tools.ModeInvestigate {
		m.System = prompts.Investigate()
		m.Context = investigateContext(statuses, t, v.Current, sd)
	} else {
		view := v.Current
		if v.Build != nil {
			view = buildView(v.Current, v.Build)
		}
		body, err := buildContext(statuses, t.Mode, t.Source, t.PanelID, t.Prompt, view, sd)
		if err != nil {
			return Message{}, err
		}
		m.System, m.Context = systemFor(statuses), body
	}
	if t.ThreadContext != "" {
		m.Context += "\n\n" + threadLead + "\n" + t.ThreadContext
	}
	if digest != "" && t.Mode != tools.ModeSuggest {
		m.Context += "\n\n" + digest
	}
	m.User = userMessage(m.Context, t.Prompt)
	return m, nil
}

func userMessage(context, prompt string) string {
	if context == "" {
		return prompt
	}
	return context + "\n\n<request>\n" + prompt + "\n</request>"
}
