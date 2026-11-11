package lifecycle

import (
	"slices"
	"strings"
)

type Doc struct {
	Title, Intro string
	Derived      bool
	States       []StateDoc
	Moves        []MoveDoc
}

type StateDoc struct {
	Name, Meaning string
	Terminal      bool
	Actions       []string
}

type MoveDoc struct {
	Action   string
	From, To []string
	Event    string
	Who      []string
}

func (m Machine[S, A]) Doc(title, intro string) Doc {
	d := Doc{Title: title, Intro: intro}
	for _, s := range m.States {
		actions := []string{}
		for _, a := range m.Allowed(s, func(Who) bool { return true }) {
			actions = append(actions, string(a))
		}
		d.States = append(d.States, StateDoc{
			Name:     string(s),
			Meaning:  m.Meaning[s],
			Terminal: slices.Contains(m.Terminal, s),
			Actions:  actions,
		})
	}
	for _, mv := range m.Moves {
		md := MoveDoc{Action: string(mv.Action)}
		for _, s := range mv.From {
			md.From = append(md.From, string(s))
		}
		for _, s := range mv.To {
			md.To = append(md.To, string(s))
		}
		for _, w := range mv.Who {
			md.Who = append(md.Who, w.String())
		}
		var events []string
		if mv.Event != "" {
			events = []string{mv.Event}
		} else {
			for _, s := range mv.To {
				events = append(events, m.Arrive[s])
			}
		}
		md.Event = codeList(events, " or ")
		d.Moves = append(d.Moves, md)
	}
	return d
}

const statesHeader = `---
title: States
description: Every state a run, comment thread and workspace object can be in, the actions each state allows and the activity event each move records.
---

This page is generated from the server's state tables by ` + "`make generate`" + `, and a test fails when it falls behind them.
An action listed for a state is offered in that object's ` + "`allowed_actions`" + ` to a caller who may take it.
`

func Render(docs ...Doc) string {
	var b strings.Builder
	b.WriteString(statesHeader)
	for _, d := range docs {
		if d.Derived {
			renderDerived(&b, d)
			continue
		}
		b.WriteString("\n## " + d.Title + "\n\n" + d.Intro + "\n\n")
		b.WriteString("| State | Meaning | Actions |\n| --- | --- | --- |\n")
		for _, s := range d.States {
			meaning := s.Meaning
			if s.Terminal {
				meaning += " Final."
			}
			b.WriteString("| `" + s.Name + "` | " + meaning + " | " + dashIfEmpty(codeList(s.Actions, ", ")) + " |\n")
		}
		if len(d.Moves) == 0 {
			continue
		}
		b.WriteString("\n| Action | From | To | Event | By |\n| --- | --- | --- | --- | --- |\n")
		for _, mv := range d.Moves {
			b.WriteString("| `" + mv.Action + "` | " + dashIfEmpty(codeList(mv.From, ", ")) + " | " + dashIfEmpty(codeList(mv.To, ", ")) +
				" | " + dashIfEmpty(mv.Event) + " | " + strings.Join(mv.Who, ", ") + " |\n")
		}
	}
	return b.String()
}

const derivedNote = "This state is computed from stored fields on every read; nothing stores it."

func renderDerived(b *strings.Builder, d Doc) {
	b.WriteString("\n## " + d.Title + "\n\n" + d.Intro + "\n" + derivedNote + "\n\n")
	b.WriteString("| State | When |\n| --- | --- |\n")
	for _, s := range d.States {
		b.WriteString("| `" + s.Name + "` | " + s.Meaning + " |\n")
	}
}

func codeList(items []string, sep string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = "`" + it + "`"
	}
	return strings.Join(quoted, sep)
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
