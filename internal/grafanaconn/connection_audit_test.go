package grafanaconn_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTestingAGrafanaConnectionIsRecordedWithItsHostAndOutcome(t *testing.T) {
	h := newAuditedHTTPRig(t, newRig(t))
	id := h.connect(t).ID

	expect(t, h.call(t, http.MethodPost, "/api/v1/grafana/"+id+"/test", ""), http.StatusOK, "")
	h.grafana.token = "rotated-away"
	expect(t, h.call(t, http.MethodPost, "/api/v1/grafana/"+id+"/test", ""), http.StatusOK, "")
	h.flush()

	events := h.db.AuditEvents(t, "grafana_connection.test")
	if len(events) != 2 {
		t.Fatalf("events = %+v, want two", events)
	}
	healthy, broken := events[0], events[1]
	if healthy.Outcome != "success" || healthy.TargetKind != "grafana_connection" || healthy.TargetID != id || healthy.ActorKind != "user" || healthy.ActorID != h.admin.ID {
		t.Fatalf("healthy test = %+v", healthy)
	}
	if healthy.Data["host"] != grafanaHost || healthy.Data["healthy"] != true {
		t.Fatalf("healthy data = %v", healthy.Data)
	}
	if broken.Outcome != "failed" || broken.Reason != "unhealthy" || broken.Data["healthy"] != false || broken.TargetID != id {
		t.Fatalf("broken test = %+v", broken)
	}
	for _, event := range events {
		raw, _ := json.Marshal(event.Data)
		if strings.Contains(string(raw), grafanaToken) || strings.Contains(string(raw), "https://") {
			t.Fatalf("event data carries a secret or a url: %s", raw)
		}
	}
}

func TestFetchingADashboardFromALinkIsRecordedWithoutTheLinkOrTheBody(t *testing.T) {
	h := newAuditedHTTPRig(t, newRig(t))
	h.grafana.add(&fakeDashboard{uid: "checkout", title: "Checkout", folder: "f-k8s", model: simpleModel("graf-prom", "prometheus"), version: 3})
	id := h.connect(t).ID

	expect(t, h.call(t, http.MethodPost, "/api/v1/grafana/link", linkBody(grafanaURL+"/d/checkout/checkout?orgId=1&secret=link-secret")), http.StatusOK, "")
	expect(t, h.call(t, http.MethodPost, "/api/v1/grafana/link", linkBody(grafanaURL+"/d/missing")), http.StatusNotFound, "dashboard_not_found")
	h.flush()

	events := h.db.AuditEvents(t, "grafana_connection.fetch_dashboard")
	if len(events) != 2 {
		t.Fatalf("events = %+v, want two", events)
	}
	fetched, refused := events[0], events[1]
	if fetched.Outcome != "success" || fetched.TargetKind != "grafana_connection" || fetched.TargetID != id || fetched.ActorID != h.admin.ID || fetched.Data["host"] != grafanaHost {
		t.Fatalf("fetch = %+v", fetched)
	}
	if refused.Outcome != "failed" || refused.Reason != "dashboard_not_found" {
		t.Fatalf("missing dashboard = %+v", refused)
	}
	for _, event := range events {
		raw, _ := json.Marshal(event.Data)
		for _, leaked := range []string{"link-secret", "Checkout", "checkout", "https://"} {
			if strings.Contains(string(raw), leaked) {
				t.Fatalf("event data carries %q: %s", leaked, raw)
			}
		}
	}
}
