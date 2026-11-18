package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

type adminData struct {
	Name         string          `json:"name"`
	FromRole     string          `json:"from_role"`
	ToRole       string          `json:"to_role"`
	Manual       bool            `json:"manual"`
	Sessions     int             `json:"sessions"`
	Grants       int             `json:"grants"`
	KeyName      string          `json:"key_name"`
	Client       string          `json:"client"`
	PersonName   string          `json:"person_name"`
	Reason       string          `json:"reason"`
	Domain       string          `json:"domain"`
	Host         string          `json:"host"`
	URLChanged   bool            `json:"url_changed"`
	TokenChanged bool            `json:"token_changed"`
	Role         json.RawMessage `json:"role"`
	TakesRuns    json.RawMessage `json:"takes_runs"`
	Enabled      json.RawMessage `json:"enabled"`
	EventTypes   json.RawMessage `json:"event_types"`
}

func (d activityData) admin() adminData {
	var out adminData
	_ = json.Unmarshal(d.raw, &out)
	return out
}

func quotedName(kind, name string) string {
	if name == "" {
		return "a " + kind
	}
	return "the " + kind + ` "` + text(name) + `"`
}

func changeText(raw json.RawMessage) string {
	var pair struct {
		From json.RawMessage `json:"from"`
		To   json.RawMessage `json:"to"`
	}
	if err := json.Unmarshal(raw, &pair); err == nil && pair.From != nil {
		return text(strings.Trim(string(pair.From), `"`)) + " to " + text(strings.Trim(string(pair.To), `"`))
	}
	return text(strings.Trim(string(raw), `"`))
}

func changes(parts map[string]json.RawMessage, order ...string) string {
	var out []string
	for _, key := range order {
		if raw := parts[key]; len(raw) > 0 {
			out = append(out, strings.ReplaceAll(key, "_", " ")+" "+changeText(raw))
		}
	}
	return strings.Join(out, ", ")
}

func describeAdmin(e activityEvent) (string, bool) {
	d := e.Data.admin()
	switch e.Type {
	case "user.role_changed":
		if !d.Manual {
			return fmt.Sprintf("cleared the role override of %s; the role is now %s (was %s)", text(d.Name), text(d.ToRole), text(d.FromRole)), true
		}
		return fmt.Sprintf("changed the role of %s from %s to %s", text(d.Name), text(d.FromRole), text(d.ToRole)), true
	case "user.signed_out_everywhere":
		return fmt.Sprintf("signed %s out everywhere (%d sessions, %d connected agents)", text(d.Name), d.Sessions, d.Grants), true
	case "service_account.created":
		return fmt.Sprintf("created %s (%s)", quotedName("service account", d.Name), changeText(d.Role)), true
	case "service_account.updated":
		return "changed " + quotedName("service account", d.Name) + ": " + changes(map[string]json.RawMessage{"role": d.Role, "takes_runs": d.TakesRuns}, "role", "takes_runs"), true
	case "service_account.key_created":
		return fmt.Sprintf("created the key %q for %s", text(d.KeyName), quotedName("service account", d.Name)), true
	case "service_account.key_revoked":
		return fmt.Sprintf("revoked the key %q of %s", text(d.KeyName), quotedName("service account", d.Name)), true
	case "webhook.created":
		return "created " + quotedName("webhook", d.Name) + " to " + text(d.Host), true
	case "webhook.updated":
		return "changed " + quotedName("webhook", d.Name) + secretNote(d), true
	case "webhook.secret_rotated":
		return "rotated the secret of " + quotedName("webhook", d.Name), true
	case "webhook.deleted":
		return "deleted " + quotedName("webhook", d.Name), true
	case "mcp_client.connected":
		return fmt.Sprintf("connected %s for %s", quotedName("agent", d.Client), text(d.PersonName)), true
	case "mcp_client.disconnected":
		return fmt.Sprintf("disconnected %s of %s (%s)", quotedName("agent", d.Client), text(d.PersonName), text(d.Reason)), true
	case "outbound_domain.added":
		return "allowed the outbound domain " + text(d.Domain), true
	case "outbound_domain.removed":
		return "removed the outbound domain " + text(d.Domain), true
	case "grafana_connection.created":
		return "connected Grafana " + text(d.Host) + " as " + quotedName("connection", d.Name), true
	case "grafana_connection.updated":
		return "changed " + quotedName("Grafana connection", d.Name) + secretNote(d), true
	case "grafana_connection.deleted":
		return "removed " + quotedName("Grafana connection", d.Name), true
	}
	return "", false
}

func secretNote(d adminData) string {
	switch {
	case d.TokenChanged:
		return " (token replaced)"
	case d.URLChanged:
		return " (address changed)"
	}
	return ""
}
