package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const commentRejection = "comment tools are for MCP clients; the server answers and resolves threads after a run"

func commentCalls() map[string]string {
	return map[string]string{
		nameListComments:   `{"dashboard_id":"` + dashA + `"}`,
		nameCreateComment:  `{"target":"dashboard:` + dashA + `","body":"done"}`,
		nameEditComment:    `{"comment_id":"` + dashB + `","body":"done"}`,
		nameReopenComment:  `{"comment_id":"` + dashB + `"}`,
		nameReplyToComment: `{"comment_id":"` + dashB + `","body":"done"}`,
		nameResolveComment: `{"comment_id":"` + dashB + `","version":1}`,
	}
}

func TestCommentToolsRejectAgentRuns(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
	runs := map[string]Run{
		"create run":       {Mode: ModeCreate},
		"revise run":       {Mode: ModeRevise, DashboardID: dashA, ParentVersion: 1},
		"other revise run": {Mode: ModeRevise, DashboardID: dashB, ParentVersion: 2},
		"zero run":         {},
	}
	for tool, input := range commentCalls() {
		for name, run := range runs {
			t.Run(tool+" "+name, func(t *testing.T) {
				res, err := r.Call(WithRun(ctx, run), tool, []byte(input))
				if err != nil {
					t.Fatal(err)
				}
				if errorCode(t, res) != codeInvalidRequest || res.Content.(errorBody).Message != commentRejection {
					t.Fatalf("res = %#v", res.Content)
				}
				if res.Step != "Rejected "+tool+" input: "+commentRejection {
					t.Fatalf("step = %q", res.Step)
				}
			})
		}
	}
}

func TestMCPOnlyTools(t *testing.T) {
	var got []string
	for _, tool := range New(Deps{}).List() {
		if tool.MCPOnly {
			got = append(got, tool.Name)
		}
	}
	if strings.Join(got, ",") != "claim_run,create_annotation,create_comment,create_finding,draft_query,edit_comment,finish_run,get_run,list_activity,list_comments,list_dashboard_health,list_folders,list_saved_searches,move_finding,reopen_comment,reply_to_comment,report_run,resolve_comment,save_search,search_workspace" {
		t.Fatalf("mcp-only tools = %v", got)
	}
}

func TestCommentToolsNeedComments(t *testing.T) {
	r := New(Deps{})
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
	for tool, input := range commentCalls() {
		t.Run(tool, func(t *testing.T) {
			res, err := r.Call(ctx, tool, []byte(input))
			if !errors.Is(err, errCommentsNotConfigured) || res.Content != nil || res.IsError {
				t.Fatalf("res = %#v err = %v, want an internal error", res, err)
			}
		})
	}
}

func TestParseCommentQuery(t *testing.T) {
	dash := `{"dashboard_id":"` + dashA + `"`
	byTarget := `{"target":"finding:` + dashB + `"`
	dashboardTarget := threads.Target{Kind: threads.KindDashboard, ID: mustUUID(t, dashA)}
	findingTarget := threads.Target{Kind: threads.KindFinding, ID: mustUUID(t, dashB)}
	tests := []struct {
		name    string
		input   string
		want    commentQuery
		wantMsg string
	}{
		{name: "defaults", input: dash + `}`, want: commentQuery{target: dashboardTarget, status: threads.StatusOpen, limit: defaultListComments}},
		{name: "by target", input: byTarget + `}`, want: commentQuery{target: findingTarget, byTarget: true, status: threads.StatusOpen, limit: defaultListComments}},
		{name: "dashboard by target", input: `{"target":"dashboard:` + dashA + `"}`, want: commentQuery{target: dashboardTarget, byTarget: true, status: threads.StatusOpen, limit: defaultListComments}},
		{name: "both", input: `{"target":"finding:` + dashB + `","dashboard_id":"` + dashA + `"}`, wantMsg: "give exactly one of target and dashboard_id"},
		{name: "neither", input: `{}`, wantMsg: "give exactly one of target and dashboard_id"},
		{name: "bad target", input: `{"target":"saved_search:` + dashB + `"}`, wantMsg: "target kind must be dashboard, finding or annotation"},
		{name: "target without id", input: `{"target":"finding"}`, wantMsg: "target must be kind:uuid"},
		{name: "target with bad id", input: `{"target":"finding:nope"}`, wantMsg: "target id must be a UUID"},
		{name: "status kept", input: dash + `,"status":"all"}`, want: commentQuery{target: dashboardTarget, status: "all", limit: defaultListComments}},
		{name: "panel kept", input: dash + `,"panel_id":"p1","limit":7,"cursor":"abc"}`, want: commentQuery{target: dashboardTarget, status: threads.StatusOpen, panelID: "p1", limit: 7, cursor: "abc"}},
		{name: "empty panel", input: dash + `,"panel_id":""}`, wantMsg: "panel_id must be at least 1 character"},
		{name: "long panel", input: dash + `,"panel_id":"` + strings.Repeat("p", maxPanelFilterBytes+1) + `"}`, wantMsg: "panel_id must be at most 64 bytes"},
		{name: "limit one", input: dash + `,"limit":1}`, want: commentQuery{target: dashboardTarget, status: threads.StatusOpen, limit: 1}},
		{name: "limit max", input: dash + `,"limit":100}`, want: commentQuery{target: dashboardTarget, status: threads.StatusOpen, limit: maxListComments}},
		{name: "limit zero", input: dash + `,"limit":0}`, wantMsg: "limit must be between"},
		{name: "limit over", input: dash + `,"limit":101}`, wantMsg: "limit must be between"},
		{name: "cursor at limit", input: dash + `,"cursor":"` + strings.Repeat("c", maxNameBytes) + `"}`, want: commentQuery{target: dashboardTarget, status: threads.StatusOpen, limit: defaultListComments, cursor: strings.Repeat("c", maxNameBytes)}},
		{name: "cursor too long", input: dash + `,"cursor":"` + strings.Repeat("c", maxNameBytes+1) + `"}`, wantMsg: "cursor must be at most 256 bytes"},
		{name: "bad dashboard", input: `{"dashboard_id":"nope"}`, wantMsg: "dashboard_id must be a UUID"},
		{name: "unknown field", input: dash + `,"extra":1}`, wantMsg: "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, res, ok := parseCommentQuery([]byte(tt.input))
			if tt.wantMsg != "" {
				if ok || errorCode(t, res) != codeInvalidRequest || !strings.Contains(res.Content.(errorBody).Message, tt.wantMsg) {
					t.Fatalf("ok = %v res = %#v, want %q", ok, res.Content, tt.wantMsg)
				}
				return
			}
			if !ok {
				t.Fatalf("res = %#v", res.Content)
			}
			if got != tt.want {
				t.Fatalf("query = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCommentItem(t *testing.T) {
	created := time.Date(2026, 10, 19, 12, 30, 0, 0, time.FixedZone("x", 3600))
	edited := created.Add(time.Hour)
	version := 4
	tests := []struct {
		name          string
		thread        threads.Thread
		message       threads.Message
		wantParent    bool
		wantPanel     bool
		wantTruncated bool
		wantBodyLen   int
	}{
		{name: "whole dashboard root", thread: threads.Thread{ID: dashA, Version: &version}, message: threads.Message{ID: dashA, Body: "hi"}, wantBodyLen: 2},
		{name: "panel reply", thread: threads.Thread{ID: dashB, PanelID: "p1", PanelPresent: true, Version: &version}, message: threads.Message{ID: dashA, ThreadID: dashB, Body: "hi"}, wantParent: true, wantPanel: true, wantBodyLen: 2},
		{name: "body at the limit", thread: threads.Thread{ID: dashA, Version: &version}, message: threads.Message{ID: dashA, Body: strings.Repeat("a", maxCommentOutBytes)}, wantBodyLen: maxCommentOutBytes},
		{name: "body over the limit", thread: threads.Thread{ID: dashA, Version: &version}, message: threads.Message{ID: dashA, Body: strings.Repeat("a", maxCommentOutBytes+1)}, wantTruncated: true, wantBodyLen: maxCommentOutBytes + len("...")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.thread.Status = threads.StatusOpen
			tt.thread.ResolvedByVersion = &version
			tt.message.CreatedAt = created
			tt.message.Author = actor.Actor{Name: "Ada"}
			tt.message.AuthorEmail = "ada@example.com"
			out := commentItem(commentRef{tt.thread, tt.message})
			if (out.ParentID != nil) != tt.wantParent || (out.PanelID != nil) != tt.wantPanel || out.PanelPresent != tt.thread.PanelPresent {
				t.Fatalf("out = %+v", out)
			}
			if out.BodyTruncated != tt.wantTruncated || len(out.Body) != tt.wantBodyLen {
				t.Fatalf("truncated = %v body length = %d", out.BodyTruncated, len(out.Body))
			}
			if out.CreatedAt != "2026-10-19T11:30:00Z" || out.Author != "Ada" || out.EditedAt != nil || out.Target != "" || out.Version != &version {
				t.Fatalf("out = %+v", out)
			}
			if (out.ResolvedByVersion != nil) == tt.wantParent {
				t.Fatalf("resolved_by_version = %v on a message with parent %v", out.ResolvedByVersion, tt.wantParent)
			}
			body, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantParent == strings.Contains(string(body), `"parent_id":null`) || tt.wantPanel == strings.Contains(string(body), `"panel_id":null`) {
				t.Fatalf("json = %s", body)
			}
			if strings.Contains(string(body), "body_truncated") != tt.wantTruncated {
				t.Fatalf("json = %s", body)
			}
			if !strings.Contains(string(body), `"edited_at":null`) || !strings.Contains(string(body), `"mentions":[]`) {
				t.Fatalf("json = %s", body)
			}
		})
	}
	t.Run("edited with mentions", func(t *testing.T) {
		message := threads.Message{ID: dashA, EditedAt: &edited, Mentions: []threads.Mention{
			{UserID: mustUUID(t, dashB), Name: "Ada"},
			{Agent: threads.AgentObserve, Name: threads.AgentObserve},
		}}
		out := commentItem(commentRef{threads.Thread{ID: dashA}, message})
		want := []mentionOut{{Kind: "user", ID: dashB, Name: "Ada"}, {Kind: "agent", ID: "observe", Name: "observe"}}
		if out.EditedAt == nil || *out.EditedAt != "2026-10-19T12:30:00Z" || !slices.Equal(out.Mentions, want) {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("off a dashboard", func(t *testing.T) {
		thread := threads.Thread{ID: dashA, Target: threads.Target{Kind: threads.KindFinding, ID: mustUUID(t, dashB)}, PanelPresent: true}
		out := targetedCommentItem(commentRef{thread, threads.Message{ID: dashA}})
		body, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		if out.Target != "finding:"+dashB || !strings.Contains(string(body), `"version":null`) || !strings.Contains(string(body), `"panel_id":null`) {
			t.Fatalf("json = %s", body)
		}
	})
}

func TestCommentItemAuthorFallsBackToEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"Ada", "ada@example.com", "Ada"},
		{"", "ada@example.com", "ada@example.com"},
		{"", "", ""},
	}
	for _, tt := range tests {
		message := threads.Message{Author: actor.Actor{Name: tt.name}, AuthorEmail: tt.email}
		if got := commentItem(commentRef{message: message}).Author; got != tt.want {
			t.Errorf("author for %+v = %q, want %q", tt, got, tt.want)
		}
	}
}

func TestCommentFailure(t *testing.T) {
	internal := errors.New("database down")
	tests := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
		wantErr  error
	}{
		{name: "invalid input", err: &threads.InvalidInputError{Message: "body is empty"}, wantCode: codeInvalidRequest, wantMsg: "body is empty"},
		{name: "wrapped invalid input", err: fmt.Errorf("wrap: %w", &threads.InvalidInputError{Message: "body is empty"}), wantCode: codeInvalidRequest, wantMsg: "body is empty"},
		{name: "invalid cursor", err: dashboards.ErrInvalidCursor, wantCode: codeInvalidRequest, wantMsg: "cursor is not valid"},
		{name: "comment not found", err: threads.ErrNotFound, wantCode: codeNotFound, wantMsg: commentNotFoundNotice},
		{name: "target not found", err: threads.ErrTargetNotFound, wantCode: codeNotFound, wantMsg: commentNotFoundNotice},
		{name: "dashboard not found", err: dashboards.ErrNotFound, wantCode: codeNotFound},
		{name: "no ability", err: folders.ErrForbidden, wantCode: codeForbiddenRole, wantMsg: roleDeniedMessage},
		{name: "not the author", err: threads.ErrNotAuthor, wantCode: codeNotAuthor, wantMsg: threads.ErrNotAuthor.Error()},
		{name: "unknown panel", err: threads.ErrUnknownPanel, wantCode: codeUnknownPanel, wantMsg: threads.ErrUnknownPanel.Error()},
		{name: "unknown version", err: threads.ErrUnknownVersion, wantCode: codeUnknownVersion, wantMsg: threads.ErrUnknownVersion.Error()},
		{name: "internal", err: internal, wantErr: internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := commentFailure(tt.err)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || res.Content != nil {
					t.Fatalf("res = %#v err = %v", res, err)
				}
				return
			}
			if err != nil || errorCode(t, res) != tt.wantCode {
				t.Fatalf("res = %#v err = %v", res.Content, err)
			}
			if msg := res.Content.(errorBody).Message; tt.wantMsg != "" && msg != tt.wantMsg {
				t.Fatalf("message = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestVersionFailureNamesTheVersionField(t *testing.T) {
	res, err := versionFailure(&threads.InvalidInputError{Code: threads.CodeInvalidVersion, Message: "resolved_by_version applies only to a dashboard thread"})
	if err != nil || res.Content.(errorBody).Message != "version applies only to a dashboard thread" {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	res, err = versionFailure(&threads.InvalidInputError{Code: threads.CodeInvalidBody, Message: "body is empty"})
	if err != nil || res.Content.(errorBody).Message != "body is empty" {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
}

func worstCaseRef(t *testing.T) commentRef {
	version := 2147483647
	author := strings.Repeat("<", 256)
	mentions := make([]threads.Mention, threads.MaxMentions)
	for i := range mentions {
		mentions[i] = threads.Mention{UserID: mustUUID(t, dashA), Name: strings.Repeat("<", 256)}
	}
	edited := time.Date(2026, 10, 19, 12, 30, 0, 0, time.UTC)
	return commentRef{
		thread: threads.Thread{ID: dashB, PanelID: strings.Repeat("<", maxPanelFilterBytes), PanelPresent: true, Version: &version, Status: threads.StatusResolved, ResolvedByVersion: &version},
		message: threads.Message{
			ID:          dashA,
			ThreadID:    dashB,
			Author:      actor.Actor{Name: author},
			AuthorEmail: author,
			CreatedAt:   time.Date(2026, 10, 19, 12, 30, 0, 0, time.UTC),
			EditedAt:    &edited,
			Body:        strings.Repeat("<", 2000),
			Mentions:    mentions,
		},
	}
}

func TestWorstCaseCommentFitsTheListBudgetAtLimitOne(t *testing.T) {
	cursor := strings.Repeat("c", 256)
	q := commentQuery{target: threads.Target{Kind: threads.KindFinding, ID: mustUUID(t, dashA)}, byTarget: true}
	out, _, cut := commentPage(q, []commentRef{worstCaseRef(t)}, cursor)
	if cut || len(out.Comments) != 1 {
		t.Fatalf("cut = %v with %d comments", cut, len(out.Comments))
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > listBudget {
		t.Fatalf("worst-case list result is %d bytes, over the %d byte budget", len(body), listBudget)
	}
	if !out.Comments[0].BodyTruncated || !strings.HasSuffix(out.Comments[0].Body, "...") {
		t.Fatalf("the worst-case body was not cut: truncated = %v", out.Comments[0].BodyTruncated)
	}
}

func TestCommentPageShrinksToFewerComments(t *testing.T) {
	body := strings.Repeat("<", 1000)
	refs := make([]commentRef, 5)
	for i := range refs {
		refs[i] = commentRef{message: threads.Message{ID: dashA, Body: body}}
	}
	q := commentQuery{target: threads.Target{Kind: threads.KindDashboard, ID: mustUUID(t, dashA)}}
	out, kept, cut := commentPage(q, refs, "next")
	if !cut || kept < 1 || kept >= len(refs) || len(out.Comments) != len(refs) {
		t.Fatalf("kept = %d cut = %v", kept, cut)
	}
	again, kept2, cut2 := commentPage(q, refs[:kept], "next")
	encoded, err := json.Marshal(again)
	if cut2 || kept2 != kept || err != nil || len(encoded) > listBudget {
		t.Fatalf("second page kept = %d cut = %v size = %d err = %v", kept2, cut2, len(encoded), err)
	}
}

func TestCommentPageNamesWhatWasListed(t *testing.T) {
	dashboard := threads.Target{Kind: threads.KindDashboard, ID: mustUUID(t, dashA)}
	finding := threads.Target{Kind: threads.KindFinding, ID: mustUUID(t, dashA)}
	tests := []struct {
		name          string
		q             commentQuery
		wantTarget    string
		wantDashboard string
	}{
		{"dashboard by id", commentQuery{target: dashboard}, "", dashA},
		{"dashboard by target", commentQuery{target: dashboard, byTarget: true}, "dashboard:" + dashA, dashA},
		{"finding by target", commentQuery{target: finding, byTarget: true}, "finding:" + dashA, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, _ := commentPage(tt.q, nil, "")
			if out.Target != tt.wantTarget || out.DashboardID != tt.wantDashboard {
				t.Fatalf("out = %+v", out)
			}
			body, err := json.Marshal(out)
			if err != nil || !strings.Contains(string(body), `"comments":[]`) {
				t.Fatalf("json = %s err = %v", body, err)
			}
		})
	}
}

func (r *accessRig) privateFinding(t *testing.T, by auth.Principal) string {
	t.Helper()
	f, _, err := r.findings.Save(t.Context(), by, findings.SaveInput{Title: "Checkout is slow", Markdown: "the 5xx rate rose", From: time.Now().Add(-time.Hour), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}

func commentOutOf(t *testing.T, res Result) commentOut {
	t.Helper()
	switch out := res.Content.(type) {
	case commentResultOut:
		return out.Comment
	case commentChangedOut:
		return out.Comment
	}
	t.Fatalf("result = %s", textOf(t, res))
	return commentOut{}
}

func TestCommentToolsWorkOnAFinding(t *testing.T) {
	r := newAccessRig(t)
	target := "finding:" + r.privateFinding(t, adminCaller)

	opened := commentOutOf(t, r.call(t, adminCaller, nameCreateComment, `{"target":"`+target+`","body":"is it the cache? @observe"}`))
	if opened.Target != target || opened.Version != nil || opened.PanelID != nil || opened.ParentID != nil || opened.Status != threads.StatusOpen {
		t.Fatalf("created = %+v", opened)
	}
	if want := []mentionOut{{Kind: "agent", ID: "observe", Name: "observe"}}; !slices.Equal(opened.Mentions, want) {
		t.Fatalf("mentions = %+v", opened.Mentions)
	}

	reply := commentOutOf(t, r.call(t, adminCaller, nameReplyToComment, `{"comment_id":"`+opened.ID+`","body":"checked, it was a deploy"}`))
	if reply.ParentID == nil || *reply.ParentID != opened.ID || reply.Target != target {
		t.Fatalf("reply = %+v", reply)
	}

	listed, ok := r.call(t, adminCaller, nameListComments, `{"target":"`+target+`"}`).Content.(commentListOut)
	if !ok || listed.Target != target || listed.DashboardID != "" || len(listed.Comments) != 2 || listed.Comments[0].ID != opened.ID || listed.Comments[1].ID != reply.ID {
		t.Fatalf("list_comments = %+v", listed)
	}

	edited := r.call(t, adminCaller, nameEditComment, `{"comment_id":"`+reply.ID+`","body":"checked, it was a deploy of v2"}`)
	if out, ok := edited.Content.(commentChangedOut); !ok || !out.Changed || out.Comment.EditedAt == nil || out.Comment.Body != "checked, it was a deploy of v2" {
		t.Fatalf("edit_comment = %s", textOf(t, edited))
	}
	same := r.call(t, adminCaller, nameEditComment, `{"comment_id":"`+reply.ID+`","body":"checked, it was a deploy of v2"}`)
	if out, ok := same.Content.(commentChangedOut); !ok || out.Changed || same.Step != "Comment already had that text" {
		t.Fatalf("an unchanged edit = %s step %q", textOf(t, same), same.Step)
	}

	withVersion := r.call(t, adminCaller, nameResolveComment, `{"comment_id":"`+opened.ID+`","version":1}`)
	if errorCode(t, withVersion) != codeInvalidRequest || withVersion.Content.(errorBody).Message != "version applies only to a dashboard thread" {
		t.Fatalf("resolve with a version = %s", textOf(t, withVersion))
	}
	resolved := r.call(t, adminCaller, nameResolveComment, `{"comment_id":"`+reply.ID+`"}`)
	if out, ok := resolved.Content.(commentChangedOut); !ok || !out.Changed || out.Comment.ID != opened.ID || out.Comment.Status != threads.StatusResolved {
		t.Fatalf("resolve_comment by a reply id = %s", textOf(t, resolved))
	}
	if open := r.call(t, adminCaller, nameListComments, `{"target":"`+target+`"}`).Content.(commentListOut); len(open.Comments) != 0 {
		t.Fatalf("open comments after resolving = %+v", open)
	}
	reopened := r.call(t, adminCaller, nameReopenComment, `{"comment_id":"`+opened.ID+`"}`)
	if out, ok := reopened.Content.(commentChangedOut); !ok || !out.Changed || out.Comment.Status != threads.StatusOpen || reopened.Step != "Reopened a comment" {
		t.Fatalf("reopen_comment = %s step %q", textOf(t, reopened), reopened.Step)
	}
	again := r.call(t, adminCaller, nameReopenComment, `{"comment_id":"`+opened.ID+`"}`)
	if out, ok := again.Content.(commentChangedOut); !ok || out.Changed || again.Step != "Comment was already open" {
		t.Fatalf("reopen_comment on an open thread = %s step %q", textOf(t, again), again.Step)
	}
}

func TestCommentToolsOnAnAnnotation(t *testing.T) {
	r := newAccessRig(t)
	note, err := r.notes.Create(t.Context(), adminCaller, annotations.CreateInput{Text: "deploy v2"})
	if err != nil {
		t.Fatal(err)
	}
	target := "annotation:" + note.ID
	opened := commentOutOf(t, r.call(t, commenterCaller, nameCreateComment, `{"target":"`+target+`","body":"rolled back later"}`))
	if opened.Target != target || opened.Author != "Cam" {
		t.Fatalf("created = %+v", opened)
	}
	if res := r.call(t, viewerCaller, nameListComments, `{"target":"`+target+`"}`); res.IsError || len(res.Content.(commentListOut).Comments) != 1 {
		t.Fatalf("a viewer's list_comments = %s", textOf(t, res))
	}
	if res := r.call(t, adminCaller, nameEditComment, `{"comment_id":"`+opened.ID+`","body":"not mine"}`); errorCode(t, res) != codeNotAuthor {
		t.Fatalf("an edit by someone else = %s", textOf(t, res))
	}
}

func TestCommentToolsOnADashboardPanel(t *testing.T) {
	r := newAccessRig(t)
	dash := r.seed(t, openTitle).String()
	target := `"target":"dashboard:` + dash + `"`

	onPanel := commentOutOf(t, r.call(t, adminCaller, nameCreateComment, `{`+target+`,"body":"why flat?","panel_id":"p_up","version":1}`))
	if onPanel.PanelID == nil || *onPanel.PanelID != "p_up" || onPanel.Version == nil || *onPanel.Version != 1 || !onPanel.PanelPresent {
		t.Fatalf("panel comment = %+v", onPanel)
	}
	whole := commentOutOf(t, r.call(t, adminCaller, nameCreateComment, `{`+target+`,"body":"nice board"}`))
	if whole.PanelID != nil || whole.Version == nil {
		t.Fatalf("dashboard comment = %+v", whole)
	}

	byDashboard := r.call(t, adminCaller, nameListComments, `{"dashboard_id":"`+dash+`","panel_id":"p_up"}`).Content.(commentListOut)
	if byDashboard.Target != "" || byDashboard.DashboardID != dash || len(byDashboard.Comments) != 1 || byDashboard.Comments[0].Target != "" {
		t.Fatalf("list by dashboard_id = %+v", byDashboard)
	}

	for name, tt := range map[string]struct{ input, code string }{
		"unknown panel":   {`{` + target + `,"body":"x","panel_id":"nope"}`, codeUnknownPanel},
		"unknown version": {`{` + target + `,"body":"x","version":9}`, codeUnknownVersion},
		"bad version":     {`{` + target + `,"body":"x","version":0}`, codeInvalidRequest},
		"empty body":      {`{` + target + `,"body":""}`, codeInvalidRequest},
		"bad target":      {`{"target":"folder:` + dash + `","body":"x"}`, codeInvalidRequest},
	} {
		if res := r.call(t, adminCaller, nameCreateComment, tt.input); errorCode(t, res) != tt.code {
			t.Errorf("%s: %s, want %s", name, textOf(t, res), tt.code)
		}
	}
	if res := r.call(t, adminCaller, nameResolveComment, `{"comment_id":"`+onPanel.ID+`","version":9}`); errorCode(t, res) != codeUnknownVersion {
		t.Errorf("resolve with an unknown version = %s", textOf(t, res))
	}
	if res := r.call(t, adminCaller, nameResolveComment, `{"comment_id":"`+onPanel.ID+`","version":1}`); res.IsError || *res.Content.(commentChangedOut).Comment.ResolvedByVersion != 1 {
		t.Errorf("resolve with version 1 = %s", textOf(t, res))
	}
}

func TestCommentToolsHideAnUnfiledFindingFromOthers(t *testing.T) {
	r := newAccessRig(t)
	id := r.privateFinding(t, commenterCaller)
	target := "finding:" + id
	opened := commentOutOf(t, r.call(t, commenterCaller, nameCreateComment, `{"target":"`+target+`","body":"private note"}`))
	calls := map[string]string{
		nameListComments:   `{"target":"` + target + `"}`,
		nameCreateComment:  `{"target":"` + target + `","body":"hello"}`,
		nameReplyToComment: `{"comment_id":"` + opened.ID + `","body":"hello"}`,
		nameEditComment:    `{"comment_id":"` + opened.ID + `","body":"hello"}`,
		nameResolveComment: `{"comment_id":"` + opened.ID + `"}`,
		nameReopenComment:  `{"comment_id":"` + opened.ID + `"}`,
	}
	for tool, input := range calls {
		res := r.call(t, editorCaller, tool, input)
		if errorCode(t, res) != codeNotFound || strings.Contains(textOf(t, res), "private note") || strings.Contains(textOf(t, res), id) {
			t.Errorf("%s by someone else = %s", tool, textOf(t, res))
		}
	}
}

func TestListCommentsNeedsExactlyOneTarget(t *testing.T) {
	r := newAccessRig(t)
	dash := r.seed(t, openTitle).String()
	for name, input := range map[string]string{
		"neither": `{}`,
		"both":    `{"target":"dashboard:` + dash + `","dashboard_id":"` + dash + `"}`,
	} {
		res := r.call(t, adminCaller, nameListComments, input)
		if errorCode(t, res) != codeInvalidRequest || res.Content.(errorBody).Message != "give exactly one of target and dashboard_id" {
			t.Errorf("%s: %s", name, textOf(t, res))
		}
	}
}
