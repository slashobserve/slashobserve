package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

type adminData struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	FromRole   string `json:"from_role"`
	ToRole     string `json:"to_role"`
	Manual     bool   `json:"manual"`
	Source     string `json:"source"`
	Sessions   int    `json:"sessions"`
	Grants     int    `json:"grants"`
	KeyName    string `json:"key_name"`
	Client     string `json:"client"`
	PersonName string `json:"person_name"`
	Reason     string `json:"reason"`
	Domain     string `json:"domain"`
	Title      string `json:"title"`
	Object     struct {
		Kind string `json:"kind"`
	} `json:"object"`
	Host         string                     `json:"host"`
	URLChanged   bool                       `json:"url_changed"`
	TokenChanged bool                       `json:"token_changed"`
	Role         json.RawMessage            `json:"role"`
	TakesRuns    json.RawMessage            `json:"takes_runs"`
	Changes      map[string]json.RawMessage `json:"changes"`
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
		return valueText(pair.From) + " to " + valueText(pair.To)
	}
	return text(strings.Trim(string(raw), `"`))
}

func valueText(raw json.RawMessage) string {
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		return fmt.Sprint(len(list))
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
	if line, ok := describeAccess(e.Type, d); ok {
		return line, true
	}
	if line, ok := describePeople(e.Type, d); ok {
		return line, true
	}
	if line, ok := describePublicLink(e.Type, d); ok {
		return line, true
	}
	return describeIntegration(e.Type, d)
}

func describeAccess(typ string, d adminData) (string, bool) {
	switch typ {
	case "user.role_changed":
		if d.Source != "" {
			return fmt.Sprintf("had the role of %s changed from %s to %s by the sign-in policy (%s)", text(d.Name), text(d.FromRole), text(d.ToRole), text(d.Source)), true
		}
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
	case "service_account.deleted":
		return "deleted " + quotedName("service account", d.Name), true
	case "service_account.key_created":
		return fmt.Sprintf("created the key %q for %s", text(d.KeyName), quotedName("service account", d.Name)), true
	case "service_account.key_revoked":
		return fmt.Sprintf("revoked the key %q of %s", text(d.KeyName), quotedName("service account", d.Name)), true
	}
	return "", false
}

func describePeople(typ string, d adminData) (string, bool) {
	switch typ {
	case "user.invited":
		return fmt.Sprintf("invited %s as %s", text(d.Email), changeText(d.Role)), true
	case "user.invite_updated":
		return "changed the invite of " + text(d.Email) + ": " + changes(map[string]json.RawMessage{"role": d.Role}, "role"), true
	case "user.invite_withdrawn":
		return "withdrew the invite of " + text(d.Email), true
	case "user.deactivated":
		return fmt.Sprintf("deactivated %s (%d sessions, %d connected agents)", text(d.Name), d.Sessions, d.Grants), true
	case "user.reactivated":
		return "reactivated " + text(d.Name), true
	case "user.removed":
		return "removed " + text(d.Name), true
	}
	return "", false
}

func describePublicLink(typ string, d adminData) (string, bool) {
	verb, ok := map[string]string{"public_link.created": "created", "public_link.revoked": "revoked"}[typ]
	if !ok {
		return "", false
	}
	kind := "dashboard"
	if d.Object.Kind == "observation" {
		kind = "RCA"
	}
	return verb + " a public link to " + quotedName(kind, d.Title), true
}

func describeIntegration(typ string, d adminData) (string, bool) {
	switch typ {
	case "webhook.created":
		return "created " + quotedName("webhook", d.Name) + " to " + text(d.Host), true
	case "webhook.updated":
		return changedText(quotedName("webhook", d.Name), d, "name", "host", "enabled", "event_types"), true
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
		return changedText(quotedName("Grafana connection", d.Name), d, "name", "host"), true
	case "grafana_connection.deleted":
		return "removed " + quotedName("Grafana connection", d.Name), true
	}
	return "", false
}

func changedText(subject string, d adminData, order ...string) string {
	out := "changed " + subject
	if parts := changes(d.Changes, order...); parts != "" {
		out += ": " + parts
	}
	return out + secretNote(d)
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
