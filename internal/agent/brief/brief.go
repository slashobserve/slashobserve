package brief

import (
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const threadLead = "The request was asked in this thread (earlier messages, oldest first):"

const factsNote = "When you answer with save_finding, write each figure from a read that gave fact ids in `facts` as its fact, `{{f3}}`, which the server fills in (`{{f3:ms}}`, `{{f3:%}}` or `{{f3:3}}` change the unit or the figures within 1%), copy trace ids whole, and list each quantity asked in `answers` with its one fact."

const withheldAdvice = "When the request needs them, say so in your answer and do not guess their contents."

var clock = time.Now

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
	TimeZone      string
	AsOf          *time.Time
	Withheld      bool
	Pointer       Pointer
	Loop          bool
	Facts         bool
	Hypotheses    string
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
		m.System = investigateSystem(t)
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
	if t.Mode != tools.ModeSuggest {
		m.Context += "\n\n" + clockNote(t.TimeZone, t.now())
		if t.Facts && t.Mode != tools.ModeInvestigate {
			m.Context += "\n\n" + factsNote
		}
	}
	if note := anchorNote(t.Pointer); note != "" {
		m.Context += "\n\n" + note
	}
	if t.ThreadContext != "" {
		m.Context += "\n\n" + threadLead + "\n" + t.ThreadContext
	}
	if t.Withheld {
		m.Context += "\n\n" + datasource.WithheldNote + " " + withheldAdvice
	}
	if t.AsOf != nil {
		m.Context += "\n\n" + asOfNote(*t.AsOf)
	}
	if digest != "" && t.Mode != tools.ModeSuggest {
		m.Context += "\n\n" + digest
	}
	if t.Hypotheses != "" && t.Mode != tools.ModeSuggest {
		m.Context += "\n\n" + t.Hypotheses
	}
	m.User = userMessage(m.Context, t.Prompt)
	return m, nil
}

func investigateSystem(t Task) string {
	switch {
	case t.Facts:
		return prompts.InvestigateWithFacts(t.Loop)
	case t.Loop:
		return prompts.InvestigateWithLedger()
	}
	return prompts.Investigate()
}

func userMessage(context, prompt string) string {
	if context == "" {
		return prompt
	}
	return context + "\n\n<request>\n" + prompt + "\n</request>"
}

func (t Task) now() time.Time {
	if t.AsOf != nil {
		return *t.AsOf
	}
	return clock()
}

func asOfNote(at time.Time) string {
	return fmt.Sprintf("Now is %s for this run: every relative time, such as now-1h, resolves to it, and the data may end there.", at.UTC().Format(time.RFC3339))
}
