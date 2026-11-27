package provision_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/provision"
)

func (r *rig) replacedVersions(t *testing.T, dashboardID string) map[int]int {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(reading(t.Context()), `
		SELECT (data->>'version')::int, coalesce((data->>'replaced_version')::int, 0)
		FROM activity WHERE dashboard_id = $1 AND type = 'dashboard.version_created' ORDER BY seq`, dashboardID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[int]int{}
	for rows.Next() {
		var version, replaced int
		if err := rows.Scan(&version, &replaced); err != nil {
			t.Fatal(err)
		}
		got[version] = replaced
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAFileChangeOverAPersonsEditRecordsTheVersionItReplaced(t *testing.T) {
	r := newApplyRig(t)
	r.write(t, "a.json", specJSON("A", "up"))
	dashboardID := entry(t, r.run(t), "a.json").DashboardID
	id, err := db.ParseUUID(dashboardID)
	if err != nil {
		t.Fatal(err)
	}

	edited, err := r.dashboards.Save(reading(t.Context()), r.editor, dashboards.SaveInput{DashboardID: id, Parent: 1, Spec: []byte(specJSON("A edited in the app", "up")), Summary: "by hand"})
	if err != nil {
		t.Fatalf("a person's save to a file dashboard: %v", err)
	}
	if edited.Version.Number != 2 || edited.Warning != folders.ManagedByFile.EditWarning() || edited.Dashboard.ManagedBy != folders.ManagedByFile {
		t.Fatalf("saved version %d with warning %q, managed by %s, want version 2, the file warning and managed by file", edited.Version.Number, edited.Warning, edited.Dashboard.ManagedBy)
	}

	r.write(t, "a.json", specJSON("A from the file", "up"))
	r.run(t)
	r.write(t, "a.json", specJSON("A from the file again", "up"))
	r.run(t)

	dash, ver := r.latest(t, dashboardID)
	if ver.Number != 4 || dash.Title != "A from the file again" {
		t.Fatalf("version %d, title %q, want the file's content at version 4", ver.Number, dash.Title)
	}
	got := r.replacedVersions(t, dashboardID)
	if len(got) != 3 || got[2] != 0 || got[3] != 2 || got[4] != 0 {
		t.Fatalf("replaced versions by saved version = %v, want only version 3 to name version 2", got)
	}
}

func TestAnApplyOverAPersonsEditRecordsTheVersionItReplaced(t *testing.T) {
	r := newApplyRig(t)
	created := r.apply(t, r.editor, "Shop/checkout.json", specJSON("Checkout", "up"))
	id, err := db.ParseUUID(created.DashboardID)
	if err != nil {
		t.Fatal(err)
	}

	edited, err := r.dashboards.Save(reading(t.Context()), r.editor, dashboards.SaveInput{DashboardID: id, Parent: 1, Spec: []byte(specJSON("Checkout edited in the app", "up")), Summary: "by hand"})
	if err != nil {
		t.Fatalf("a person's save to an applied dashboard: %v", err)
	}
	if edited.Version.Number != 2 || edited.Warning != folders.ManagedByApply.EditWarning() || edited.Dashboard.ManagedBy != folders.ManagedByApply {
		t.Fatalf("saved version %d with warning %q, managed by %s, want version 2, the apply warning and managed by apply", edited.Version.Number, edited.Warning, edited.Dashboard.ManagedBy)
	}

	if same := r.apply(t, r.editor, "Shop/checkout.json", specJSON("Checkout", "up")); same.Outcome != provision.OutcomeUnchanged || same.Version != 2 {
		t.Fatalf("applying the unchanged file = %+v, want it to leave the edit at version 2", same)
	}
	for i, title := range []string{"Checkout from the file", "Checkout from the file again"} {
		if updated := r.apply(t, r.editor, "Shop/checkout.json", specJSON(title, "up")); updated.Outcome != provision.OutcomeUpdated || updated.Version != 3+i {
			t.Fatalf("applying the changed file = %+v, want version %d", updated, 3+i)
		}
	}

	got := r.replacedVersions(t, created.DashboardID)
	if len(got) != 3 || got[2] != 0 || got[3] != 2 || got[4] != 0 {
		t.Fatalf("replaced versions by saved version = %v, want only version 3 to name version 2", got)
	}
}
