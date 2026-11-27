package findings_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func (r rig) activityFor(t *testing.T, by auth.Principal) []activity.Event {
	t.Helper()
	store := activity.NewStore(r.db.DB, folders.NewAccess(r.db.DB))
	audience, err := store.Audience(reading(t.Context()), by)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := store.Next(reading(t.Context()), audience, activity.Filter{}, activity.Position{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func typesOf(events []activity.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func dataOf(t *testing.T, e activity.Event) map[string]string {
	t.Helper()
	var data map[string]string
	if err := json.Unmarshal(e.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAFindingOnADashboardLandsInItsFolderWithItsTitleAndRun(t *testing.T) {
	r := newRig(t)
	request := r.investigation(t, editor, r.open)
	f := r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Title: "Why checkout is slow", Markdown: "a distinctive finding body"})

	got := findingEvents(r.activityFor(t, viewer))
	if len(got) != 1 {
		t.Fatalf("viewer sees %+v", got)
	}
	e := got[0]
	if e.Type != activity.TypeFindingCreated || e.TargetKind != "finding" || e.TargetID != f.ID || e.DashboardID != r.open.String() {
		t.Errorf("event = %+v", e)
	}
	var folder string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT folder_id::text FROM dashboards WHERE id = $1`, r.open).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	if e.FolderID != folder {
		t.Errorf("folder = %q, want %q", e.FolderID, folder)
	}
	if e.Actor.ID != editor.ID || e.Actor.RunID != request.String() {
		t.Errorf("actor = %+v", e.Actor)
	}
	want := map[string]string{"title": "Why checkout is slow", "agent_request_id": request.String()}
	if data := dataOf(t, e); !slices.Equal(sortedPairs(data), sortedPairs(want)) {
		t.Errorf("data = %s, want %v", e.Data, want)
	}
}

func sortedPairs(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

func TestAFindingAKeyFiledInGeneralIsSeenByEveryoneWhoSeesGeneral(t *testing.T) {
	r := newRig(t)
	f := r.save(t, ciKey, findings.SaveInput{Title: "Shared"})
	for _, by := range []auth.Principal{ciKey, admin, otherEditor, viewer} {
		got := findingEvents(r.activityFor(t, by))
		if len(got) != 1 || got[0].TargetID != f.ID || got[0].FolderID != r.general(t).String() || got[0].DashboardID != "" {
			t.Errorf("%s sees %+v", by.Name, got)
		}
		if data := dataOf(t, got[0]); len(data) != 1 || data["title"] != "Shared" {
			t.Errorf("%s: data = %s", by.Name, got[0].Data)
		}
	}
}

func TestAFindingEventCarriesTheObservationAndTheFindingColumns(t *testing.T) {
	r := newRig(t)
	own := r.save(t, ciKey, findings.SaveInput{})
	onDashboard := r.save(t, ciKey, findings.SaveInput{DashboardID: r.open})
	type columns struct{ observation, finding, dashboard string }
	read := func(target string) columns {
		var c columns
		err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT COALESCE(observation_id::text, ''), COALESCE(finding_id::text, ''), COALESCE(dashboard_id::text, '') FROM activity WHERE type = $1 AND target_id = $2::uuid`, activity.TypeFindingCreated, target).Scan(&c.observation, &c.finding, &c.dashboard)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if got, want := read(own.ID), (columns{observation: own.ObservationID, finding: own.ID}); got != want {
		t.Errorf("a finding without a dashboard: %+v, want %+v", got, want)
	}
	if got, want := read(onDashboard.ID), (columns{dashboard: r.open.String()}); got != want {
		t.Errorf("a finding on a dashboard stays dashboard-owned: %+v, want %+v", got, want)
	}
}

func findingEvents(events []activity.Event) []activity.Event {
	var out []activity.Event
	for _, e := range events {
		if e.TargetKind == "finding" {
			out = append(out, e)
		}
	}
	return out
}

func TestAFindingInMyDashboardsFollowsItsFolderAndIsSeenByItsCreatorOnly(t *testing.T) {
	r := newRig(t)
	f := r.save(t, editor, findings.SaveInput{Title: "Private"})
	personal := r.personalFolder(t, editor)
	cases := []struct {
		name    string
		by      auth.Principal
		visible bool
	}{
		{"the creator", editor, true},
		{"an admin", admin, false},
		{"another editor", otherEditor, false},
	}
	for _, c := range cases {
		got := findingEvents(r.activityFor(t, c.by))
		if !c.visible {
			if len(got) != 0 {
				t.Errorf("%s sees %+v", c.name, got)
			}
			continue
		}
		if len(got) != 1 || got[0].TargetID != f.ID || got[0].FolderID != personal.String() || got[0].DashboardID != "" {
			t.Errorf("%s sees %+v, want the finding in folder %s", c.name, got, personal)
		}
	}
}

func TestAReplayedFindingAppendsOnce(t *testing.T) {
	r := newRig(t)
	request := r.investigation(t, editor, r.open)
	r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open})
	_, replayed, err := r.service.Save(reading(t.Context()), editor, findings.SaveInput{
		AgentRequestID: request, DashboardID: r.open, Title: "again", Markdown: "second", From: windowTo.Add(-time.Hour), To: windowTo,
	})
	if err != nil || !replayed {
		t.Fatalf("second save: replayed %v, %v", replayed, err)
	}
	if got := typesOf(findingEvents(r.activityFor(t, admin))); !slices.Equal(got, []string{activity.TypeFindingCreated}) {
		t.Errorf("activity = %v", got)
	}
}

func TestAFindingOfARunKeptWithAdminsIsRecordedOnItsObservationNotOnTheDashboard(t *testing.T) {
	r := newRig(t)
	subject := r.openRunObservation(t, editor, r.request(t, editor, r.open))
	locked := r.folder(t, "Payroll")
	r.db.Exec(t, `UPDATE folders SET admins_only = true WHERE id = $1`, locked)
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, subject, locked)
	request := r.request(t, admin, r.open)
	r.db.Exec(t, `UPDATE agent_requests SET observation_id = $2 WHERE id = $1`, request, subject)
	title := "Outage caused by the vendor contract dispute"
	f := r.save(t, admin, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Title: title})

	if got := findingEvents(r.activityFor(t, viewer)); len(got) != 0 {
		t.Fatalf("a Viewer of the dashboard sees %+v", got)
	}
	got := findingEvents(r.activityFor(t, admin))
	if len(got) != 1 || got[0].TargetID != f.ID || got[0].FolderID != locked.String() || got[0].DashboardID != "" {
		t.Fatalf("an Admin sees %+v, want the finding in folder %s and on no dashboard", got, locked)
	}
	if data := dataOf(t, got[0]); data["title"] != title {
		t.Errorf("data = %s", got[0].Data)
	}
}
