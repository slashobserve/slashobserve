package threads

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r *rig) open(t *testing.T, by auth.Principal, in OpenInput) (Thread, Message) {
	t.Helper()
	thread, message, err := r.service.Open(t.Context(), by, in, nil)
	if err != nil {
		t.Fatalf("open %+v: %v", in, err)
	}
	return thread, message
}

func (r *rig) reply(t *testing.T, by auth.Principal, id, body string) (Thread, Message) {
	t.Helper()
	thread, message, err := r.service.Reply(t.Context(), by, mustParse(t, id), body)
	if err != nil {
		t.Fatalf("reply to %s: %v", id, err)
	}
	return thread, message
}

func (r *rig) list(t *testing.T, by auth.Principal, tg Target) Page {
	t.Helper()
	page, err := r.service.List(t.Context(), by, ListQuery{Target: tg, Status: StatusAll, Limit: 50})
	if err != nil {
		t.Fatalf("list %s: %v", tg, err)
	}
	return page
}

func TestEveryTargetOpensRepliesAndLists(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	finding := r.createFinding(t, testUser, dash)
	detached := r.createFinding(t, testUser, pgtype.UUID{})
	annotation := r.createAnnotation(t, testUser, dash)
	global := r.createAnnotation(t, testUser, pgtype.UUID{})

	tests := []struct {
		name   string
		target Target
		owner  string
	}{
		{"dashboard", target(KindDashboard, dash), dash.String()},
		{"finding on a dashboard", target(KindFinding, finding), dash.String()},
		{"finding with no dashboard", target(KindFinding, detached), ""},
		{"annotation on a dashboard", target(KindAnnotation, annotation), dash.String()},
		{"global annotation", target(KindAnnotation, global), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread, first := r.open(t, testUser, OpenInput{Target: tt.target, Body: "first"})
			if thread.ID != first.ID || first.ThreadID != thread.ID || thread.Target != tt.target || thread.Status != StatusOpen {
				t.Fatalf("thread %+v, first message %+v", thread, first)
			}
			if (tt.target.Kind == KindDashboard) != (thread.Version != nil) || (tt.target.Kind == KindDashboard) != (thread.DashboardID != "") {
				t.Errorf("anchor of %s = version %v, dashboard %q", tt.name, thread.Version, thread.DashboardID)
			}
			again, second := r.reply(t, testUser, first.ID, "second")
			if again.ID != thread.ID || second.ThreadID != thread.ID || second.ID == first.ID {
				t.Fatalf("reply %+v on %+v", second, again)
			}
			_, third := r.reply(t, testUser, second.ID, "third")
			if third.ThreadID != thread.ID {
				t.Errorf("a reply to a message must join its thread: %+v", third)
			}

			for _, id := range []string{thread.ID, second.ID, third.ID} {
				row, err := r.db.Queries().GetThreadOfMessage(t.Context(), mustParse(t, id))
				if err != nil || row.ID.String() != thread.ID {
					t.Errorf("GetThreadOfMessage(%s) = %v, %v", id, row.ID, err)
				}
			}

			page := r.list(t, testUser, tt.target)
			if len(page.Threads) != 1 || len(page.Messages) != 3 || page.Next != "" || !page.CanComment {
				t.Fatalf("page = %+v", page)
			}
			for i, want := range []string{first.ID, second.ID, third.ID} {
				if page.Messages[i].ID != want || !page.Messages[i].CanEdit {
					t.Errorf("message %d = %+v, want %s editable by its author", i, page.Messages[i], want)
				}
			}

			rows := r.activity(t)
			last := rows[len(rows)-1]
			if last.Type != activity.TypeThreadReplied || last.Dashboard != tt.owner {
				t.Errorf("last activity = %+v, want a reply owned by dashboard %q", last, tt.owner)
			}
		})
	}
}

func TestOpenOnADashboardAnchorsTheVersionAndPanel(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	r.addVersion(t, dash, 1)

	latest, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "whole"})
	if latest.Version == nil || *latest.Version != 2 || latest.PanelID != "" || !latest.PanelPresent {
		t.Errorf("default anchor = %+v", latest)
	}
	pinned, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Version: new(1), PanelID: new("rps"), Body: "panel"})
	if pinned.Version == nil || *pinned.Version != 1 || pinned.PanelID != "rps" || !pinned.PanelPresent {
		t.Errorf("pinned anchor = %+v", pinned)
	}

	invalid := func(err error, code string) bool {
		var in *InvalidInputError
		return errors.As(err, &in) && in.Code == code
	}
	finding := r.createFinding(t, testUser, dash)
	tests := []struct {
		name string
		in   OpenInput
		want func(error) bool
	}{
		{"a version above the latest", OpenInput{Target: target(KindDashboard, dash), Version: new(3), Body: "x"}, func(err error) bool { return errors.Is(err, ErrUnknownVersion) }},
		{"version zero", OpenInput{Target: target(KindDashboard, dash), Version: new(0), Body: "x"}, func(err error) bool { return invalid(err, CodeInvalidVersion) }},
		{"a panel not on that version", OpenInput{Target: target(KindDashboard, dash), Version: new(1), PanelID: new("errors"), Body: "x"}, func(err error) bool { return errors.Is(err, ErrUnknownPanel) }},
		{"an empty panel id", OpenInput{Target: target(KindDashboard, dash), PanelID: new(""), Body: "x"}, func(err error) bool { return invalid(err, CodeInvalidPanelID) }},
		{"a version on a finding", OpenInput{Target: target(KindFinding, finding), Version: new(1), Body: "x"}, func(err error) bool { return invalid(err, CodeInvalidTarget) }},
		{"a panel on a finding", OpenInput{Target: target(KindFinding, finding), PanelID: new("rps"), Body: "x"}, func(err error) bool { return invalid(err, CodeInvalidTarget) }},
		{"an empty body", OpenInput{Target: target(KindDashboard, dash), Body: " \n"}, func(err error) bool { return invalid(err, CodeInvalidBody) }},
		{"a body with a control character", OpenInput{Target: target(KindDashboard, dash), Body: "a\x07b"}, func(err error) bool { return invalid(err, CodeInvalidBody) }},
	}
	for _, tt := range tests {
		if _, _, err := r.service.Open(t.Context(), testUser, tt.in, nil); !tt.want(err) {
			t.Errorf("%s: err = %v", tt.name, err)
		}
	}
	if page := r.list(t, testUser, target(KindDashboard, dash)); len(page.Threads) != 2 {
		t.Errorf("a rejected open left %d threads, want 2", len(page.Threads))
	}
}

func TestAHiddenTargetAnswersNotFoundEverywhere(t *testing.T) {
	r := newRig(t)
	hidden := r.createDashboard(t, "rps")
	finding := r.createFinding(t, testUser, hidden)
	annotation := r.createAnnotation(t, testUser, hidden)
	thread, message := r.open(t, testUser, OpenInput{Target: target(KindDashboard, hidden), Body: "secret"})
	foldertest.Move(t, r.db, hidden, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))

	for _, tg := range []Target{target(KindDashboard, hidden), target(KindFinding, finding), target(KindAnnotation, annotation)} {
		if _, _, err := r.service.Open(t.Context(), viewer, OpenInput{Target: tg, Body: "hi"}, nil); !errors.Is(err, ErrTargetNotFound) {
			t.Errorf("open on hidden %s: err = %v", tg, err)
		}
		if _, err := r.service.List(t.Context(), viewer, ListQuery{Target: tg, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
			t.Errorf("list of hidden %s: err = %v", tg, err)
		}
		if _, err := r.service.Abilities(t.Context(), viewer, tg); !errors.Is(err, ErrTargetNotFound) {
			t.Errorf("abilities of hidden %s: err = %v", tg, err)
		}
	}
	missing := target(KindFinding, mustParse(t, "00000000-0000-4000-8000-000000000000"))
	if _, err := r.service.List(t.Context(), testUser, ListQuery{Target: missing, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("list of a missing finding: err = %v", err)
	}

	id := mustParse(t, thread.ID)
	if _, _, err := r.service.Reply(t.Context(), viewer, id, "hi"); !errors.Is(err, ErrNotFound) {
		t.Errorf("reply on a hidden thread: err = %v", err)
	}
	if _, _, _, err := r.service.Edit(t.Context(), viewer, mustParse(t, message.ID), "hi"); !errors.Is(err, ErrNotFound) {
		t.Errorf("edit in a hidden thread: err = %v", err)
	}
	if _, _, err := r.service.SetStatus(t.Context(), viewer, id, StatusResolved, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("resolve of a hidden thread: err = %v", err)
	}
}

func TestAnUnfiledFindingIsPrivateToItsCreatorAndAdmins(t *testing.T) {
	r := newRig(t)
	unfiled := r.createFinding(t, viewer, pgtype.UUID{})
	tg := target(KindFinding, unfiled)

	thread, _ := r.open(t, viewer, OpenInput{Target: tg, Body: "mine"})
	if _, err := r.service.List(t.Context(), admin, ListQuery{Target: tg, Limit: 10}); err != nil {
		t.Errorf("admin list: %v", err)
	}
	r.reply(t, admin, thread.ID, "an admin may comment")
	for _, by := range []auth.Principal{testUser, commenter, {Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Ed", Role: auth.RoleEditor}} {
		if _, err := r.service.List(t.Context(), by, ListQuery{Target: tg, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
			t.Errorf("%s list: err = %v, want ErrTargetNotFound", by.Name, err)
		}
	}

	abilities, err := r.service.Abilities(t.Context(), viewer, tg)
	if err != nil || !abilities.View || !abilities.Comment || !abilities.Edit || abilities.Manage {
		t.Errorf("a creator's abilities = %+v, err %v, want view, comment and edit only", abilities, err)
	}
}

func TestAFindingFiledInAFolderFollowsThatFoldersAccess(t *testing.T) {
	r := newRig(t)
	filed := target(KindFinding, r.createFinding(t, testUser, pgtype.UUID{}))
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'bo', 'bo@example.com', 'Bo')`, commenter.ID)
	mine := target(KindFinding, r.createFinding(t, commenter, pgtype.UUID{}))

	r.open(t, commenter, OpenInput{Target: filed, Body: "a commenter of General may comment"})
	if _, _, err := r.service.Open(t.Context(), viewer, OpenInput{Target: filed, Body: "no"}, nil); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer of General opening: err = %v, want folders.ErrForbidden", err)
	}
	if page, err := r.service.List(t.Context(), viewer, ListQuery{Target: filed, Limit: 10}); err != nil || len(page.Messages) != 1 || page.CanComment {
		t.Errorf("a viewer of General reads without commenting: %+v, err %v", page, err)
	}

	if _, err := r.service.List(t.Context(), testUser, ListQuery{Target: mine, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("a finding in someone's My Dashboards: err = %v, want ErrTargetNotFound", err)
	}
	if _, err := r.service.List(t.Context(), admin, ListQuery{Target: mine, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("an admin reading a finding in someone's My Dashboards: err = %v, want ErrTargetNotFound", err)
	}
	r.open(t, commenter, OpenInput{Target: mine, Body: "mine"})
}

func TestAGlobalAnnotationFollowsTheGeneralFolder(t *testing.T) {
	r := newRig(t)
	global := target(KindAnnotation, r.createAnnotation(t, testUser, pgtype.UUID{}))

	viewerAbilities, err := r.service.Abilities(t.Context(), viewer, global)
	if err != nil || !viewerAbilities.View || viewerAbilities.Comment {
		t.Errorf("viewer abilities = %+v, err %v", viewerAbilities, err)
	}
	if _, _, err := r.service.Open(t.Context(), viewer, OpenInput{Target: global, Body: "hi"}, nil); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer opening: err = %v, want folders.ErrForbidden", err)
	}
	if page, err := r.service.List(t.Context(), viewer, ListQuery{Target: global, Limit: 10}); err != nil || page.CanComment {
		t.Errorf("viewer list: %+v, err %v", page, err)
	}

	thread, _ := r.open(t, commenter, OpenInput{Target: global, Body: "from a commenter"})
	if _, _, err := r.service.Reply(t.Context(), viewer, mustParse(t, thread.ID), "no"); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer replying: err = %v, want folders.ErrForbidden", err)
	}
	if _, _, err := r.service.SetStatus(t.Context(), viewer, mustParse(t, thread.ID), StatusResolved, nil); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer resolving: err = %v, want folders.ErrForbidden", err)
	}

	rows := r.activity(t)
	if len(rows) != 1 || rows[0].Dashboard != "" || rows[0].Folder == "" {
		t.Errorf("a global annotation thread must land in the General folder: %+v", rows)
	}
}

func TestEditIsForTheAuthorOnly(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	thread, first := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "draft"})
	_, other := r.reply(t, commenter, thread.ID, "mine")

	if _, _, _, err := r.service.Edit(t.Context(), commenter, mustParse(t, first.ID), "hijack"); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("a non-author edit: err = %v, want ErrNotAuthor", err)
	}
	if _, _, _, err := r.service.Edit(t.Context(), testUser, mustParse(t, other.ID), "hijack"); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("an editor editing a commenter's message: err = %v, want ErrNotAuthor", err)
	}
	if _, _, _, err := r.service.Edit(t.Context(), testUser, mustParse(t, "00000000-0000-4000-8000-000000000000"), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unknown message: err = %v, want ErrNotFound", err)
	}

	_, edited, changed, err := r.service.Edit(t.Context(), testUser, mustParse(t, first.ID), "final")
	if err != nil || !changed || edited.Body != "final" || edited.EditedAt == nil || !edited.CanEdit {
		t.Fatalf("edit = %+v, changed %v, err %v", edited, changed, err)
	}
	if got := r.list(t, testUser, target(KindDashboard, dash)).Messages[0]; got.Body != "final" || got.EditedAt == nil {
		t.Errorf("listed message = %+v", got)
	}
	if got := r.list(t, viewer, target(KindDashboard, dash)).Messages[0]; got.CanEdit {
		t.Errorf("a viewer must not see an editable message: %+v", got)
	}
}

func TestEditNeedsCommentAbilityToStillHold(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	thread, message := r.open(t, commenter, OpenInput{Target: target(KindDashboard, dash), Body: "draft"})
	demoted := commenter
	demoted.Role = auth.RoleViewer
	if _, _, _, err := r.service.Edit(t.Context(), demoted, mustParse(t, message.ID), "later"); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("err = %v, want folders.ErrForbidden", err)
	}
	if got := r.list(t, demoted, target(KindDashboard, dash)).Messages[0]; got.CanEdit || got.Body != "draft" || got.ThreadID != thread.ID {
		t.Errorf("message = %+v", got)
	}
}

func TestAnUnchangedEditChangesAndAppendsNothing(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	_, first := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "same"})
	before := len(r.activity(t))

	_, got, changed, err := r.service.Edit(t.Context(), testUser, mustParse(t, first.ID), "  same\n")
	if err != nil || changed || got.EditedAt != nil || !got.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("edit = %+v, changed %v, err %v", got, changed, err)
	}
	if after := len(r.activity(t)); after != before {
		t.Errorf("an unchanged edit appended %d events", after-before)
	}
}

func TestEditRecordsMentionsAgainAndAppendsOnlyAnEditedEvent(t *testing.T) {
	r := newRig(t)
	finding := target(KindFinding, r.createFinding(t, testUser, pgtype.UUID{}))
	bo := r.createUser(t, "Bo Baker", "bo@example.com")
	cy := r.createUser(t, "", "cy@example.com")
	token := func(name string, id pgtype.UUID) string { return "@[" + name + "](user:" + id.String() + ")" }

	_, first := r.open(t, testUser, OpenInput{Target: finding, Body: "hi " + token("whoever", bo) + " and @observe"})
	if len(first.Mentions) != 2 || first.Mentions[0].Name != "Bo Baker" || first.Mentions[1].Agent != AgentObserve {
		t.Fatalf("recorded mentions = %+v, want the stored name, not the token's, then the agent", first.Mentions)
	}
	opened := r.activity(t)
	if got := activityTypes(opened); strings.Join(got, ",") != "thread.opened,thread.mentioned,thread.mentioned" {
		t.Fatalf("activity after open = %v", got)
	}

	_, edited, changed, err := r.service.Edit(t.Context(), testUser, mustParse(t, first.ID), "now "+token("x", cy)+" only")
	if err != nil || !changed {
		t.Fatalf("edit: changed %v, err %v", changed, err)
	}
	if len(edited.Mentions) != 1 || edited.Mentions[0].UserID != cy || edited.Mentions[0].Name != "cy@example.com" {
		t.Errorf("mentions after the edit = %+v, want only Cy by email", edited.Mentions)
	}
	if listed := r.list(t, testUser, finding).Messages[0].Mentions; len(listed) != 1 || listed[0].UserID != cy {
		t.Errorf("listed mentions = %+v", listed)
	}
	rows := r.activity(t)
	if got := activityTypes(rows[len(opened):]); strings.Join(got, ",") != "thread.edited" {
		t.Errorf("an edit must append only thread.edited, got %v", got)
	}
}

func TestOnlyRealUsersAreRecordedAsMentions(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	ghost := "@[Ghost](user:99999999-9999-4999-8999-999999999999)"
	present := r.createUser(t, "Real", "real@example.com")

	_, message := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: ghost + " @[Real](user:" + present.String() + ")"})
	if len(message.Mentions) != 1 || message.Mentions[0].UserID != present {
		t.Fatalf("mentions = %+v", message.Mentions)
	}
	if got := activityTypes(r.activity(t)); strings.Join(got, ",") != "thread.opened,thread.mentioned" {
		t.Errorf("activity = %v, want one mention event for the real user", got)
	}
	_, _, err := r.service.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, dash), Body: strings.Repeat("@observe ", 3) + manyTokens(21)}, nil)
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeTooManyMentions {
		t.Errorf("21 mentions: err = %v", err)
	}
}

func manyTokens(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("@[U](user:")
		b.WriteString(strings.Replace("00000000-0000-4000-8000-0000000000XX", "XX", string(rune('0'+i/10))+string(rune('0'+i%10)), 1))
		b.WriteString(") ")
	}
	return b.String()
}

func TestAReplyNeverReopensAResolvedThread(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "root"})
	id := mustParse(t, thread.ID)
	if _, changed, err := r.service.SetStatus(t.Context(), testUser, id, StatusResolved, nil); err != nil || !changed {
		t.Fatalf("resolve: changed %v, err %v", changed, err)
	}
	got, _ := r.reply(t, commenter, thread.ID, "late reply")
	if got.Status != StatusResolved || got.ResolvedBy == nil || got.ResolvedBy.ID != testUserID {
		t.Errorf("thread after a reply = %+v, want still resolved by Ada", got)
	}
	if page := r.list(t, testUser, target(KindDashboard, dash)); page.Threads[0].Status != StatusResolved || len(page.Messages) != 2 {
		t.Errorf("page = %+v", page)
	}
}

func TestStatusChangesReportChangeAndRecordTheResolver(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	thread, first := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Version: new(1), Body: "root"})
	id := mustParse(t, first.ID)

	resolved, changed, err := r.service.SetStatus(t.Context(), commenter, id, StatusResolved, new(2))
	if err != nil || !changed || resolved.ID != thread.ID || resolved.Status != StatusResolved || resolved.ResolvedAt == nil ||
		resolved.ResolvedByVersion == nil || *resolved.ResolvedByVersion != 2 || resolved.ResolvedBy == nil || resolved.ResolvedBy.ID != commenterID {
		t.Fatalf("resolved = %+v, changed %v, err %v", resolved, changed, err)
	}
	again, changed, err := r.service.SetStatus(t.Context(), testUser, id, StatusResolved, nil)
	if err != nil || changed || again.ResolvedBy.ID != commenterID || !again.UpdatedAt.Equal(resolved.UpdatedAt) {
		t.Fatalf("a repeat resolve = %+v, changed %v, err %v", again, changed, err)
	}
	reopened, changed, err := r.service.SetStatus(t.Context(), testUser, id, StatusOpen, nil)
	if err != nil || !changed || reopened.Status != StatusOpen || reopened.ResolvedAt != nil || reopened.ResolvedBy != nil || reopened.ResolvedByVersion != nil {
		t.Fatalf("reopened = %+v, changed %v, err %v", reopened, changed, err)
	}

	var invalid *InvalidInputError
	for name, tt := range map[string]struct {
		status  string
		version *int
		want    error
	}{
		"above the latest":        {StatusResolved, new(3), ErrUnknownVersion},
		"below the thread":        {StatusResolved, new(0), nil},
		"with status open":        {StatusOpen, new(1), nil},
		"an unknown status":       {"closed", nil, nil},
		"a version on a finding":  {StatusResolved, new(1), nil},
		"zero":                    {StatusResolved, new(0), nil},
		"a version above int32":   {StatusResolved, new(1 << 31), nil},
		"a negative version":      {StatusResolved, new(-4), nil},
		"status open and version": {StatusOpen, new(2), nil},
	} {
		target := id
		if name == "a version on a finding" {
			finding := r.createFinding(t, testUser, pgtype.UUID{})
			ft, _ := r.open(t, testUser, OpenInput{Target: Target{Kind: KindFinding, ID: finding}, Body: "x"})
			target = mustParse(t, ft.ID)
		}
		_, _, err := r.service.SetStatus(t.Context(), testUser, target, tt.status, tt.version)
		if tt.want != nil && !errors.Is(err, tt.want) || tt.want == nil && !errors.As(err, &invalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestEveryEventTypeAppendsItsDataWithoutBodyText(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps")
	bo := r.createUser(t, "Bo", "bo@example.com")
	const secret = "a distinctive comment body"

	thread, first := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), PanelID: new("rps"), Body: secret + " @[Bo](user:" + bo.String() + ")"})
	_, second := r.reply(t, commenter, thread.ID, secret+" again")
	if _, _, _, err := r.service.Edit(t.Context(), commenter, mustParse(t, second.ID), secret+" edited"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, thread.ID), StatusResolved, new(1)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, thread.ID), StatusOpen, nil); err != nil {
		t.Fatal(err)
	}

	rows := r.activity(t)
	want := []string{"thread.opened", "thread.mentioned", "thread.replied", "thread.edited", "thread.resolved", "thread.reopened"}
	if got := activityTypes(rows); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("types = %v, want %v", got, want)
	}
	object := map[string]any{"kind": "dashboard", "id": dash.String()}
	for i, row := range rows {
		if row.TargetKind != "thread" || row.TargetID != thread.ID || row.Dashboard != dash.String() || row.Folder == "" {
			t.Errorf("row %d = %+v", i, row)
		}
		if got, _ := row.Data["object"].(map[string]any); got["kind"] != object["kind"] || got["id"] != object["id"] {
			t.Errorf("row %d object = %v", i, row.Data["object"])
		}
	}
	if rows[0].ActorID != testUserID || rows[0].Data["version"] != float64(1) || rows[0].Data["panel_id"] != "rps" {
		t.Errorf("opened = %+v", rows[0])
	}
	if m, _ := rows[1].Data["mentioned"].(map[string]any); m["kind"] != "user" || m["id"] != bo.String() || m["name"] != "Bo" || rows[1].Data["message_id"] != first.ID {
		t.Errorf("mentioned = %+v", rows[1])
	}
	if rows[2].ActorID != commenterID || rows[2].Data["message_id"] != second.ID {
		t.Errorf("replied = %+v", rows[2])
	}
	if rows[3].Data["message_id"] != second.ID {
		t.Errorf("edited = %+v", rows[3])
	}
	if rows[4].Data["resolved_by_version"] != float64(1) {
		t.Errorf("resolved = %+v", rows[4])
	}
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT string_agg(data::text, ' ') FROM activity WHERE target_kind = 'thread'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "distinctive") {
		t.Errorf("activity data carries comment text: %s", raw)
	}
}

func TestDeletingTheTargetDeletesItsThreads(t *testing.T) {
	r := newRig(t)
	annotation := r.createAnnotation(t, testUser, pgtype.UUID{})
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindAnnotation, annotation), Body: "gone soon"})
	r.reply(t, testUser, thread.ID, "reply")

	if err := r.annotations.Delete(t.Context(), testUser, annotation); err != nil {
		t.Fatal(err)
	}
	var threads, messages int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM threads), (SELECT count(*) FROM thread_messages)`).Scan(&threads, &messages); err != nil {
		t.Fatal(err)
	}
	if threads != 0 || messages != 0 {
		t.Errorf("%d threads and %d messages survived their annotation", threads, messages)
	}
	if _, _, err := r.service.Reply(t.Context(), testUser, mustParse(t, thread.ID), "late"); !errors.Is(err, ErrNotFound) {
		t.Errorf("reply on a deleted target: err = %v, want ErrNotFound", err)
	}
}

func TestListPagesByMessageAndFilters(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t, "rps", "errors")
	tg := target(KindDashboard, dash)
	a, _ := r.open(t, testUser, OpenInput{Target: tg, PanelID: new("rps"), Body: "a"})
	r.reply(t, testUser, a.ID, "a2")
	b, _ := r.open(t, testUser, OpenInput{Target: tg, PanelID: new("errors"), Body: "b"})
	if _, _, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, b.ID), StatusResolved, nil); err != nil {
		t.Fatal(err)
	}

	var bodies []string
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		page, err := r.service.List(t.Context(), testUser, ListQuery{Target: tg, Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page.Messages {
			bodies = append(bodies, m.Body)
		}
		if page.Next == "" {
			break
		}
		cursor = page.Next
	}
	if strings.Join(bodies, ",") != "a,a2,b" {
		t.Errorf("paged bodies = %v", bodies)
	}

	for name, tt := range map[string]struct {
		query ListQuery
		want  int
	}{
		"open only":     {ListQuery{Target: tg, Status: StatusOpen, Limit: 10}, 2},
		"resolved only": {ListQuery{Target: tg, Status: StatusResolved, Limit: 10}, 1},
		"one panel":     {ListQuery{Target: tg, Status: StatusAll, PanelID: "errors", Limit: 10}, 1},
	} {
		page, err := r.service.List(t.Context(), testUser, tt.query)
		if err != nil || len(page.Messages) != tt.want {
			t.Errorf("%s: %d messages, err %v, want %d", name, len(page.Messages), err, tt.want)
		}
	}
	if _, err := r.service.List(t.Context(), testUser, ListQuery{Target: tg, Status: "bogus", Limit: 10}); err == nil {
		t.Error("an unknown status was accepted")
	}
	if _, err := r.service.List(t.Context(), testUser, ListQuery{Target: tg, Limit: 10, Cursor: "!!"}); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}

func TestAThreadMessageOnAFindingWithNoDashboardIsOwnedByTheFinding(t *testing.T) {
	r := newRig(t)
	detached := r.createFinding(t, testUser, pgtype.UUID{})
	onDashboard := r.createFinding(t, testUser, r.createDashboard(t, "rps"))
	r.open(t, testUser, OpenInput{Target: target(KindFinding, detached), Body: "who owns this"})
	r.open(t, testUser, OpenInput{Target: target(KindFinding, onDashboard), Body: "and this"})

	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT coalesce(finding_id::text, ''), coalesce(dashboard_id::text, '') FROM activity WHERE type = 'thread.opened' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var findingIDs, dashboardIDs []string
	for rows.Next() {
		var finding, dashboard string
		if err := rows.Scan(&finding, &dashboard); err != nil {
			t.Fatal(err)
		}
		findingIDs, dashboardIDs = append(findingIDs, finding), append(dashboardIDs, dashboard)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(findingIDs) != 2 || findingIDs[0] != detached.String() || dashboardIDs[0] != "" || findingIDs[1] != "" || dashboardIDs[1] == "" {
		t.Fatalf("finding ids %v and dashboard ids %v, want the detached finding owning its thread and the dashboard finding's thread owned by its dashboard", findingIDs, dashboardIDs)
	}
}
