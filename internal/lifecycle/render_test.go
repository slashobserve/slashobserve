package lifecycle_test

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

func TestRenderWritesTheStatesPage(t *testing.T) {
	m := lifecycle.Machine[state, action]{
		Name:     "ticket",
		States:   []state{open, closed},
		Terminal: []state{closed},
		Meaning:  map[state]string{open: "Waiting.", closed: "Done."},
		Arrive:   map[state]string{open: "ticket.opened", closed: "ticket.closed"},
		Moves: []lifecycle.Move[state, action]{
			{Action: "create", To: []state{open}, Who: []lifecycle.Who{lifecycle.Requester}},
			{Action: "close", From: []state{open}, To: []state{open, closed}, Who: []lifecycle.Who{lifecycle.Requester, lifecycle.System}},
			{Action: "pin", From: []state{open, closed}, Event: "ticket.pinned", Who: []lifecycle.Who{lifecycle.Starter}},
			{Action: "lock", From: []state{open}, Who: []lifecycle.Who{lifecycle.System}},
		},
	}
	got := lifecycle.Render(m.Doc("Ticket", "A ticket is one thing to do."))
	want := strings.Join([]string{
		"---",
		"title: States",
		"description: Every state a run, comment thread and workspace object can be in, the actions each state allows and the activity event each move records.",
		"---",
		"",
		"This page is generated from the server's state tables by `make generate`, and a test fails when it falls behind them.",
		"An action listed for a state is offered in that object's `allowed_actions` to a caller who may take it.",
		"",
		"## Ticket",
		"",
		"A ticket is one thing to do.",
		"",
		"| State | Meaning | Actions |",
		"| --- | --- | --- |",
		"| `open` | Waiting. | `close`, `pin` |",
		"| `closed` | Done. Final. | `pin` |",
		"",
		"| Action | From | To | Event | By |",
		"| --- | --- | --- | --- | --- |",
		"| `create` | - | `open` | `ticket.opened` | requester |",
		"| `close` | `open` | `open`, `closed` | `ticket.opened` or `ticket.closed` | requester, server |",
		"| `pin` | `open`, `closed` | - | `ticket.pinned` | anyone who may start one |",
		"| `lock` | `open` | - | - | server |",
		"",
	}, "\n")
	if got != want {
		t.Errorf("Render mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderOmitsTheMovesTableWhenThereAreNoMoves(t *testing.T) {
	doc := lifecycle.Doc{
		Title:  "Derived",
		Intro:  "Computed on read.",
		States: []lifecycle.StateDoc{{Name: "ok", Meaning: "Fine."}},
	}
	got := lifecycle.Render(doc)
	if strings.Contains(got, "| Action |") {
		t.Errorf("moves table rendered for a doc with no moves:\n%s", got)
	}
	if !strings.Contains(got, "| `ok` | Fine. | - |\n") || !strings.HasSuffix(got, "|\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("unexpected rendering:\n%s", got)
	}
}
