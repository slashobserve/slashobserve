package health_test

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/health/healthrule"
	"github.com/slashobserve/slashobserve/internal/repairs"
)

func TestRunRecordsEachDashboardsProblemsAndListShowsOnlyUnhealthyOnes(t *testing.T) {
	r := newRig(t)
	healthy := r.create(t, boardSpec("Healthy", "up"))
	empty := r.create(t, boardSpec("Empty", "up", "empty_series"))
	failing := r.create(t, boardSpec("Failing", "fail_series"))
	variable := r.create(t, variableSpec("Variable"))
	viewer := r.create(t, boardSpec("Per viewer", `up{user="${__user.login}"}`, "empty_series"))
	r.broken.Store(true)

	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	items := r.list(t, r.editor, "")
	for _, id := range []pgtype.UUID{healthy, viewer} {
		if _, listed := items[id.String()]; listed {
			t.Errorf("dashboard %s without problems is listed", id.String())
		}
	}
	want := map[pgtype.UUID][]health.Kind{
		empty:    {health.KindNoData},
		failing:  {health.KindQueryFailed},
		variable: {health.KindVariableFailed},
	}
	for id, kindsWanted := range want {
		it, ok := items[id.String()]
		if !ok {
			t.Errorf("dashboard %s is not listed", id.String())
			continue
		}
		if !slices.Equal(kinds(it), kindsWanted) {
			t.Errorf("%s problems = %+v, want kinds %v", it.Title, it.Problems, kindsWanted)
		}
		if it.CheckedVersion == nil || *it.CheckedVersion != 1 || it.CheckedAt == nil || !it.CanEdit {
			t.Errorf("%s checked version %v at %v, can edit %v", it.Title, it.CheckedVersion, it.CheckedAt, it.CanEdit)
		}
	}
	if p := items[empty.String()].Problems[0]; p.PanelID != "p1" || p.PanelTitle != "Panel 1" || p.Code != "query_empty" {
		t.Errorf("no data problem = %+v", p)
	}
	if p := items[variable.String()].Problems[0]; p.Variable != "pod" {
		t.Errorf("variable problem = %+v", p)
	}

	var skipped string
	r.scan(t, `SELECT skipped FROM dashboard_health WHERE dashboard_id = $1`, viewer.String(), &skipped)
	if skipped != health.SkippedDependsOnViewer {
		t.Errorf("per viewer dashboard skipped = %q", skipped)
	}
}

func TestRunChecksADashboardAgainOnlyAfterADay(t *testing.T) {
	r := newRig(t)
	id := r.create(t, boardSpec("Board", "up"))
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkedAt := func() time.Time {
		var at time.Time
		r.scan(t, `SELECT checked_at FROM dashboard_health WHERE dashboard_id = $1`, id.String(), &at)
		return at
	}
	first := checkedAt()
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if again := checkedAt(); !again.Equal(first) {
		t.Errorf("a second run rechecked the dashboard: %v then %v", first, again)
	}
	r.db.Exec(t, `UPDATE dashboard_health SET checked_at = now() - interval '24 hours' WHERE dashboard_id = $1`, id.String())
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if again := checkedAt(); !again.After(first) {
		t.Errorf("a day old check was not run again: %v", again)
	}
}

func TestRunChecksAnHourOldDashboardAgainOnlyWhenItFailedOrIsOutdated(t *testing.T) {
	r := newRig(t)
	healthy := r.create(t, boardSpec("Healthy", "up"))
	failing := r.create(t, boardSpec("Failing", "fail_series"))
	variable := r.create(t, variableSpec("Variable"))
	empty := r.create(t, boardSpec("Empty", "empty_series"))
	outdated := r.create(t, boardSpec("Outdated", "up"))
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err := r.dashboards.Save(t.Context(), r.editor, dashboards.SaveInput{
		DashboardID: outdated, Parent: 1, Spec: boardSpec("Outdated", "up", "up"), Summary: "edit",
	})
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE dashboard_health SET checked_at = now() - interval '1 hour'`)
	checkedAt := func(id pgtype.UUID) time.Time {
		var at time.Time
		r.scan(t, `SELECT checked_at FROM dashboard_health WHERE dashboard_id = $1`, id.String(), &at)
		return at
	}
	before := map[pgtype.UUID]time.Time{}
	for _, id := range []pgtype.UUID{healthy, failing, variable, empty, outdated} {
		before[id] = checkedAt(id)
	}

	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	want := map[pgtype.UUID]bool{healthy: false, failing: true, variable: true, empty: false, outdated: true}
	for id, rechecked := range want {
		if got := checkedAt(id).After(before[id]); got != rechecked {
			t.Errorf("dashboard %s rechecked = %v, want %v", id.String(), got, rechecked)
		}
	}
	var version int
	r.scan(t, `SELECT version FROM dashboard_health WHERE dashboard_id = $1`, outdated.String(), &version)
	if version != 2 {
		t.Errorf("outdated dashboard was checked at version %d, want 2", version)
	}
}

func TestRunChecksAFailingDashboardOnceInOneRun(t *testing.T) {
	r := newRig(t)
	failing := r.create(t, boardSpec("Failing", "fail_series"))
	r.broken.Store(true)
	r.queries.Store(0)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	queriesPerCheck := r.queries.Load()
	r.db.Exec(t, `UPDATE dashboard_health SET checked_at = now() - interval '1 hour' WHERE dashboard_id = $1`, failing.String())

	r.queries.Store(0)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.queries.Load(); got != queriesPerCheck {
		t.Errorf("one run ran %d queries for the failing dashboard, want %d for one check", got, queriesPerCheck)
	}
}

func TestListFindsDuplicateTitlesAndUnopenedDashboardsAndFiltersByKind(t *testing.T) {
	r := newRig(t)
	first := r.create(t, boardSpec("Checkout", "up"))
	second := r.create(t, boardSpec("checkout", "up"))
	old := r.create(t, boardSpec("Old", "up"))
	opened := r.create(t, boardSpec("Opened", "up"))
	r.db.Exec(t, `UPDATE dashboards SET created_at = now() - interval '100 days' WHERE id IN ($1, $2)`, old.String(), opened.String())
	if err := r.svc.RecordView(t.Context(), r.viewer, opened); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.RecordView(t.Context(), r.key, old); err != nil {
		t.Fatal(err)
	}

	items := r.list(t, r.editor, "")
	for _, id := range []pgtype.UUID{first, second} {
		if got := kinds(items[id.String()]); !slices.Equal(got, []health.Kind{health.KindDuplicateTitle}) {
			t.Errorf("%s kinds = %v, want duplicate_title", id.String(), got)
		}
	}
	if got := kinds(items[old.String()]); !slices.Equal(got, []health.Kind{health.KindUnopened}) {
		t.Errorf("old kinds = %v, want unopened (a key's open does not count)", got)
	}
	if _, listed := items[opened.String()]; listed {
		t.Error("a dashboard opened today is listed")
	}
	if it := items[old.String()]; it.LastViewedAt != nil {
		t.Errorf("a key's open was recorded at %v", it.LastViewedAt)
	}

	unopened := r.list(t, r.editor, health.KindUnopened)
	if len(unopened) != 1 || unopened[old.String()].ID == "" {
		t.Errorf("kind unopened lists %d dashboards", len(unopened))
	}
	if noData := r.list(t, r.editor, health.KindNoData); len(noData) != 0 {
		t.Errorf("kind no_data lists %d dashboards", len(noData))
	}
}

func TestListPagesWithACursor(t *testing.T) {
	r := newRig(t)
	for range 3 {
		r.create(t, boardSpec("Same", "up"))
	}
	seen := map[string]bool{}
	cursor := ""
	for range 3 {
		page, err := r.svc.List(t.Context(), r.editor, health.ListParams{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range page.Items {
			seen[it.ID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 3 || cursor != "" {
		t.Errorf("paging saw %d dashboards, last cursor %q", len(seen), cursor)
	}
	if _, err := r.svc.List(t.Context(), r.editor, health.ListParams{Limit: 2, Cursor: "not a cursor"}); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}

const day = 24 * time.Hour

func (r *rig) ordered(t *testing.T, by auth.Principal, p health.ListParams) []string {
	t.Helper()
	page, err := r.svc.List(t.Context(), by, p)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, it.Title)
	}
	return out
}

func (r *rig) rankedFixture(t *testing.T) []string {
	t.Helper()
	variable := r.create(t, variableSpec("Variable fails"))
	r.create(t, boardSpec("Query fails", "fail_series"))
	noData := r.create(t, boardSpec("No data", "empty_series"))
	twinOpened := r.create(t, boardSpec("Twin", "up"))
	r.create(t, boardSpec("twin", "up"))
	staleOpened := r.create(t, boardSpec("Stale opened", "up"))
	staleB := r.create(t, boardSpec("Stale B", "up"))
	staleA := r.create(t, boardSpec("Stale A", "up"))
	r.create(t, boardSpec("Healthy", "up"))
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	r.openedAgo(t, variable, 30*day)
	r.openedAgo(t, noData, 0)
	r.openedAgo(t, twinOpened, time.Hour)
	for _, id := range []pgtype.UUID{staleOpened, staleB, staleA} {
		r.createdAgo(t, id, 100*day)
	}
	r.openedAgo(t, staleOpened, 95*day)
	return []string{"Variable fails", "Query fails", "No data", "Twin", "twin", "Stale opened", "Stale A", "Stale B"}
}

func TestListRanksFailingThenNoDataThenDuplicatesThenUnopenedAndRecentlyOpenedFirst(t *testing.T) {
	r := newRig(t)
	want := r.rankedFixture(t)
	if got := r.ordered(t, r.editor, health.ListParams{Limit: 100}); !slices.Equal(got, want) {
		t.Errorf("order = %q\nwant %q", got, want)
	}
}

func TestListPagesThroughTheRankOrderWithoutRepeats(t *testing.T) {
	r := newRig(t)
	want := r.rankedFixture(t)
	for _, limit := range []int{1, 2, 3} {
		var got []string
		cursor := ""
		for range len(want) + 1 {
			page, err := r.svc.List(t.Context(), r.editor, health.ListParams{Limit: limit, Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range page.Items {
				got = append(got, it.Title)
			}
			if cursor = page.NextCursor; cursor == "" {
				break
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("limit %d pages = %q\nwant %q", limit, got, want)
		}
	}
}

func TestListByKindKeepsEachDashboardsRank(t *testing.T) {
	r := newRig(t)
	r.create(t, boardSpec("Fails and empty", "fail_series", "empty_series"))
	never := r.create(t, boardSpec("Empty never opened", "empty_series"))
	opened := r.create(t, boardSpec("Empty opened", "empty_series"))
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	r.openedAgo(t, opened, time.Hour)
	want := []string{"Fails and empty", "Empty opened", "Empty never opened"}
	if got := r.ordered(t, r.editor, health.ListParams{Kind: health.KindNoData, Limit: 100}); !slices.Equal(got, want) {
		t.Errorf("no_data order = %q\nwant %q", got, want)
	}
	if it := r.list(t, r.editor, health.KindNoData)[never.String()]; !slices.Equal(kinds(it), []health.Kind{health.KindNoData}) {
		t.Errorf("no_data list kinds = %v", kinds(it))
	}
}

func TestListAcceptsEveryKind(t *testing.T) {
	r := newRig(t)
	r.rankedFixture(t)
	for _, kind := range healthrule.Kinds {
		items := r.list(t, r.editor, kind)
		if len(items) == 0 {
			t.Errorf("kind %s lists nothing", kind)
		}
		for _, it := range items {
			if !slices.Contains(kinds(it), kind) {
				t.Errorf("kind %s lists %s with %v", kind, it.Title, kinds(it))
			}
		}
	}
}

func TestListSummaryCountsDashboardsPerKindOnTheFirstPage(t *testing.T) {
	r := newRig(t)
	r.create(t, boardSpec("Two empty panels", "empty_series", "empty_series"))
	r.create(t, variableSpec("Fails twice", "fail_series"))
	r.create(t, boardSpec("Fails", "fail_series"))
	r.create(t, boardSpec("Twin", "up"))
	r.create(t, boardSpec("twin", "up"))
	stale := r.create(t, boardSpec("Stale", "up"))
	r.create(t, boardSpec("Healthy", "up"))
	other := r.folder(t, "Other")
	r.createIn(t, other, boardSpec("Elsewhere", "empty_series"))
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	r.createdAgo(t, stale, 100*day)

	all := health.Summary{Dashboards: 7, Failing: 2, QueryFailed: 2, VariableFailed: 1, NoData: 2, DuplicateTitles: 2, Unopened: 1}
	for _, kind := range []health.Kind{"", health.KindNoData, health.KindDuplicateTitle} {
		page, err := r.svc.List(t.Context(), r.editor, health.ListParams{Kind: kind, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if page.Summary == nil || *page.Summary != all {
			t.Errorf("kind %q summary = %+v, want %+v", kind, page.Summary, all)
		}
		if page.NextCursor == "" {
			t.Fatalf("kind %q has one page", kind)
		}
		next, err := r.svc.List(t.Context(), r.editor, health.ListParams{Kind: kind, Limit: 1, Cursor: page.NextCursor})
		if err != nil {
			t.Fatal(err)
		}
		if next.Summary != nil {
			t.Errorf("kind %q later page summary = %+v, want nil", kind, next.Summary)
		}
	}

	page, err := r.svc.List(t.Context(), r.editor, health.ListParams{FolderID: other, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if want := (health.Summary{Dashboards: 1, NoData: 1}); page.Summary == nil || *page.Summary != want {
		t.Errorf("folder summary = %+v, want %+v", page.Summary, want)
	}
}

func TestListShowsEachDashboardsLatestHealthRepair(t *testing.T) {
	r := newRig(t)
	queued := r.create(t, boardSpec("Queued", "empty_series"))
	succeeded := r.create(t, boardSpec("Succeeded", "empty_series"))
	failed := r.create(t, boardSpec("Failed", "empty_series"))
	silent := r.create(t, boardSpec("Failed silently", "empty_series"))
	twice := r.create(t, boardSpec("Twice", "empty_series"))
	chat := r.create(t, boardSpec("Chat", "empty_series"))
	never := r.create(t, boardSpec("Never", "empty_series"))
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	out, err := r.repairs.Repair(t.Context(), r.editor, []pgtype.UUID{queued})
	if err != nil || len(out.Queued) != 1 {
		t.Fatalf("queue a repair: %+v, %v", out, err)
	}
	var prompt string
	r.scan(t, `SELECT prompt FROM agent_requests WHERE id = $1`, out.Queued[0].RequestID, &prompt)
	three, two := 3, 2
	gaveUp, old, empty := "the agent could not make the panel return data", "an older failure", ""
	succeededID := r.request(t, succeeded, request{prompt: prompt, status: "succeeded", resultVersion: &three, ago: time.Hour})
	r.request(t, failed, request{prompt: prompt, status: "failed", failure: &gaveUp, ago: time.Hour})
	r.request(t, silent, request{prompt: prompt, status: "failed", failure: &empty, ago: time.Hour})
	r.request(t, twice, request{prompt: prompt, status: "failed", failure: &old, ago: 2 * time.Hour})
	twiceID := r.request(t, twice, request{prompt: prompt, status: "succeeded", resultVersion: &two, ago: time.Hour})
	r.request(t, twice, request{prompt: "Make every panel blue", status: "queued", ago: time.Minute})
	r.request(t, chat, request{prompt: "Make every panel blue", status: "succeeded", resultVersion: &two, ago: time.Minute})
	r.request(t, chat, request{prompt: repairs.Prompt, status: "queued", ago: time.Second})

	items := r.list(t, r.editor, "")
	check := func(id pgtype.UUID, state, requestID string, version *int, failure *string) {
		t.Helper()
		got := items[id.String()].Repair
		if got == nil {
			t.Fatalf("%s has no repair", id.String())
		}
		if got.State != state || (requestID != "" && got.RequestID != requestID) || got.UpdatedAt.IsZero() {
			t.Errorf("%s repair = %+v, want state %s request %s", id.String(), got, state, requestID)
		}
		if !equalPtr(got.Version, version) || !equalPtr(got.Error, failure) {
			t.Errorf("%s repair version %v error %v, want %v %v", id.String(), got.Version, got.Error, version, failure)
		}
	}
	check(queued, "queued", out.Queued[0].RequestID, nil, nil)
	check(succeeded, "succeeded", succeededID, &three, nil)
	check(failed, "failed", "", nil, &gaveUp)
	check(silent, "failed", "", nil, nil)
	check(twice, "succeeded", twiceID, &two, nil)
	for _, id := range []pgtype.UUID{chat, never} {
		if got := items[id.String()].Repair; got != nil {
			t.Errorf("%s repair = %+v, want none", id.String(), got)
		}
	}

	var finished time.Time
	r.scan(t, `SELECT finished_at FROM agent_requests WHERE id = $1`, succeededID, &finished)
	if got := items[succeeded.String()].Repair.UpdatedAt; !got.Equal(finished) {
		t.Errorf("succeeded repair updated at %v, want its finish %v", got, finished)
	}
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (r *rig) moveToPersonalFolderOf(t *testing.T, id pgtype.UUID, owner auth.Principal) {
	t.Helper()
	r.db.Exec(t, `UPDATE dashboards SET folder_id = (SELECT id FROM folders WHERE owner_id = $2) WHERE id = $1`, id.String(), owner.ID)
}

func (r *rig) dashboardIDsFixture(t *testing.T) (empty, failing, healthy, hidden pgtype.UUID) {
	t.Helper()
	empty = r.create(t, boardSpec("Empty", "empty_series"))
	failing = r.create(t, boardSpec("Failing", "fail_series"))
	healthy = r.create(t, boardSpec("Healthy", "up"))
	hidden = r.create(t, boardSpec("Hidden", "empty_series"))
	r.moveToPersonalFolderOf(t, hidden, r.viewer)
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	return empty, failing, healthy, hidden
}

func titles(items []health.Item) []string {
	got := make([]string, 0, len(items))
	for _, it := range items {
		got = append(got, it.Title)
	}
	return got
}

func TestListWithDashboardIDsReadsEveryVisibleRequestedDashboardHealthyOnesIncluded(t *testing.T) {
	r := newRig(t)
	empty, failing, healthy, hidden := r.dashboardIDsFixture(t)

	page, err := r.svc.List(t.Context(), r.editor, health.ListParams{DashboardIDs: []pgtype.UUID{empty, failing, healthy, hidden}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Failing", "Empty", "Healthy"}; !slices.Equal(titles(page.Items), want) {
		t.Errorf("listed %v, want %v", titles(page.Items), want)
	}
	if page.Summary != nil {
		t.Errorf("summary = %+v, want nil", page.Summary)
	}
	ok := page.Items[2]
	if len(ok.Problems) != 0 || ok.State != health.StateHealthy || ok.CheckedAt == nil || ok.CheckedVersion == nil || *ok.CheckedVersion != 1 || ok.Skipped != "" || ok.Repair != nil {
		t.Errorf("healthy dashboard = %+v", ok)
	}

	owner, err := r.svc.List(t.Context(), r.viewer, health.ListParams{DashboardIDs: []pgtype.UUID{hidden}, Limit: 100})
	if err != nil || len(owner.Items) != 1 || owner.Items[0].Title != "Hidden" {
		t.Errorf("owner list = %+v, %v", owner.Items, err)
	}
	stranger, err := r.svc.List(t.Context(), r.editor, health.ListParams{DashboardIDs: []pgtype.UUID{hidden}, Limit: 100})
	if err != nil || len(stranger.Items) != 0 {
		t.Errorf("a dashboard in another user's personal folder listed %+v, %v", stranger.Items, err)
	}
}

func TestListWithDashboardIDsReadsASkippedAndANeverCheckedDashboard(t *testing.T) {
	r := newRig(t)
	viewer := r.create(t, boardSpec("Per viewer", `up{user="${__user.login}"}`))
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := r.create(t, boardSpec("Fresh", "up"))

	page, err := r.svc.List(t.Context(), r.editor, health.ListParams{DashboardIDs: []pgtype.UUID{viewer, fresh}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]health.Item{}
	for _, it := range page.Items {
		byTitle[it.Title] = it
	}
	if got := byTitle["Per viewer"]; got.Skipped != health.SkippedDependsOnViewer || got.State != health.StateSkipped || len(got.Problems) != 0 {
		t.Errorf("skipped dashboard = %+v", got)
	}
	if got := byTitle["Fresh"]; got.State != health.StateUnchecked || got.CheckedAt != nil || got.CheckedVersion != nil || len(got.Problems) != 0 {
		t.Errorf("never checked dashboard = %+v", got)
	}
}

func TestListWithDashboardIDsAndAKindKeepsOnlyDashboardsWithThatKind(t *testing.T) {
	r := newRig(t)
	empty, failing, healthy, _ := r.dashboardIDsFixture(t)
	ids := []pgtype.UUID{empty, failing, healthy}

	noData, err := r.svc.List(t.Context(), r.editor, health.ListParams{DashboardIDs: ids, Kind: health.KindNoData, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Empty"}; !slices.Equal(titles(noData.Items), want) {
		t.Errorf("kind no_data listed %v, want %v", titles(noData.Items), want)
	}
	unopened, err := r.svc.List(t.Context(), r.editor, health.ListParams{DashboardIDs: []pgtype.UUID{healthy}, Kind: health.KindUnopened, Limit: 100})
	if err != nil || len(unopened.Items) != 0 {
		t.Errorf("kind unopened listed %v for a healthy dashboard, %v", titles(unopened.Items), err)
	}
}

func TestListWithoutDashboardIDsStillListsOnlyDashboardsWithAProblem(t *testing.T) {
	r := newRig(t)
	r.dashboardIDsFixture(t)

	page, err := r.svc.List(t.Context(), r.editor, health.ListParams{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Failing", "Empty"}; !slices.Equal(titles(page.Items), want) {
		t.Errorf("listed %v, want %v", titles(page.Items), want)
	}
}

func TestListWithDashboardIDsHasNoSummaryOnAnyPage(t *testing.T) {
	r := newRig(t)
	empty, failing, _, _ := r.dashboardIDsFixture(t)
	ids := []pgtype.UUID{empty, failing}

	first, err := r.svc.List(t.Context(), r.editor, health.ListParams{DashboardIDs: ids, Limit: 1})
	if err != nil || first.NextCursor == "" || first.Summary != nil || len(first.Items) != 1 {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	second, err := r.svc.List(t.Context(), r.editor, health.ListParams{DashboardIDs: ids, Limit: 1, Cursor: first.NextCursor})
	if err != nil || second.NextCursor != "" || second.Summary != nil || len(second.Items) != 1 {
		t.Fatalf("second page = %+v, %v", second, err)
	}

	all, err := r.svc.List(t.Context(), r.editor, health.ListParams{Limit: 1})
	if err != nil || all.Summary == nil {
		t.Fatalf("unfiltered first page summary = %+v, %v", all.Summary, err)
	}
}
