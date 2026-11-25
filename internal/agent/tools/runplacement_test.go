package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	ownRunID   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	otherRunID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func readsIn(t *testing.T, by auth.Principal, audience datasource.Audience) (context.Context, func() bool) {
	t.Helper()
	access := datasource.Access{By: by, Audience: audience}
	ctx := WithRun(auth.WithPrincipal(t.Context(), by), Run{RequestID: ownRunID, Access: access})
	return datasource.NotingRestrictedReads(datasource.WithAccess(ctx, access))
}

func TestPrivateReadRuleFollowsWhoReadsWhatTheRunWrites(t *testing.T) {
	cases := []struct {
		name  string
		inRun bool
		a     datasource.Access
		ok    bool
		want  readRule
	}{
		{"outside a run", false, datasource.Access{By: adminCaller}, true, readsFreely},
		{"the server itself", true, datasource.Access{System: true}, true, readsFreely},
		{"a private chat", true, datasource.Access{By: editorCaller, Audience: datasource.AudienceCaller}, true, readsAndMarks},
		{"a run writing where only Admins read", true, datasource.Access{By: adminCaller, Audience: datasource.AudienceAdmins}, true, readsAndMarks},
		{"a run writing where others read", true, datasource.Access{By: editorCaller, Audience: datasource.AudienceEveryone}, true, readsRefused},
		{"an Admin's run writing where others read", true, datasource.Access{By: adminCaller, Audience: datasource.AudienceEveryone}, true, readsRefused},
		{"a run with no access in its context", true, datasource.Access{}, false, readsRefused},
	}
	for _, c := range cases {
		if got := privateReadRule(c.inRun, c.a, c.ok); got != c.want {
			t.Errorf("%s: rule %d, want %d", c.name, got, c.want)
		}
	}
}

func TestAuditInRunRuleFollowsWhoReadsWhatTheRunWrites(t *testing.T) {
	cases := []struct {
		name  string
		inRun bool
		a     datasource.Access
		want  readRule
	}{
		{"outside a run", false, datasource.Access{By: editorCaller}, readsFreely},
		{"an Admin's private chat", true, datasource.Access{By: adminCaller, Audience: datasource.AudienceCaller}, readsAndMarks},
		{"an Admin writing where only Admins read", true, datasource.Access{By: adminCaller, Audience: datasource.AudienceAdmins}, readsAndMarks},
		{"an Admin writing where others read", true, datasource.Access{By: adminCaller, Audience: datasource.AudienceEveryone}, readsRefused},
		{"an Editor's private chat", true, datasource.Access{By: editorCaller, Audience: datasource.AudienceCaller}, readsRefused},
	}
	for _, c := range cases {
		if got := auditReadRule(c.inRun, c.a, true); got != c.want {
			t.Errorf("%s: rule %d, want %d", c.name, got, c.want)
		}
	}
}

func TestAuditInRunToolsRefuseWhereOthersReadAndMarkWhereOnlyAdminsDo(t *testing.T) {
	r := New(Deps{Logger: slog.New(slog.DiscardHandler)})
	handlers := map[string]func(context.Context, json.RawMessage) (Result, error){
		nameSearchAuditEvents:    r.searchAuditEvents,
		nameGetAuditEvent:        r.getAuditEvent,
		nameListAuditFacets:      r.listAuditFacets,
		nameGetAuditHistogram:    r.getAuditHistogram,
		nameAggregateAuditEvents: r.aggregateAuditEvents,
	}
	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			ctx, read := readsIn(t, adminCaller, datasource.AudienceEveryone)
			res, err := handle(ctx, []byte(`{}`))
			if err != nil || errorCode(t, res) != CodeAdminsOnlyDatasource || read() {
				t.Fatalf("an Admin writing where others read: %+v, %v, marked %v", res, err, read())
			}
			ctx, read = readsIn(t, editorCaller, datasource.AudienceCaller)
			res, err = handle(ctx, []byte(`{}`))
			if err != nil || errorCode(t, res) != codeForbiddenRole || read() {
				t.Fatalf("an Editor: %+v, %v, marked %v", res, err, read())
			}
			ctx, read = readsIn(t, adminCaller, datasource.AudienceCaller)
			if _, err := handle(ctx, []byte(`{}`)); !errors.Is(err, errAuditNotConfigured) || !read() {
				t.Fatalf("an Admin in a private chat: err %v, marked %v", err, read())
			}
		})
	}
}

func TestPrivateReadsInARunMarkItWhereOnlyTheRequesterAndAdminsReadAndRefuseItWhereOthersRead(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"requests":[],"chats":[],"turns":[],"chat":{}}`)}
	r := New(Deps{Runs: runs, Logger: slog.New(slog.DiscardHandler)})
	reads := map[string]struct {
		handle func(context.Context, json.RawMessage) (Result, error)
		input  string
	}{
		nameGetRun:      {r.getRun, `{"id":"` + otherRunID + `"}`},
		nameGetRunBuild: {r.getRunBuild, `{"id":"` + otherRunID + `"}`},
		nameListRuns:    {r.listRuns, `{}`},
		nameListChats:   {r.listChats, `{}`},
		nameGetChat:     {r.getChat, `{"id":"` + otherRunID + `"}`},
	}
	for name, c := range reads {
		t.Run(name, func(t *testing.T) {
			ctx, read := readsIn(t, editorCaller, datasource.AudienceCaller)
			if _, err := c.handle(ctx, []byte(c.input)); err != nil || !read() {
				t.Fatalf("a private chat: err %v, marked %v", err, read())
			}
			runs.invoked = false
			ctx, read = readsIn(t, editorCaller, datasource.AudienceEveryone)
			res, err := c.handle(ctx, []byte(c.input))
			if err != nil || errorCode(t, res) != codePrivateRead || read() || runs.invoked {
				t.Fatalf("a run others read: %+v, %v, marked %v, read the desk %v", res, err, read(), runs.invoked)
			}
		})
	}
}

func TestPrivateReadOfTheRunsOwnRecordMarksNothing(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{}`)}
	r := New(Deps{Runs: runs, Logger: slog.New(slog.DiscardHandler)})
	for _, audience := range []datasource.Audience{datasource.AudienceCaller, datasource.AudienceEveryone} {
		ctx, read := readsIn(t, editorCaller, audience)
		res, err := r.getRun(ctx, []byte(`{"id":"`+ownRunID+`"}`))
		if err != nil || res.IsError || read() {
			t.Fatalf("audience %q: %+v, %v, marked %v", audience, res, err, read())
		}
	}
}

func TestPrivateReadOutsideARunReadsAsBefore(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{}`)}
	r := New(Deps{Runs: runs, Logger: slog.New(slog.DiscardHandler)})
	ctx, read := datasource.NotingRestrictedReads(datasource.WithAccess(auth.WithPrincipal(t.Context(), editorCaller), datasource.Access{By: editorCaller}))
	if res, err := r.getRun(ctx, []byte(`{"id":"`+otherRunID+`"}`)); err != nil || res.IsError || read() {
		t.Fatalf("%+v, %v, marked %v", res, err, read())
	}
}

func TestRestrictedTitlesLeaveOutWhatIsInAFolderOnlyAdminsReadAndSayHowManyWithoutOfferingTheFlag(t *testing.T) {
	open, hidden := mustUUID(t, "11111111-1111-4111-8111-111111111111"), mustUUID(t, "22222222-2222-4222-8222-222222222222")
	run := Run{Access: datasource.Access{By: adminCaller, Audience: datasource.AudienceCaller}}
	titles := &restrictedTitles{rule: titleRuleFor(run, true, false), run: run, adminsOnly: map[pgtype.UUID]bool{hidden: true}}
	type item struct{ folder, id string }
	items := []item{{open.String(), "a"}, {hidden.String(), "b"}, {"", "c"}, {hidden.String(), "d"}}
	kept := keepReadable(titles, items, func(i item) (string, string) { return i.folder, i.id })
	if len(kept) != 2 || kept[0].id != "a" || kept[1].id != "c" || titles.leftOut != 2 {
		t.Fatalf("kept %+v, left out %d", kept, titles.leftOut)
	}
	report := titles.report("dashboard", "dashboards")
	if report.LeftOut != 2 || !strings.Contains(report.LeftOutWhy, "2 dashboards") || strings.Contains(report.LeftOutWhy, "include_restricted") {
		t.Fatalf("report %+v", report)
	}
	kept = keepReadable(titles, []item{{open.String(), "a"}}, func(i item) (string, string) { return i.folder, i.id })
	if len(kept) != 1 || titles.report("dashboard", "dashboards") != (leftOutOut{}) {
		t.Fatalf("a second page starts its count again: kept %d, report %+v", len(kept), titles.report("dashboard", "dashboards"))
	}
}

func TestRestrictedTitlesKeepEverythingWhereTheRunCannotReadWhatIsHidden(t *testing.T) {
	hidden := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	for name, run := range map[string]Run{
		"an Editor's private chat":           {Access: datasource.Access{By: editorCaller, Audience: datasource.AudienceCaller}},
		"an Admin writing where others read": {Access: datasource.Access{By: adminCaller, Audience: datasource.AudienceEveryone}},
		"outside a run":                      {},
	} {
		titles := &restrictedTitles{rule: titleRuleFor(run, name != "outside a run", false), run: run, adminsOnly: map[pgtype.UUID]bool{hidden: true}}
		if !titles.keepIn(hidden.String(), "x") || !titles.keepAdminsOnly() || titles.leftOut != 0 {
			t.Errorf("%s: left something out", name)
		}
	}
}

func TestRestrictedTitlesHoldingASingleObjectMarksTheRunInsteadOfHidingIt(t *testing.T) {
	hidden := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	run := Run{Access: datasource.Access{By: adminCaller, Audience: datasource.AudienceCaller}}
	titles := &restrictedTitles{rule: titleRuleFor(run, true, false), run: run, adminsOnly: map[pgtype.UUID]bool{hidden: true}}
	ctx, read := readsIn(t, adminCaller, datasource.AudienceCaller)
	titles.holds(ctx, mustUUID(t, "11111111-1111-4111-8111-111111111111").String(), "x")
	if read() {
		t.Fatal("a dashboard in a folder everyone reads marked the run")
	}
	titles.holds(ctx, hidden.String(), "x")
	if !read() {
		t.Fatal("a dashboard in a folder only Admins read did not mark the run")
	}
}

func TestRestrictedTitlesKeepActivityOfAdminChangesOutOfAnAdminsPrivateRun(t *testing.T) {
	run := Run{Access: datasource.Access{By: adminCaller, Audience: datasource.AudienceCaller}}
	titles := &restrictedTitles{rule: titleRuleFor(run, true, false), run: run, adminsOnly: map[pgtype.UUID]bool{}}
	adminType := activity.AdminTypes()[0]
	events := []activity.EventJSON{{Type: adminType}, {Type: "dashboard.saved", Target: activity.TargetJSON{ID: "d"}}}
	kept := keepActivity(titles, events)
	if len(kept) != 1 || kept[0].Type != "dashboard.saved" || titles.leftOut != 1 {
		t.Fatalf("kept %+v, left out %d", kept, titles.leftOut)
	}
}
