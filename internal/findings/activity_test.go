package findings_test

import (
	"encoding/json"
	"slices"
	"strings"
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
	audience, err := store.Audience(t.Context(), by)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := store.Next(t.Context(), audience, activity.Filter{}, activity.Position{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func withoutObservationEvents(events []activity.Event) []activity.Event {
	var out []activity.Event
	for _, e := range events {
		if !strings.HasPrefix(e.Type, "observation.") {
			out = append(out, e)
		}
	}
	return out
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
	request := r.request(t, editor, r.open)
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
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT folder_id::text FROM dashboards WHERE id = $1`, r.open).Scan(&folder); err != nil {
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
		err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT COALESCE(observation_id::text, ''), COALESCE(finding_id::text, ''), COALESCE(dashboard_id::text, '') FROM activity WHERE type = $1 AND target_id = $2::uuid`, activity.TypeFindingCreated, target).Scan(&c.observation, &c.finding, &c.dashboard)
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
	request := r.request(t, editor, r.open)
	r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open})
	_, replayed, err := r.service.Save(t.Context(), editor, findings.SaveInput{
		AgentRequestID: request, DashboardID: r.open, Title: "again", Markdown: "second", From: windowTo.Add(-time.Hour), To: windowTo,
	})
	if err != nil || !replayed {
		t.Fatalf("second save: replayed %v, %v", replayed, err)
	}
	if got := typesOf(findingEvents(r.activityFor(t, admin))); !slices.Equal(got, []string{activity.TypeFindingCreated}) {
		t.Errorf("activity = %v", got)
	}
}

func TestAPinAppendsTheAnnotationAndThePin(t *testing.T) {
	r := newRig(t)
	f := r.save(t, editor, findings.SaveInput{DashboardID: r.open})
	a, created, err := r.service.Pin(t.Context(), editor, mustUUID(t, f.ID))
	if err != nil || !created {
		t.Fatalf("pin: created %v, %v", created, err)
	}
	if _, created, err := r.service.Pin(t.Context(), editor, mustUUID(t, f.ID)); err != nil || created {
		t.Fatalf("second pin: created %v, %v", created, err)
	}

	got := withoutObservationEvents(r.activityFor(t, viewer))
	want := []string{activity.TypeFindingCreated, activity.TypeAnnotationCreated, activity.TypeFindingPinned}
	if !slices.Equal(typesOf(got), want) {
		t.Fatalf("activity = %v, want %v", typesOf(got), want)
	}
	pinned := got[2]
	if pinned.TargetID != f.ID || pinned.DashboardID != r.open.String() || pinned.FolderID != got[0].FolderID || pinned.Actor.ID != editor.ID {
		t.Errorf("pin event = %+v", pinned)
	}
	if data := dataOf(t, pinned); len(data) != 1 || data["annotation_id"] != a.ID {
		t.Errorf("pin data = %s, want annotation %s", pinned.Data, a.ID)
	}
}

func TestALostPinRaceCleansUpWithoutPinningTwice(t *testing.T) {
	r := newRig(t)
	f := r.save(t, editor, findings.SaveInput{DashboardID: r.open})
	id := mustUUID(t, f.ID)
	var winner string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO annotations (dashboard_id, time_from, text, created_by) VALUES ($1, now(), 'the winner', $2::uuid) RETURNING id::text`, r.open, otherEditor.ID).Scan(&winner); err != nil {
		t.Fatal(err)
	}
	holder, err := r.db.SQLDB(t).BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.ExecContext(t.Context(), `UPDATE findings SET annotation_id = $1 WHERE id = $2`, winner, id); err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		annotationID string
		created      bool
		err          error
	}
	done := make(chan outcome, 1)
	go func() {
		a, created, err := r.service.Pin(t.Context(), editor, id)
		done <- outcome{a.ID, created, err}
	}()
	waitForALockWait(t, r)
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}

	select {
	case o := <-done:
		if o.err != nil || o.created || o.annotationID != winner {
			t.Fatalf("pin = %+v, want the winner's annotation %s and created false", o, winner)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("pin did not finish")
	}
	got := withoutObservationEvents(r.activityFor(t, admin))
	want := []string{activity.TypeFindingCreated, activity.TypeAnnotationCreated, activity.TypeAnnotationDeleted}
	if !slices.Equal(typesOf(got), want) {
		t.Fatalf("activity = %v, want %v", typesOf(got), want)
	}
	if got[1].TargetID != got[2].TargetID || got[1].TargetID == winner {
		t.Errorf("created %s, deleted %s, winner %s: the cleanup must remove the annotation the losing pin made", got[1].TargetID, got[2].TargetID, winner)
	}
}

func waitForALockWait(t *testing.T, r rig) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the pin never waited on the finding row")
}
