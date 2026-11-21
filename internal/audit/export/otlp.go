package export

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
)

const (
	serviceName = "slashobserve"
	scopeName   = "slashobserve.audit"
	prefix      = "slashobserve.audit."
)

const (
	severityInfo  = 9
	severityWarn  = 13
	severityError = 17
)

type anyValue struct {
	String *string `json:"stringValue,omitempty"`
	Int    *string `json:"intValue,omitempty"`
}

type attribute struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

type logRecord struct {
	TimeUnixNano   string      `json:"timeUnixNano"`
	SeverityNumber int         `json:"severityNumber"`
	SeverityText   string      `json:"severityText"`
	Body           anyValue    `json:"body"`
	Attributes     []attribute `json:"attributes"`
	TraceID        string      `json:"traceId,omitempty"`
	EventName      string      `json:"eventName"`
}

type scope struct {
	Name string `json:"name"`
}

type scopeLogs struct {
	Scope      scope       `json:"scope"`
	LogRecords []logRecord `json:"logRecords"`
}

type resource struct {
	Attributes []attribute `json:"attributes"`
}

type resourceLogs struct {
	Resource  resource    `json:"resource"`
	ScopeLogs []scopeLogs `json:"scopeLogs"`
}

type request struct {
	ResourceLogs []resourceLogs `json:"resourceLogs"`
}

func Encode(events []trail.Event) ([]byte, error) {
	records := make([]logRecord, 0, len(events))
	for _, e := range events {
		records = append(records, recordOf(e))
	}
	return json.Marshal(request{ResourceLogs: []resourceLogs{{
		Resource:  resource{Attributes: []attribute{text("service.name", serviceName)}},
		ScopeLogs: []scopeLogs{{Scope: scope{Name: scopeName}, LogRecords: records}},
	}}})
}

func recordOf(e trail.Event) logRecord {
	number, name := severityOf(e.Outcome)
	line := auditsource.Line(e)
	r := logRecord{
		TimeUnixNano:   strconv.FormatInt(unixNano(e.OccurredAt), 10),
		SeverityNumber: number,
		SeverityText:   name,
		Body:           anyValue{String: &line},
		EventName:      e.Action,
	}
	if e.TraceID != nil && validTraceID(*e.TraceID) {
		r.TraceID = *e.TraceID
	}
	var a attributes
	a.text("event.name", &e.Action)
	a.text("user.id", e.ActorID)
	a.text("client.address", e.ClientIP)
	a.text("user_agent.original", e.UserAgent)
	a.text("http.route", e.Route)
	a.text("http.request.method", e.HTTPMethod)
	a.number("http.response.status_code", e.Status)
	a.text(prefix+"id", &e.ID)
	a.integer(prefix+"seq", e.Seq)
	a.text(prefix+"category", &e.Category)
	a.text(prefix+"outcome", &e.Outcome)
	a.text(prefix+"reason", e.Reason)
	a.number(prefix+"duration_ms", e.DurationMs)
	a.text(prefix+"actor_kind", &e.ActorKind)
	a.text(prefix+"actor_name", e.ActorName)
	a.text(prefix+"actor_role", e.ActorRole)
	a.text(prefix+"initiator", &e.Initiator)
	a.text(prefix+"agent_id", e.AgentID)
	a.text(prefix+"agent_name", e.AgentName)
	a.text(prefix+"run_id", e.RunID)
	a.text(prefix+"auth_method", e.AuthMethod)
	a.text(prefix+"credential_id", e.CredentialID)
	a.text(prefix+"origin", &e.Origin)
	a.text(prefix+"origin_client", e.OriginClient)
	a.text(prefix+"tool", e.Tool)
	a.text(prefix+"request_id", e.RequestID)
	a.text(prefix+"trace_id", e.TraceID)
	a.text(prefix+"target_kind", e.TargetKind)
	a.text(prefix+"target_id", e.TargetID)
	a.text(prefix+"target_name", e.TargetName)
	a.text(prefix+"folder_id", e.FolderID)
	a.text(prefix+"dashboard_id", e.DashboardID)
	a.text(prefix+"observation_id", e.ObservationID)
	a.text(prefix+"thread_id", e.ThreadID)
	a.text(prefix+"chat_id", e.ChatID)
	a.text(prefix+"datasource_uid", e.DatasourceUID)
	if data := string(e.Data); data != "" && data != "{}" {
		a.text(prefix+"data", &data)
	}
	a.text(prefix+"server_id", &e.ServerID)
	a.text(prefix+"server_version", &e.ServerVersion)
	r.Attributes = a.list
	return r
}

func unixNano(occurredAt string) int64 {
	at, err := time.Parse(time.RFC3339Nano, occurredAt)
	if err != nil {
		return 0
	}
	return at.UnixNano()
}

func severityOf(outcome string) (int, string) {
	switch outcome {
	case "failed":
		return severityError, "ERROR"
	case "denied", "unknown":
		return severityWarn, "WARN"
	}
	return severityInfo, "INFO"
}

func validTraceID(id string) bool {
	if len(id) != 32 {
		return false
	}
	nonZero := false
	for _, c := range id {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
			nonZero = nonZero || c != '0'
		default:
			return false
		}
	}
	return nonZero
}

type attributes struct {
	list []attribute
}

func (a *attributes) text(key string, value *string) {
	if value != nil && *value != "" {
		a.list = append(a.list, text(key, *value))
	}
}

func (a *attributes) number(key string, value *int32) {
	if value != nil {
		a.integer(key, int64(*value))
	}
}

func (a *attributes) integer(key string, value int64) {
	n := strconv.FormatInt(value, 10)
	a.list = append(a.list, attribute{Key: key, Value: anyValue{Int: &n}})
}

func text(key, value string) attribute {
	return attribute{Key: key, Value: anyValue{String: &value}}
}
