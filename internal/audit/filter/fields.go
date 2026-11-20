package filter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/kql"
)

type Type string

const (
	TypeText   Type = "text"
	TypeEnum   Type = "enum"
	TypeInt    Type = "int"
	TypeUUID   Type = "uuid"
	TypeIP     Type = "ip"
	TypeDate   Type = "date"
	TypeTarget Type = "target"
)

type Field struct {
	Name        string
	Alias       string
	Type        Type
	Description string
	Values      []string
	column      string
}

const (
	tableName      = "audit_events"
	durationField  = "duration_ms"
	occurredField  = "occurred_at"
	targetFieldKey = "target"
)

var (
	categoryValues   = []string{"auth", "access", "content", "conversation", "agent", "query", "admin", "system"}
	outcomeValues    = []string{"success", "denied", "failed"}
	actorKindValues  = []string{"user", "service_account", "anonymous", "system"}
	initiatorValues  = []string{"person", "agent", "system"}
	authMethodValues = []string{"session", "api_key", "oauth", "public_link", "none"}
	originValues     = []string{"web", "cli", "mcp", "api", "agent", "job", "provision"}
)

var fieldList = []Field{
	{Name: occurredField, Type: TypeDate, column: occurredField, Description: "When the event happened, as an RFC 3339 time or a date"},
	{Name: "id", Type: TypeUUID, column: "id", Description: "Event id"},
	{Name: "category", Type: TypeEnum, column: "category", Values: categoryValues, Description: "Kind of action: auth, access, content, conversation, agent, query, admin or system"},
	{Name: "action", Type: TypeText, column: "action", Description: "What was attempted, such as dashboard.update; end with * for a prefix such as dashboard.*"},
	{Name: "outcome", Type: TypeEnum, column: "outcome", Values: outcomeValues, Description: "success, denied or failed"},
	{Name: "reason", Type: TypeText, column: "reason", Description: "API error code of a refusal or failure, such as forbidden_role"},
	{Name: "status", Type: TypeInt, column: "status", Description: "HTTP status of the request"},
	{Name: durationField, Alias: "duration", Type: TypeInt, column: durationField, Description: "How long the action took, in milliseconds"},
	{Name: "actor_kind", Type: TypeEnum, column: "actor_kind", Values: actorKindValues, Description: "Whose permissions were used: user, service_account, anonymous or system"},
	{Name: "actor_id", Type: TypeUUID, column: "actor_id", Description: "Id of the user or service account"},
	{Name: "actor_name", Alias: "actor", Type: TypeText, column: "actor_name", Description: "Name of the actor when the event happened"},
	{Name: "actor_role", Type: TypeText, column: "actor_role", Description: "Role of the actor when the event happened"},
	{Name: "initiator", Type: TypeEnum, column: "initiator", Values: initiatorValues, Description: "Who started it: person, agent or system"},
	{Name: "agent_id", Type: TypeUUID, column: "agent_id", Description: "Id of the agent that acted"},
	{Name: "agent_name", Type: TypeText, column: "agent_name", Description: "Name of the agent that acted"},
	{Name: "run_id", Alias: "run", Type: TypeUUID, column: "run_id", Description: "Agent run that made the event"},
	{Name: "auth_method", Type: TypeEnum, column: "auth_method", Values: authMethodValues, Description: "How the caller proved who they are: session, api_key, oauth, public_link or none"},
	{Name: "credential_id", Type: TypeText, column: "credential_id", Description: "Id of the session, key, grant or public link used, never the secret"},
	{Name: "origin", Type: TypeEnum, column: "origin", Values: originValues, Description: "Where it came from: web, cli, mcp, api, agent, job or provision"},
	{Name: "origin_client", Type: TypeText, column: "origin_client", Description: "Name of the connected client"},
	{Name: "tool", Type: TypeText, column: "tool", Description: "MCP or agent tool name"},
	{Name: "client_ip", Alias: "ip", Type: TypeIP, column: "client_ip", Description: "Address of the caller; accepts an address or a CIDR such as 10.4.0.0/16"},
	{Name: "user_agent", Type: TypeText, column: "user_agent", Description: "User agent header of the request"},
	{Name: "request_id", Alias: "request", Type: TypeUUID, column: "request_id", Description: "Request id, also returned as X-Request-Id"},
	{Name: "trace_id", Type: TypeText, column: "trace_id", Description: "Trace id of the request, when one was sent"},
	{Name: "http_method", Type: TypeText, column: "http_method", Description: "HTTP method of the request"},
	{Name: "route", Type: TypeText, column: "route", Description: "Route template of the request"},
	{Name: targetFieldKey, Type: TypeTarget, Description: "What the event was about as kind/id, such as dashboard/8f3c; kind alone and kind/id* also work"},
	{Name: "target_kind", Type: TypeText, column: "target_kind", Description: "Kind of the target, such as dashboard"},
	{Name: "target_id", Type: TypeText, column: "target_id", Description: "Id of the target"},
	{Name: "target_name", Type: TypeText, column: "target_name", Description: "Name of the target when the event happened"},
	{Name: "folder_id", Type: TypeUUID, column: "folder_id", Description: "Folder the target lives in"},
	{Name: "dashboard_id", Type: TypeUUID, column: "dashboard_id", Description: "Dashboard the event touched"},
	{Name: "observation_id", Type: TypeUUID, column: "observation_id", Description: "Observation the event touched"},
	{Name: "thread_id", Type: TypeUUID, column: "thread_id", Description: "Comment thread the event touched"},
	{Name: "chat_id", Type: TypeUUID, column: "chat_id", Description: "Chat the event touched"},
	{Name: "datasource_uid", Alias: "datasource", Type: TypeText, column: "datasource_uid", Description: "Datasource a query ran on"},
	{Name: "server_id", Type: TypeText, column: "server_id", Description: "Server replica that recorded the event"},
	{Name: "server_version", Type: TypeText, column: "server_version", Description: "Build of the server that recorded the event"},
}

var eventColumns = []string{
	"id", "seq", "occurred_at", "category", "action", "outcome", "reason", "status", "duration_ms",
	"actor_kind", "actor_id", "actor_name", "actor_role", "initiator", "agent_id", "agent_name", "run_id",
	"auth_method", "credential_id", "origin", "origin_client", "tool", "client_ip", "user_agent",
	"request_id", "trace_id", "http_method", "route", "target_kind", "target_id", "target_name",
	"folder_id", "dashboard_id", "observation_id", "thread_id", "chat_id", "datasource_uid", "data",
	"server_id", "server_version",
}

var (
	fieldIndex = indexFields(fieldList)
	fieldNames = knownNames(fieldList)
)

func Fields() []Field {
	out := make([]Field, len(fieldList))
	for i, f := range fieldList {
		f.Values = slices.Clone(f.Values)
		out[i] = f
	}
	return out
}

func indexFields(list []Field) map[string]*Field {
	index := make(map[string]*Field, 2*len(list))
	for i := range list {
		f := &list[i]
		index[f.Name] = f
		if f.Alias != "" {
			index[f.Alias] = f
		}
	}
	return index
}

func knownNames(list []Field) []string {
	var names []string
	for _, f := range list {
		names = append(names, f.Name)
		if f.Alias != "" {
			names = append(names, f.Alias)
		}
	}
	return names
}

func lookupField(name string) (*Field, bool) {
	f, ok := fieldIndex[strings.ToLower(name)]
	return f, ok
}

func (f *Field) groupable() bool {
	return f.Type != TypeTarget
}

func (f *Field) textExpr() string {
	switch f.Type {
	case TypeIP:
		return "host(" + f.column + ")"
	case TypeTarget:
		return "concat(target_kind, chr(47) || target_id)"
	case TypeText:
		return f.column
	default:
		return f.column + "::text"
	}
}

func (f *Field) nullColumn() string {
	if f.Type == TypeTarget {
		return "target_kind"
	}
	return f.column
}

func kqlFields() kql.Fields {
	kinds := map[Type]kql.FieldKind{TypeInt: kql.Number, TypeDate: kql.Date, TypeIP: kql.IP}
	out := make(kql.Fields, len(fieldIndex))
	for name, f := range fieldIndex {
		kind, ok := kinds[f.Type]
		if !ok {
			kind = kql.Keyword
		}
		out[name] = kind
	}
	return out
}

func matchingFields(pattern string) []*Field {
	var out []*Field
	for i := range fieldList {
		f := &fieldList[i]
		if f.Type != TypeTarget && globMatch(pattern, f.Name) {
			out = append(out, f)
		}
	}
	return out
}

func globMatch(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	if !strings.HasPrefix(name, parts[0]) || !strings.HasSuffix(name, parts[len(parts)-1]) {
		return false
	}
	name = name[len(parts[0]):]
	for _, part := range parts[1 : len(parts)-1] {
		at := strings.Index(name, part)
		if at < 0 {
			return false
		}
		name = name[at+len(part):]
	}
	return len(name) >= len(parts[len(parts)-1])
}

func nearest(name string, candidates []string) string {
	name = strings.ToLower(name)
	for _, c := range candidates {
		if len(name) >= 3 && strings.HasPrefix(c, name) {
			return c
		}
	}
	limit := max(1, len(name)/3)
	best, bestDistance := "", limit+1
	for _, c := range candidates {
		if d := editDistance(name, c); d < bestDistance {
			best, bestDistance = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func unknownFieldMessage(name string) string {
	if suggestion := nearest(name, fieldNames); suggestion != "" {
		return fmt.Sprintf("unknown field %q; did you mean %q?", name, suggestion)
	}
	return fmt.Sprintf("unknown field %q", name)
}
