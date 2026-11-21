package auditsource

import (
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
)

type document struct {
	trail.Event
	Message string `json:"message"`
}

func documentOf(e trail.Event) document {
	return document{Event: e, Message: Line(e)}
}

func Line(e trail.Event) string {
	var b strings.Builder
	b.WriteString(e.Action + " " + e.Outcome)
	if e.Reason != nil {
		b.WriteString(" (" + *e.Reason + ")")
	}
	b.WriteString(" by " + actorOf(e))
	if e.AgentName != nil && e.Initiator == "agent" {
		b.WriteString(" with agent " + strconv.Quote(*e.AgentName))
	}
	b.WriteString(" via " + e.Origin)
	if e.TargetKind != nil {
		b.WriteString(" on " + *e.TargetKind)
		if name := firstSet(e.TargetName, e.TargetID); name != "" {
			b.WriteString(" " + strconv.Quote(name))
		}
	}
	return b.String()
}

func actorOf(e trail.Event) string {
	if name := firstSet(e.ActorName); name != "" {
		return name
	}
	return e.ActorKind
}

func firstSet(values ...*string) string {
	for _, v := range values {
		if v != nil && *v != "" {
			return *v
		}
	}
	return ""
}

type capability struct {
	Type         string `json:"type"`
	Searchable   bool   `json:"searchable"`
	Aggregatable bool   `json:"aggregatable"`
}

var capabilityTypes = map[filter.Type]string{
	filter.TypeInt:  "long",
	filter.TypeIP:   "ip",
	filter.TypeDate: "date_nanos",
}

func fieldCaps() map[string]any {
	fields := map[string]any{
		MessageField: map[string]capability{"text": {Type: "text"}},
	}
	for _, f := range filter.Fields() {
		kind, ok := capabilityTypes[f.Type]
		if !ok {
			kind = "keyword"
		}
		fields[f.Name] = map[string]capability{kind: {Type: kind, Searchable: true, Aggregatable: f.Groupable()}}
	}
	return map[string]any{"indices": []string{Index}, "fields": fields}
}

func fieldAliases() map[string]string {
	aliases := map[string]string{}
	for _, f := range filter.Fields() {
		for _, alias := range f.Aliases {
			aliases[alias] = f.Name
		}
	}
	return aliases
}
