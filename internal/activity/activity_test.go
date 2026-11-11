package activity

import "testing"

func TestTargetKindAndKnownForEveryType(t *testing.T) {
	tests := map[string]string{
		"dashboard.created":         "dashboard",
		"dashboard.version_created": "dashboard",
		"dashboard.restored":        "dashboard",
		"dashboard.moved":           "dashboard",
		"dashboard.deleted":         "dashboard",
		"annotation.created":        "annotation",
		"annotation.deleted":        "annotation",
		"finding.created":           "finding",
		"finding.pinned":            "finding",
		"thread.opened":             "thread",
		"thread.replied":            "thread",
		"thread.edited":             "thread",
		"thread.resolved":           "thread",
		"thread.reopened":           "thread",
		"thread.mentioned":          "thread",
		"run.queued":                "run",
		"run.started":               "run",
		"run.succeeded":             "run",
		"run.failed":                "run",
		"run.canceled":              "run",
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
