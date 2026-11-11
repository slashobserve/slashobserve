package actor

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/origin"
)

type Actor struct {
	Kind   auth.PrincipalKind
	ID     string
	Name   string
	Origin origin.Origin
	RunID  string
}

func Of(p auth.Principal, o origin.Origin, runID string) Actor {
	return Actor{Kind: p.Kind, ID: p.ID, Name: cmp.Or(p.Name, p.Email), Origin: o, RunID: runID}
}

type Columns struct {
	Kind   string
	ID     pgtype.UUID
	Name   string
	Origin pgtype.Text
	Client pgtype.Text
	RunID  pgtype.UUID
}

func (a Actor) Columns() (Columns, error) {
	c := Columns{Kind: string(a.Kind), Name: a.Name}
	if err := c.ID.Scan(a.ID); err != nil {
		return Columns{}, fmt.Errorf("actor id: %w", err)
	}
	if a.RunID != "" {
		if err := c.RunID.Scan(a.RunID); err != nil {
			return Columns{}, fmt.Errorf("actor run id: %w", err)
		}
	}
	c.Origin, c.Client = a.Origin.Columns()
	return c, nil
}

func FromColumns(c Columns) Actor {
	return Actor{
		Kind: auth.PrincipalKind(c.Kind),
		ID:   c.ID.String(),
		Name: c.Name,
		Origin: origin.Origin{
			Kind:   origin.Kind(textOf(c.Origin)),
			Client: textOf(c.Client),
		},
		RunID: c.RunID.String(),
	}
}

func textOf(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

var clientNames = map[string]string{
	"claude-code":        "Claude Code",
	"claude code":        "Claude Code",
	"claude-ai":          "Claude",
	"cursor":             "Cursor",
	"cursor-vscode":      "Cursor",
	"opencode":           "OpenCode",
	"codex":              "Codex",
	"codex-mcp-client":   "Codex",
	"windsurf":           "Windsurf",
	"zed":                "Zed",
	"vscode":             "VS Code",
	"visual studio code": "VS Code",
}

func ClientName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if name, ok := clientNames[strings.ToLower(trimmed)]; ok {
		return name
	}
	return trimmed
}

func (a Actor) Label() string {
	name := cmp.Or(a.Name, "Unknown")
	switch {
	case a.RunID != "":
		return "Observe Agent for " + name
	case a.Origin.Kind == origin.MCP:
		return name + " (via " + cmp.Or(ClientName(a.Origin.Client), "MCP") + ")"
	default:
		return name
	}
}

type JSON struct {
	Kind   string  `json:"kind"`
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Origin *string `json:"origin"`
	Client *string `json:"client"`
	RunID  *string `json:"run_id"`
	Label  string  `json:"label"`
}

func (a Actor) JSON() JSON {
	j := JSON{
		Kind:   string(a.Kind),
		ID:     a.ID,
		Name:   a.Name,
		Origin: nonEmpty(string(a.Origin.Kind)),
		RunID:  nonEmpty(a.RunID),
		Label:  a.Label(),
	}
	if a.Origin.Kind == origin.MCP {
		j.Client = nonEmpty(ClientName(a.Origin.Client))
	}
	return j
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
