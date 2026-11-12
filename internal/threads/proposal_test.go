package threads

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r *rig) createProposal(t *testing.T, dash pgtype.UUID) pgtype.UUID {
	t.Helper()
	res, err := r.dashboards.Save(t.Context(), commenter, dashboards.SaveInput{
		DashboardID:     dash,
		Parent:          1,
		Spec:            panelsSpec("t", "rps", "errors"),
		Summary:         "add errors",
		ProposeIfNeeded: true,
	})
	if err != nil || res.Proposal == nil {
		t.Fatalf("propose: %+v, %v", res, err)
	}
	return mustParse(t, res.Proposal.ID)
}

func (r *rig) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAProposalTakesCommentsFromThoseWhoCanCommentOnItsDashboard(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	tg := target(KindProposal, r.createProposal(t, dash))

	thread, first := r.open(t, commenter, OpenInput{Target: tg, Body: "why drop the old panel?"})
	if thread.Target != tg || thread.Status != StatusOpen || thread.DashboardID != "" || thread.Version != nil || thread.PanelID != "" {
		t.Fatalf("thread %+v", thread)
	}
	r.reply(t, testUser, first.ID, "it was unused")
	resolved, changed, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, thread.ID), StatusResolved, nil)
	if err != nil || !changed || resolved.Status != StatusResolved {
		t.Fatalf("resolve = %+v, %v, %v", resolved, changed, err)
	}
	reopened, changed, err := r.service.SetStatus(t.Context(), commenter, mustParse(t, thread.ID), StatusOpen, nil)
	if err != nil || !changed || reopened.Status != StatusOpen {
		t.Fatalf("reopen = %+v, %v, %v", reopened, changed, err)
	}

	page := r.list(t, viewer, tg)
	if len(page.Threads) != 1 || len(page.Messages) != 2 || page.CanComment {
		t.Fatalf("a viewer reads %+v", page)
	}
	if page := r.list(t, commenter, tg); !page.CanComment {
		t.Error("a commenter can write")
	}
	if page := r.list(t, testUser, target(KindDashboard, dash)); len(page.Threads) != 0 {
		t.Errorf("the dashboard's own threads include the proposal's: %+v", page)
	}
}

func TestAViewerReadsAProposalThreadButCannotWriteToIt(t *testing.T) {
	r := newRig(t)
	tg := target(KindProposal, r.createProposal(t, r.createDashboard(t)))
	thread, _ := r.open(t, commenter, OpenInput{Target: tg, Body: "hello"})

	if _, _, err := r.service.Open(t.Context(), viewer, OpenInput{Target: tg, Body: "hi"}, nil); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("open: %v", err)
	}
	if _, _, err := r.service.Reply(t.Context(), viewer, mustParse(t, thread.ID), "hi"); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("reply: %v", err)
	}
	denied := r.call(t, viewer, http.MethodPost, "/threads", map[string]any{"target": tg.String(), "body": "hi"})
	denied.expect(t, http.StatusForbidden, "forbidden_role")
}

func TestAProposalOnADashboardTheCallerCannotSeeIsNotFound(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	tg := target(KindProposal, r.createProposal(t, dash))
	thread, _ := r.open(t, commenter, OpenInput{Target: tg, Body: "private"})
	foldertest.Move(t, r.db, dash, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))

	if _, _, err := r.service.Open(t.Context(), viewer, OpenInput{Target: tg, Body: "hi"}, nil); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("open: %v", err)
	}
	if _, err := r.service.List(t.Context(), viewer, ListQuery{Target: tg, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("list: %v", err)
	}
	if _, _, err := r.service.Reply(t.Context(), viewer, mustParse(t, thread.ID), "hi"); !errors.Is(err, ErrNotFound) {
		t.Errorf("reply: %v", err)
	}
	r.call(t, viewer, http.MethodGet, "/threads?target="+tg.String(), nil).expect(t, http.StatusNotFound, "")
	r.call(t, viewer, http.MethodPost, "/threads", map[string]any{"target": tg.String(), "body": "hi"}).expect(t, http.StatusNotFound, "")
	if _, err := r.service.List(t.Context(), admin, ListQuery{Target: tg, Limit: 10}); err != nil {
		t.Errorf("the admin: %v", err)
	}
}

func TestAMissingProposalIsNotFound(t *testing.T) {
	r := newRig(t)
	missing := target(KindProposal, mustParse(t, "00000000-0000-4000-8000-000000000000"))
	if _, err := r.service.List(t.Context(), testUser, ListQuery{Target: missing, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("list: %v", err)
	}
	if _, _, err := r.service.Open(t.Context(), testUser, OpenInput{Target: missing, Body: "hi"}, nil); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("open: %v", err)
	}
}

func TestAProposalThreadIsAnAnchorlessTarget(t *testing.T) {
	r := newRig(t)
	tg := target(KindProposal, r.createProposal(t, r.createDashboard(t)))
	version, panel := 1, "rps"
	for name, in := range map[string]OpenInput{"a version": {Target: tg, Body: "x", Version: &version}, "a panel": {Target: tg, Body: "x", PanelID: &panel}} {
		_, _, err := r.service.Open(t.Context(), commenter, in, nil)
		var invalid *InvalidInputError
		if !errors.As(err, &invalid) || invalid.Code != CodeInvalidTarget {
			t.Errorf("%s: %v", name, err)
		}
	}
	thread, _ := r.open(t, commenter, OpenInput{Target: tg, Body: "x"})
	one := 1
	_, _, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, thread.ID), StatusResolved, &one)
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeInvalidVersion {
		t.Errorf("resolved_by_version: %v", err)
	}
}

func TestAStoredProposalThreadReadsBackAsAProposalTarget(t *testing.T) {
	r := newRig(t)
	id := r.createProposal(t, r.createDashboard(t))
	thread, _ := r.open(t, commenter, OpenInput{Target: target(KindProposal, id), Body: "x"})
	if got := targetOf(r.threadRow(t, thread.ID)); got != target(KindProposal, id) {
		t.Errorf("targetOf = %+v", got)
	}
}

func TestOpeningAThreadOnAProposalOverHTTPAnswersTheProposalTarget(t *testing.T) {
	r := newRig(t)
	tg := "proposal:" + r.createProposal(t, r.createDashboard(t)).String()
	created := r.call(t, commenter, http.MethodPost, "/threads", map[string]any{"target": tg, "body": "first"})
	created.expect(t, http.StatusCreated, "")
	if created.at(t, "thread", "target", "kind") != "proposal" || created.at(t, "thread", "target", "id") != strings.TrimPrefix(tg, "proposal:") || created.at(t, "thread", "dashboard_id") != nil {
		t.Errorf("thread = %v", created.body["thread"])
	}
	listed := r.call(t, viewer, http.MethodGet, "/threads?target="+tg, nil)
	listed.expect(t, http.StatusOK, "")
	if messages, _ := listed.at(t, "messages").([]any); len(messages) != 1 || listed.at(t, "can_comment") != false {
		t.Errorf("list = %v", listed.body)
	}
}

func TestThreadEventsOnAProposalBelongToItsDashboardAndStartNoRun(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	bo := r.createUser(t, "Bo Baker", "bo@example.com")
	thread, first := r.open(t, commenter, OpenInput{Target: target(KindProposal, r.createProposal(t, dash)), Body: "ping @[x](user:" + bo.String() + ") and @observe"})
	r.reply(t, testUser, first.ID, "more")
	if _, _, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, thread.ID), StatusResolved, nil); err != nil {
		t.Fatal(err)
	}

	rows := r.activity(t)
	want := []string{activity.TypeThreadOpened, activity.TypeThreadMentioned, activity.TypeThreadMentioned, activity.TypeThreadReplied, activity.TypeThreadResolved}
	if got := activityTypes(rows); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("types %v, want %v", got, want)
	}
	for _, row := range rows {
		if row.Dashboard != dash.String() || row.Folder == "" {
			t.Errorf("%s: dashboard %q folder %q", row.Type, row.Dashboard, row.Folder)
		}
		if object := row.Data["object"].(map[string]any); object["kind"] != "proposal" {
			t.Errorf("%s: object %v", row.Type, object)
		}
	}
	if n := r.count(t, `SELECT count(*) FROM agent_requests`); n != 0 {
		t.Errorf("%d agent requests started", n)
	}
}

func TestDeletingADashboardDeletesItsProposalsAndTheirThreads(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	r.open(t, commenter, OpenInput{Target: target(KindProposal, r.createProposal(t, dash)), Body: "x"})
	if n := r.count(t, `SELECT count(*) FROM threads WHERE proposal_id IS NOT NULL`); n != 1 {
		t.Fatalf("threads before = %d", n)
	}

	if err := r.dashboards.Delete(t.Context(), testUser, dash); err != nil {
		t.Fatal(err)
	}
	if n := r.count(t, `SELECT (SELECT count(*) FROM proposals) + (SELECT count(*) FROM threads)`); n != 0 {
		t.Errorf("%d proposals and threads left", n)
	}
}
