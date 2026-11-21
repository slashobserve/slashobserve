package datasourceadmin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTestingADatasourceIsRecordedWithItsHostAndOutcome(t *testing.T) {
	server := upstream(t, nil)
	down := httptest.NewServer(http.NotFoundHandler())
	downAddress := down.URL
	down.Close()
	h := newAuditedHTTPRig(t)
	h.call(t, http.MethodPost, "/api/v1/datasources", createBody("added", server.URL))

	good := h.call(t, http.MethodPost, "/api/v1/datasources/test?base=added", `{"name":"Draft"}`)
	bad := h.call(t, http.MethodPost, "/api/v1/datasources/test", `{"uid":"draft","type":"prometheus","name":"Draft","address":"`+downAddress+`","secrets":{"bearer_token":"token-secret-9"}}`)
	h.flush()
	if good.Code != http.StatusOK || bad.Code != http.StatusOK {
		t.Fatalf("answered %d and %d: %s %s", good.Code, bad.Code, good.Body, bad.Body)
	}

	events := h.database.AuditEvents(t, "datasource.test")
	if len(events) != 2 {
		t.Fatalf("events = %+v, want two", events)
	}
	healthy, broken := events[0], events[1]
	if healthy.Outcome != "success" || healthy.TargetKind != "datasource" || healthy.TargetID != "added" || healthy.ActorKind != "user" || healthy.ActorID != h.admin.ID {
		t.Fatalf("healthy test = %+v", healthy)
	}
	if healthy.Data["type"] != "prometheus" || healthy.Data["healthy"] != true || healthy.Data["host"] == "" {
		t.Fatalf("healthy data = %v", healthy.Data)
	}
	if broken.Outcome != "failed" || broken.Reason != "unhealthy" || broken.TargetID != "draft" || broken.Data["healthy"] != false {
		t.Fatalf("broken test = %+v", broken)
	}
	for _, event := range events {
		raw, _ := json.Marshal(event.Data)
		for _, leaked := range []string{"token-secret-9", "token-1", "http://"} {
			if strings.Contains(string(raw), leaked) {
				t.Fatalf("event data carries %q: %s", leaked, raw)
			}
		}
	}
}
