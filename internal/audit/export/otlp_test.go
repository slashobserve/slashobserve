package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
)

func ptr[T any](v T) *T { return &v }

func fullEvent() trail.Event {
	return trail.Event{
		ID: "0198f2c4-0000-7000-8000-000000000001", Seq: 42, OccurredAt: "2026-11-21T10:30:15.123456789Z",
		Category: "content", Action: "dashboard.update", Outcome: "success",
		Status: ptr(int32(200)), DurationMs: ptr(int32(12)),
		ActorKind: "user", ActorID: ptr("0198f2c4-0000-7000-8000-0000000000aa"), ActorName: ptr("Ada"), ActorRole: ptr("editor"),
		Initiator: "agent", AgentID: ptr("0198f2c4-0000-7000-8000-0000000000bb"), AgentName: ptr("Scout"),
		Origin: "api", ClientIP: ptr("203.0.113.9"), UserAgent: ptr("curl/8.4"),
		RequestID: ptr("0198f2c4-0000-7000-8000-0000000000cc"), TraceID: ptr("4bf92f3577b34da6a3ce929d0e0e4736"),
		HTTPMethod: ptr("PUT"), Route: ptr("/api/v1/dashboards/{id}"),
		TargetKind: ptr("dashboard"), TargetID: ptr("d1"), TargetName: ptr("Checkout"),
		Data: json.RawMessage(`{"version":3}`), ServerID: "server-a", ServerVersion: "1.2.3",
	}
}

type decoded struct {
	ResourceLogs []struct {
		Resource struct {
			Attributes []map[string]any `json:"attributes"`
		} `json:"resource"`
		ScopeLogs []struct {
			Scope      map[string]any `json:"scope"`
			LogRecords []struct {
				TimeUnixNano   string           `json:"timeUnixNano"`
				SeverityNumber int              `json:"severityNumber"`
				SeverityText   string           `json:"severityText"`
				Body           map[string]any   `json:"body"`
				Attributes     []map[string]any `json:"attributes"`
				TraceID        string           `json:"traceId"`
				EventName      string           `json:"eventName"`
			} `json:"logRecords"`
		} `json:"scopeLogs"`
	} `json:"resourceLogs"`
}

func decode(t *testing.T, body []byte) decoded {
	t.Helper()
	var d decoded
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("the payload is not JSON: %v", err)
	}
	if len(d.ResourceLogs) != 1 || len(d.ResourceLogs[0].ScopeLogs) != 1 {
		t.Fatalf("payload has %d resources", len(d.ResourceLogs))
	}
	return d
}

func attrs(t *testing.T, list []map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, a := range list {
		key := a["key"].(string)
		if _, dup := out[key]; dup {
			t.Fatalf("attribute %q appears twice", key)
		}
		out[key] = a["value"].(map[string]any)
	}
	return out
}

func TestEncodeMapsAnEventToAnOTLPLogRecord(t *testing.T) {
	e := fullEvent()
	body, err := Encode([]trail.Event{e})
	if err != nil {
		t.Fatal(err)
	}
	d := decode(t, body)
	if got := attrs(t, d.ResourceLogs[0].Resource.Attributes)["service.name"]["stringValue"]; got != "slashobserve" {
		t.Errorf("service.name = %v", got)
	}
	if got := d.ResourceLogs[0].ScopeLogs[0].Scope["name"]; got != "slashobserve.audit" {
		t.Errorf("scope name = %v", got)
	}
	records := d.ResourceLogs[0].ScopeLogs[0].LogRecords
	if len(records) != 1 {
		t.Fatalf("%d records", len(records))
	}
	r := records[0]
	if r.TimeUnixNano != "1795257015123456789" {
		t.Errorf("timeUnixNano = %s", r.TimeUnixNano)
	}
	if r.SeverityNumber != 9 || r.SeverityText != "INFO" {
		t.Errorf("severity = %d %s", r.SeverityNumber, r.SeverityText)
	}
	if r.EventName != "dashboard.update" {
		t.Errorf("eventName = %q", r.EventName)
	}
	if r.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("traceId = %q", r.TraceID)
	}
	if r.Body["stringValue"] != auditsource.Line(e) {
		t.Errorf("body = %v, want the rendered line %q", r.Body["stringValue"], auditsource.Line(e))
	}

	got := attrs(t, r.Attributes)
	text := map[string]string{
		"event.name": "dashboard.update", "user.id": "0198f2c4-0000-7000-8000-0000000000aa", "client.address": "203.0.113.9",
		"user_agent.original": "curl/8.4", "http.route": "/api/v1/dashboards/{id}", "http.request.method": "PUT",
		"slashobserve.audit.id": e.ID, "slashobserve.audit.category": "content", "slashobserve.audit.outcome": "success",
		"slashobserve.audit.actor_kind": "user", "slashobserve.audit.actor_name": "Ada", "slashobserve.audit.actor_role": "editor",
		"slashobserve.audit.initiator": "agent", "slashobserve.audit.agent_id": "0198f2c4-0000-7000-8000-0000000000bb",
		"slashobserve.audit.agent_name": "Scout", "slashobserve.audit.origin": "api",
		"slashobserve.audit.request_id":  "0198f2c4-0000-7000-8000-0000000000cc",
		"slashobserve.audit.trace_id":    "4bf92f3577b34da6a3ce929d0e0e4736",
		"slashobserve.audit.target_kind": "dashboard", "slashobserve.audit.target_id": "d1", "slashobserve.audit.target_name": "Checkout",
		"slashobserve.audit.data": `{"version":3}`, "slashobserve.audit.server_id": "server-a", "slashobserve.audit.server_version": "1.2.3",
	}
	for key, want := range text {
		if v, ok := got[key]; !ok || v["stringValue"] != want {
			t.Errorf("attribute %s = %v, want %q", key, got[key], want)
		}
	}
	numbers := map[string]string{"http.response.status_code": "200", "slashobserve.audit.duration_ms": "12", "slashobserve.audit.seq": "42"}
	for key, want := range numbers {
		if v, ok := got[key]; !ok || v["intValue"] != want {
			t.Errorf("attribute %s = %v, want int %s", key, got[key], want)
		}
	}
	for _, absent := range []string{"slashobserve.audit.action", "slashobserve.audit.reason", "slashobserve.audit.run_id", "slashobserve.audit.tool"} {
		if _, ok := got[absent]; ok {
			t.Errorf("attribute %s is set for an event without it", absent)
		}
	}
	for key := range got {
		if strings.HasPrefix(key, "slashobserve.") && !strings.HasPrefix(key, "slashobserve.audit.") {
			t.Errorf("attribute %s is outside slashobserve.audit.*", key)
		}
	}
}

func TestEncodeKeepsTheOrderOfTheEvents(t *testing.T) {
	var events []trail.Event
	for _, action := range []string{"a.one", "b.two", "c.three"} {
		e := fullEvent()
		e.Action = action
		events = append(events, e)
	}
	body, err := Encode(events)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range decode(t, body).ResourceLogs[0].ScopeLogs[0].LogRecords {
		names = append(names, r.EventName)
	}
	if strings.Join(names, ",") != "a.one,b.two,c.three" {
		t.Fatalf("order = %v", names)
	}
}

func TestEncodeLeavesOutWhatTheEventLacks(t *testing.T) {
	e := trail.Event{
		ID: "id", Seq: 1, OccurredAt: "2026-11-21T10:30:15Z", Category: "auth", Action: "auth.login", Outcome: "denied",
		ActorKind: "anonymous", Initiator: "person", Origin: "web", ServerID: "s", ServerVersion: "v", Data: json.RawMessage(`{}`),
		TraceID: ptr("not-a-trace-id"),
	}
	body, err := Encode([]trail.Event{e})
	if err != nil {
		t.Fatal(err)
	}
	r := decode(t, body).ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	if r.SeverityNumber != 13 || r.SeverityText != "WARN" {
		t.Errorf("a denied event has severity %d %s", r.SeverityNumber, r.SeverityText)
	}
	if r.TraceID != "" {
		t.Errorf("traceId = %q for a value that is not a trace id", r.TraceID)
	}
	got := attrs(t, r.Attributes)
	for _, absent := range []string{"user.id", "client.address", "user_agent.original", "http.route", "http.response.status_code", "slashobserve.audit.data"} {
		if _, ok := got[absent]; ok {
			t.Errorf("attribute %s is set for an event without it", absent)
		}
	}
	if got["slashobserve.audit.trace_id"]["stringValue"] != "not-a-trace-id" {
		t.Error("the stored trace id text is not kept as an attribute")
	}
}

func TestSeverityFollowsTheOutcome(t *testing.T) {
	for outcome, want := range map[string]int{"success": 9, "denied": 13, "unknown": 13, "failed": 17} {
		if got, _ := severityOf(outcome); got != want {
			t.Errorf("%s: severity %d, want %d", outcome, got, want)
		}
	}
}

func TestValidTraceID(t *testing.T) {
	for id, want := range map[string]bool{
		"4bf92f3577b34da6a3ce929d0e0e4736":  true,
		"00000000000000000000000000000000":  false,
		"4BF92F3577B34DA6A3CE929D0E0E4736":  false,
		"4bf92f3577b34da6a3ce929d0e0e473":   false,
		"4bf92f3577b34da6a3ce929d0e0e47366": false,
		"":                                  false,
	} {
		if got := validTraceID(id); got != want {
			t.Errorf("validTraceID(%q) = %v", id, got)
		}
	}
}
