package threads

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type recipientAgent struct {
	Kind string  `json:"kind"`
	ID   *string `json:"id"`
}

type recipientCase struct {
	Name   string `json:"name"`
	Thread struct {
		Agent *recipientAgent `json:"agent"`
		Off   bool            `json:"off"`
	} `json:"thread"`
	Body         string `json:"body"`
	To           To     `json:"to"`
	BuiltinAgent bool   `json:"builtinAgent"`
	Want         struct {
		Error       *string         `json:"error"`
		Agent       *recipientAgent `json:"agent"`
		Sticky      string          `json:"sticky"`
		Unavailable bool            `json:"unavailable"`
	} `json:"want"`
}

func readRecipientCases(t *testing.T) []recipientCase {
	t.Helper()
	root, err := os.OpenRoot("../../spec/examples")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile("threads/recipient-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []recipientCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func assigneeFrom(t *testing.T, a *recipientAgent) *revisions.Assignee {
	t.Helper()
	if a == nil {
		return nil
	}
	assignee := revisions.Assignee{Kind: revisions.AssigneeKind(a.Kind)}
	if a.ID != nil {
		id, err := db.ParseUUID(*a.ID)
		if err != nil {
			t.Fatal(err)
		}
		assignee.ID = id
	}
	return &assignee
}

func agentLabel(a *revisions.Assignee) string {
	if a == nil {
		return "none"
	}
	if !a.ID.Valid {
		return string(a.Kind)
	}
	return string(a.Kind) + ":" + a.ID.String()
}

func stickyName(s stickyChange) string {
	switch s {
	case stickyOff:
		return "off"
	case stickyFollow:
		return "follow"
	}
	return "keep"
}

func TestRecipientOfSharedCases(t *testing.T) {
	cases := readRecipientCases(t)
	if len(cases) < 40 {
		t.Fatalf("%d cases, want at least 40", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			thread := threadAgent{Agent: assigneeFrom(t, c.Thread.Agent), Off: c.Thread.Off}
			got, err := recipientOf(thread, c.Body, c.To, c.BuiltinAgent)
			if c.Want.Error != nil {
				var invalid *InvalidInputError
				if !errors.As(err, &invalid) || invalid.Code != *c.Want.Error {
					t.Fatalf("error = %v, want code %s", err, *c.Want.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := assigneeFrom(t, c.Want.Agent); agentLabel(got.Agent) != agentLabel(want) {
				t.Errorf("agent = %s, want %s", agentLabel(got.Agent), agentLabel(want))
			}
			if name := stickyName(got.Sticky); name != c.Want.Sticky {
				t.Errorf("sticky = %s, want %s", name, c.Want.Sticky)
			}
			if got.Unavailable != c.Want.Unavailable {
				t.Errorf("unavailable = %v, want %v", got.Unavailable, c.Want.Unavailable)
			}
		})
	}
}

func TestRecipientOfRejectsMoreThanTwentyMentions(t *testing.T) {
	body := ""
	for range MaxMentions + 1 {
		body += "@observe "
	}
	_, err := recipientOf(threadAgent{}, body, ToAuto, true)
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeTooManyMentions {
		t.Fatalf("error = %v, want code %s", err, CodeTooManyMentions)
	}
}
