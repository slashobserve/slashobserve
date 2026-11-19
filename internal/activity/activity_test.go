package activity

import "testing"

func TestTargetKindAndKnownForEveryType(t *testing.T) {
	tests := map[string]string{
		"dashboard.created":           "dashboard",
		"dashboard.version_created":   "dashboard",
		"dashboard.restored":          "dashboard",
		"dashboard.moved":             "dashboard",
		"dashboard.deleted":           "dashboard",
		"annotation.created":          "annotation",
		"annotation.updated":          "annotation",
		"annotation.deleted":          "annotation",
		"finding.created":             "finding",
		"finding.pinned":              "finding",
		"finding.moved":               "finding",
		"saved_search.created":        "saved_search",
		"saved_search.updated":        "saved_search",
		"saved_search.moved":          "saved_search",
		"saved_search.deleted":        "saved_search",
		"observation.created":         "observation",
		"observation.updated":         "observation",
		"observation.moved":           "observation",
		"observation.resolved":        "observation",
		"observation.reopened":        "observation",
		"observation.published":       "observation",
		"observation.block_added":     "observation",
		"observation.block_updated":   "observation",
		"observation.block_kept":      "observation",
		"observation.block_discarded": "observation",
		"observation.block_removed":   "observation",
		"proposal.opened":             "proposal",
		"proposal.updated":            "proposal",
		"proposal.merged":             "proposal",
		"proposal.discarded":          "proposal",
		"thread.opened":               "thread",
		"thread.replied":              "thread",
		"thread.edited":               "thread",
		"thread.resolved":             "thread",
		"thread.reopened":             "thread",
		"thread.mentioned":            "thread",
		"run.queued":                  "run",
		"run.started":                 "run",
		"run.succeeded":               "run",
		"run.failed":                  "run",
		"run.canceled":                "run",
		"run.canceling":               "run",
		"run.released":                "run",
		"run.kept":                    "run",
		"public_link.created":         "public_link",
		"public_link.revoked":         "public_link",
		"user.role_changed":           "user",
		"user.signed_out_everywhere":  "user",
		"service_account.created":     "service_account",
		"service_account.updated":     "service_account",
		"service_account.key_created": "service_account",
		"service_account.key_revoked": "service_account",
		"webhook.created":             "webhook",
		"webhook.updated":             "webhook",
		"webhook.secret_rotated":      "webhook",
		"webhook.deleted":             "webhook",
		"mcp_client.connected":        "mcp_client",
		"mcp_client.disconnected":     "mcp_client",
		"outbound_domain.added":       "outbound_domain",
		"outbound_domain.removed":     "outbound_domain",
		"grafana_connection.created":  "grafana_connection",
		"grafana_connection.updated":  "grafana_connection",
		"grafana_connection.deleted":  "grafana_connection",
	}
	for typ, kind := range tests {
		if !Known(typ) {
			t.Errorf("Known(%q) = false", typ)
		}
		if got := targetKind(typ); got != kind {
			t.Errorf("targetKind(%q) = %q, want %q", typ, got, kind)
		}
	}
	if len(Types()) != len(tests) {
		t.Errorf("Types() has %d entries, want %d", len(Types()), len(tests))
	}
}

func TestKnownRefusesOtherTypes(t *testing.T) {
	for _, typ := range []string{"", "dashboard", "comment.created", "thread.created", "run.", ".queued", "Dashboard.created"} {
		if Known(typ) {
			t.Errorf("Known(%q) = true", typ)
		}
	}
}

func TestTypesReturnsACopy(t *testing.T) {
	got := Types()
	got[0] = "changed"
	if Types()[0] == "changed" {
		t.Fatal("Types exposes the package list")
	}
}
