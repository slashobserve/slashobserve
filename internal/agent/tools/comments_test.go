package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const commentRejection = "comment writes are for MCP clients; the server answers and resolves threads after a run"

func commentCalls() map[string]string {
	return map[string]string{
		nameListComments:    `{"dashboard_id":"` + dashA + `"}`,
		nameListMentionable: `{"target":"dashboard:` + dashA + `"}`,
		nameCreateComment:   `{"target":"dashboard:` + dashA + `","body":"done"}`,
		nameEditComment:     `{"comment_id":"` + dashB + `","body":"done"}`,
		nameReopenComment:   `{"comment_id":"` + dashB + `"}`,
		nameReplyToComment:  `{"comment_id":"` + dashB + `","body":"done"}`,
		nameResolveComment:  `{"comment_id":"` + dashB + `","version":1}`,
	}
}

func TestCommentWritesRejectAgentRunsAndCommentReadsReachTheThreads(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
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
				if leftOutOfRuns[tool] == "" {
					if !errors.Is(err, errCommentsNotConfigured) {
						t.Fatalf("res = %#v err = %v, want the read to reach the threads", res, err)
					}
					return
				}
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

func TestToolsLeftOutOfRuns(t *testing.T) {
	var got []string
	registered := map[string]bool{}
	for _, tool := range New(Deps{}).List() {
		registered[tool.Name] = true
		if tool.MCPOnly {
			got = append(got, tool.Name)
		}
		if tool.MCPOnly != (leftOutOfRuns[tool.Name] != "") {
			t.Errorf("%s: MCPOnly %v, left out of runs for %q", tool.Name, tool.MCPOnly, leftOutOfRuns[tool.Name])
		}
	}
	if strings.Join(got, ",") != "apply_suggested_change,cancel_run,claim_run,create_comment,draft_query,duplicate_dashboard,edit_comment,finish_run,get_run_brief,make_dashboard,merge_proposal,reopen_comment,reply_to_comment,report_run,resolve_comment,restore_dashboard_version,retry_run,run_dashboard_health,save_run_answer,share_answer,show_panel,start_run" {
		t.Fatalf("tools left out of runs = %v", got)
	}
	for name, reason := range leftOutOfRuns {
		if !registered[name] {
			t.Errorf("leftOutOfRuns names %s, which is not a registered tool", name)
		}
		if reason == "" {
			t.Errorf("%s is left out of runs without a reason", name)
		}
	}
}

func TestToolsOfferedInInvestigationsAndChatQuestionsOnly(t *testing.T) {
	var got []string
	registered := map[string]bool{}
	for _, tool := range New(Deps{}).List() {
		registered[tool.Name] = true
		pinned := tool.OnlyIn == ModeInvestigate && !tool.AgentOnly
		if pinned {
			got = append(got, tool.Name)
		}
		if pinned != (contextReads[tool.Name] != "") || pinned != tool.AlsoInChats {
			t.Errorf("%s: offered in investigations and chat questions only %v, in chats %v, reason %q", tool.Name, pinned, tool.AlsoInChats, contextReads[tool.Name])
		}
	}
	if strings.Join(got, ",") != "call_context_tool,find_context_tools,list_context_sources" {
		t.Fatalf("tools offered in investigations and chat questions only = %v", got)
	}
	for name := range contextReads {
		if !registered[name] {
			t.Errorf("contextReads names %s, which is not a registered tool", name)
		}
	}
}

func TestRunsOfferEveryToolNotLeftOut(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		if tool.AgentOnly || tool.MCPOnly || tool.OnlyIn != "" {
			continue
		}
		if !tool.OfferedIn(ModeCreate) {
			t.Errorf("%s is neither offered in a create run nor left out of runs with a reason", tool.Name)
		}
	}
}

func TestCommentToolsNeedComments(t *testing.T) {
	r := New(Deps{})
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
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
	byTarget := `{"target":"observation:` + dashB + `"`
	dashboardTarget := threads.Target{Kind: threads.KindDashboard, ID: mustUUID(t, dashA)}
	findingTarget := threads.Target{Kind: threads.KindObservation, ID: mustUUID(t, dashB)}
	tests := []struct {
		name    string
		input   string
		want    commentQuery
		wantMsg string
	}{
		{name: "defaults", input: dash + `}`, want: commentQuery{target: dashboardTarget, status: threads.StatusOpen, limit: defaultListComments}},
		{name: "by target", input: byTarget + `}`, want: commentQuery{target: findingTarget, byTarget: true, status: threads.StatusOpen, limit: defaultListComments}},
		{name: "dashboard by target", input: `{"target":"dashboard:` + dashA + `"}`, want: commentQuery{target: dashboardTarget, byTarget: true, status: threads.StatusOpen, limit: defaultListComments}},
		{name: "both", input: `{"target":"observation:` + dashB + `","dashboard_id":"` + dashA + `"}`, wantMsg: "give exactly one of target and dashboard_id"},
		{name: "neither", input: `{}`, wantMsg: "give exactly one of target and dashboard_id"},
		{name: "bad target", input: `{"target":"saved_search:` + dashB + `"}`, wantMsg: "target kind must be dashboard, annotation"},
		{name: "target without id", input: `{"target":"observation"}`, wantMsg: "target must be kind:uuid"},
		{name: "target with bad id", input: `{"target":"observation:nope"}`, wantMsg: "target id must be a UUID"},
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
	t.Run("anchored to a block and quoting a message", func(t *testing.T) {
		message := threads.Message{ID: dashA, BlockID: dashB, ReplyTo: &threads.ReplyTo{
			ID:      dashB,
			Author:  actor.Actor{Name: "Grace", RunID: dashA},
			Excerpt: "p95 rose after the deploy",
		}}
		out := commentItem(commentRef{threads.Thread{ID: dashA}, message})
		body, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			`"block_id":"` + dashB + `"`,
			`"reply_to":{"id":"` + dashB + `","author":"Observe Agent for Grace","excerpt":"p95 rose after the deploy"}`,
		} {
			if !strings.Contains(string(body), want) {
				t.Errorf("json = %s, want %s", body, want)
			}
		}
	})
	t.Run("about the whole page and quoting nothing", func(t *testing.T) {
		body, err := json.Marshal(commentItem(commentRef{threads.Thread{ID: dashA}, threads.Message{ID: dashA}}))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `"block_id":null`) || !strings.Contains(string(body), `"reply_to":null`) {
			t.Fatalf("json = %s", body)
		}
	})
	t.Run("off a dashboard", func(t *testing.T) {
		thread := threads.Thread{ID: dashA, Target: threads.Target{Kind: threads.KindObservation, ID: mustUUID(t, dashB)}, PanelPresent: true}
		out := targetedCommentItem(commentRef{thread, threads.Message{ID: dashA}})
		body, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		if out.Target != "observation:"+dashB || !strings.Contains(string(body), `"version":null`) || !strings.Contains(string(body), `"panel_id":null`) {
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
		{name: "not the starter", err: threads.ErrNotStarter, wantCode: codeNotStarter, wantMsg: threads.ErrNotStarter.Error()},
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

func largestAnchor(t *testing.T) *anchor.Anchor {
	t.Helper()
	at := time.Date(2026, 11, 20, 23, 56, 0, 0, time.UTC)
	grow := func(n int) anchor.Anchor {
		return anchor.Anchor{Kind: anchor.LogLine, Pos: anchor.Pos{X: 0.123456789, Y: 0.987654321}, From: &at, ID: strings.Repeat("<", 40), Quote: strings.Repeat("<", n), Series: map[string]string{"k": strings.Repeat("<", 40)}}
	}
	n := 0
	for grow(n+1).Validate() == nil {
		n++
	}
	a := grow(n)
	encoded, err := json.Marshal(a)
	if err != nil || n == 0 || len(encoded) < 1500 {
		t.Fatalf("largest anchor is %d bytes with a %d character quote: %v", len(encoded), n, err)
	}
	return &a
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
		thread: threads.Thread{ID: dashB, PanelID: strings.Repeat("<", maxPanelFilterBytes), PanelPresent: true, Version: &version, Status: threads.StatusResolved, ResolvedByVersion: &version, Anchor: largestAnchor(t)},
		message: threads.Message{
			ID:          dashB,
			ThreadID:    dashB,
			Author:      actor.Actor{Name: author, Origin: origin.Origin{Kind: origin.MCP, Client: strings.Repeat("<", origin.MaxClientRunes)}},
			AuthorEmail: author,
			CreatedAt:   time.Date(2026, 10, 19, 12, 30, 0, 0, time.UTC),
			EditedAt:    &edited,
			Body:        strings.Repeat("<", 2000),
			Mentions:    mentions,
		},
	}
}

func worstCaseRunReplyRef(t *testing.T) commentRef {
	ref := worstCaseRef(t)
	ref.message.ID = dashA
	ref.message.Mentions = nil
	ref.message.Body = strings.Repeat("<", 520)
	ref.message.Answer = &threads.Answer{
		RunID:           dashA,
		FindingID:       dashB,
		Title:           strings.Repeat("<", 400),
		Markdown:        strings.Repeat("<", 4000),
		EvidencePanels:  4,
		SuggestedChange: strings.Repeat("<", maxSuggestedChangeRunes),
		ObservationID:   dashB,
	}
	return ref
}

func TestWorstCaseRunReplyWithItsAnswerFitsTheListBudget(t *testing.T) {
	builtIn := worstCaseRunReplyRef(t)
	builtIn.message.Author = actor.Actor{Name: builtIn.message.Author.Name, RunID: dashA}
	tests := []struct {
		name      string
		ref       commentRef
		mayCutRun bool
	}{
		{name: "built-in agent", ref: builtIn},
		{name: "connected agent with the longest client name", ref: worstCaseRunReplyRef(t), mayCutRun: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := commentQuery{target: threads.Target{Kind: threads.KindObservation, ID: mustUUID(t, dashA)}, byTarget: true}
			out, _, cut := commentPage(q, []commentRef{tt.ref}, strings.Repeat("c", 256))
			body, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			if cut || len(out.Comments) != 1 || len(body) > listBudget {
				t.Fatalf("cut %v, %d comments, %d bytes over the %d byte budget", cut, len(out.Comments), len(body), listBudget)
			}
			got := out.Comments[0]
			if got.Answer == nil || got.Answer.Markdown != commentItem(tt.ref).Answer.Markdown || got.BodyTruncated && !tt.mayCutRun {
				t.Fatalf("answer %+v, status line truncated %v", got.Answer, got.BodyTruncated)
			}
		})
	}
}

func TestWorstCaseCommentFitsTheListBudgetAtLimitOne(t *testing.T) {
	cursor := strings.Repeat("c", 256)
	q := commentQuery{target: threads.Target{Kind: threads.KindObservation, ID: mustUUID(t, dashA)}, byTarget: true}
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
	got := out.Comments[0]
	if got.Anchor == nil || len(got.Mentions) != threads.MaxMentions || got.Mentions[0].ID != dashA {
		t.Fatalf("the worst-case comment lost its anchor or its mention ids: anchor %v, mentions %+v", got.Anchor, got.Mentions)
	}
	if got.At != "" || got.Mentions[threads.MaxMentions-1].Name != "" || got.Actor.Client == nil || len(got.Body) <= len("...") {
		t.Fatalf("the worst case did not drop at and the mention names before cutting the body, with every field at its maximum: at %q, last mention %q, client %v, body %d bytes", got.At, got.Mentions[threads.MaxMentions-1].Name, got.Actor.Client, len(got.Body))
	}
}

func TestTheFirstCommentOfAThreadCarriesItsAnchorAsCreateCommentTakesIt(t *testing.T) {
	at := time.Date(2026, 11, 20, 23, 56, 0, 0, time.UTC)
	pinned := &anchor.Anchor{Kind: anchor.Point, Pos: anchor.Pos{X: 0.4, Y: 0.3}, From: &at, Series: map[string]string{"service": "checkout"}, Shown: "1.2s", Window: &anchor.Window{From: "now-6h", To: "now"}}
	thread := threads.Thread{ID: dashB, PanelID: "p1", Anchor: pinned}
	listed := func(thread threads.Thread, m threads.Message) (json.RawMessage, string) {
		t.Helper()
		body, err := json.Marshal(commentItem(commentRef{thread, m}))
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Anchor json.RawMessage `json:"anchor"`
			At     string          `json:"at"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out.Anchor, out.At
	}
	raw, words := listed(thread, threads.Message{ID: dashB, ThreadID: dashB})
	back, err := anchor.Parse(string(raw))
	if err != nil || !reflect.DeepEqual(&back, pinned) || words != pinned.Describe() {
		t.Fatalf("first comment: anchor %s parses back to %+v, %v; at %q", raw, back, err, words)
	}
	if raw, words := listed(thread, threads.Message{ID: dashA, ThreadID: dashB}); raw != nil || words != "" {
		t.Fatalf("a reply repeats the thread's anchor %s, at %q", raw, words)
	}
	if raw, words := listed(threads.Thread{ID: dashB}, threads.Message{ID: dashB}); raw != nil || words != "" {
		t.Fatalf("a thread pinned nowhere lists anchor %s, at %q", raw, words)
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
	finding := threads.Target{Kind: threads.KindObservation, ID: mustUUID(t, dashA)}
	tests := []struct {
		name          string
		q             commentQuery
		wantTarget    string
		wantDashboard string
	}{
		{"dashboard by id", commentQuery{target: dashboard}, "", dashA},
		{"dashboard by target", commentQuery{target: dashboard, byTarget: true}, "dashboard:" + dashA, dashA},
		{"finding by target", commentQuery{target: finding, byTarget: true}, "observation:" + dashA, ""},
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

func (r *accessRig) privateFinding(t *testing.T, by auth.Principal) findings.Finding {
	t.Helper()
	f, _, err := r.findings.Save(reading(t.Context()), by, findings.SaveInput{Title: "Checkout is slow", Markdown: "the 5xx rate rose", From: time.Now().Add(-time.Hour), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return f
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

func TestCommentToolsWorkOnAnObservation(t *testing.T) {
	r := newAccessRig(t)
	finding := r.privateFinding(t, adminCaller)
	target := "observation:" + finding.ObservationID

	opened := commentOutOf(t, r.call(t, adminCaller, nameCreateComment, `{"target":"observation:`+finding.ObservationID+`","body":"is it the cache? @observe"}`))
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

	listed, ok := r.call(t, adminCaller, nameListComments, `{"target":"observation:`+finding.ObservationID+`"}`).Content.(commentListOut)
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
	note, err := r.notes.Create(reading(t.Context()), adminCaller, annotations.CreateInput{Text: "deploy v2"})
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

func TestCommentToolsHideAFindingInAPersonalFolderFromOthers(t *testing.T) {
	r := newAccessRig(t)
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'cam', 'cam@example.com', 'Cam')`, commenterCaller.ID)
	id := r.privateFinding(t, commenterCaller).ObservationID
	target := "observation:" + id
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

func TestCommentToolsStartARunOnlyWhenAskedTo(t *testing.T) {
	r := newAccessRig(t)
	dash := r.seed(t, openTitle).String()
	target := `"target":"dashboard:` + dash + `"`
	var account string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO service_accounts (name, takes_runs) VALUES ('runner', true) RETURNING id::text`).Scan(&account); err != nil {
		t.Fatal(err)
	}
	token := "@[runner](service_account:" + account + ")"

	plain := r.call(t, adminCaller, nameCreateComment, `{`+target+`,"body":"`+token+` look"}`)
	if out, ok := plain.Content.(commentResultOut); !ok || out.Run != nil || out.RunRefusal != nil || plain.Step != "Opened a comment" {
		t.Fatalf("without to agent = %s, step %q", textOf(t, plain), plain.Step)
	}
	if strings.Contains(textOf(t, plain), `"run"`) || strings.Contains(textOf(t, plain), `"run_refusal"`) {
		t.Fatalf("a result without to agent names a run: %s", textOf(t, plain))
	}

	started := r.call(t, adminCaller, nameCreateComment, `{`+target+`,"body":"`+token+` look","to":"agent"}`)
	out, ok := started.Content.(commentResultOut)
	if !ok || out.Run == nil || out.Run.AssignedKind != "service_account" || out.Run.Status != "queued" || out.RunRefusal != nil || started.Step != "Opened a comment and started a run" {
		t.Fatalf("with to agent = %s, step %q", textOf(t, started), started.Step)
	}

	replied := r.call(t, adminCaller, nameReplyToComment, `{"comment_id":"`+out.Comment.ID+`","body":"`+token+` and again","to":"agent"}`)
	if again, ok := replied.Content.(commentResultOut); !ok || again.Run == nil || again.Run.ID == out.Run.ID || replied.Step != "Replied to a comment and started a run" {
		t.Fatalf("reply with to agent = %s, step %q", textOf(t, replied), replied.Step)
	}

	refused := r.call(t, adminCaller, nameCreateComment, `{`+target+`,"body":"@observe look","to":"agent"}`)
	if got, ok := refused.Content.(commentResultOut); !ok || got.Run != nil || got.RunRefusal == nil || got.RunRefusal.Code != threads.RefusalAgentUnavailable || refused.Step != "Opened a comment" {
		t.Fatalf("observe without a provider = %s, step %q", textOf(t, refused), refused.Step)
	}

	two := r.call(t, adminCaller, nameCreateComment, `{`+target+`,"body":"@observe `+token+`","to":"agent"}`)
	if errorCode(t, two) != codeInvalidRequest {
		t.Fatalf("two agents = %s", textOf(t, two))
	}
}

func TestAskingAnAgentNeedsTheMentionRuns(t *testing.T) {
	r := newAccessRig(t)
	registry := New(Deps{Threads: r.threads})
	ctx := auth.WithPrincipal(t.Context(), adminCaller)
	for tool, input := range map[string]string{
		nameCreateComment:  `{"target":"dashboard:` + dashA + `","body":"@observe hi","to":"agent"}`,
		nameReplyToComment: `{"comment_id":"` + dashB + `","body":"@observe hi","to":"agent"}`,
	} {
		if _, err := registry.Call(ctx, tool, []byte(input)); !errors.Is(err, errCommentsNotConfigured) {
			t.Errorf("%s: err = %v, want errCommentsNotConfigured", tool, err)
		}
	}
}

func TestCommentToolsAskAnAgentOnAnObservationThreadWithoutWritingOnThePage(t *testing.T) {
	r := newAccessRig(t)
	var account string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO service_accounts (name, takes_runs) VALUES ('runner', true) RETURNING id::text`).Scan(&account); err != nil {
		t.Fatal(err)
	}
	token := "@[runner](service_account:" + account + ")"
	observation := r.privateFinding(t, adminCaller).ObservationID
	blocks := func() int {
		var n int
		if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM observation_blocks WHERE observation_id = $1`, observation).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := blocks()

	created := r.call(t, adminCaller, nameCreateComment, `{"target":"observation:`+observation+`","body":"`+token+` what changed?","to":"agent"}`)
	out, ok := created.Content.(commentResultOut)
	if !ok || out.Run == nil || out.RunRefusal != nil {
		t.Fatalf("create = %s", textOf(t, created))
	}
	replied := r.call(t, adminCaller, nameReplyToComment, `{"comment_id":"`+out.Comment.ID+`","body":"`+token+` and before?","to":"agent"}`)
	if again, ok := replied.Content.(commentResultOut); !ok || again.Run == nil || again.RunRefusal != nil {
		t.Fatalf("reply = %s", textOf(t, replied))
	}
	var asks int
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM agent_requests WHERE intent = 'ask' AND observation_id IS NULL AND subject_observation_id = $1 AND thread_message_id IS NOT NULL`, observation).Scan(&asks); err != nil || asks != 2 {
		t.Fatalf("%d read-only asks about the observation, err %v, want 2", asks, err)
	}
	if after := blocks(); after != before {
		t.Fatalf("the observation has %d blocks after the asks, want %d", after, before)
	}
}

func TestCommentToolsPassTheBlockAndTheQuotedMessageToTheThreadsService(t *testing.T) {
	r := newAccessRig(t)
	finding := r.privateFinding(t, adminCaller)
	target := "observation:" + finding.ObservationID
	missing := "00000000-0000-4000-8000-000000000001"

	refused := r.call(t, adminCaller, nameCreateComment, `{"target":"`+target+`","body":"here","block_id":"`+missing+`"}`)
	if errorCode(t, refused) != codeInvalidRequest || refused.Content.(errorBody).Message != "block_id must be a block of this observation" {
		t.Fatalf("create_comment with an unknown block = %s", textOf(t, refused))
	}
	opened := commentOutOf(t, r.call(t, adminCaller, nameCreateComment, `{"target":"`+target+`","body":"is it the cache?"}`))

	refused = r.call(t, adminCaller, nameReplyToComment, `{"comment_id":"`+opened.ID+`","body":"here","block_id":"`+missing+`"}`)
	if errorCode(t, refused) != codeInvalidRequest || refused.Content.(errorBody).Message != "block_id must be a block of this observation" {
		t.Fatalf("reply_to_comment with an unknown block = %s", textOf(t, refused))
	}
	refused = r.call(t, adminCaller, nameReplyToComment, `{"comment_id":"`+opened.ID+`","body":"here","reply_to_id":"`+missing+`"}`)
	if errorCode(t, refused) != codeInvalidRequest || refused.Content.(errorBody).Message != "reply_to_id must be a message in this thread" {
		t.Fatalf("reply_to_comment with an unknown quoted message = %s", textOf(t, refused))
	}

	reply := commentOutOf(t, r.call(t, adminCaller, nameReplyToComment, `{"comment_id":"`+opened.ID+`","body":"checked","reply_to_id":"`+opened.ID+`"}`))
	if reply.ReplyTo == nil || reply.ReplyTo.ID != opened.ID || reply.ReplyTo.Excerpt != "is it the cache?" {
		t.Errorf("reply_to_comment returned reply_to %+v", reply.ReplyTo)
	}
	parsed, err := threads.ParseTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	page, err := r.threads.List(reading(t.Context()), adminCaller, threads.ListQuery{Target: parsed, Status: threads.StatusAll, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range page.Messages {
		if m.ID == reply.ID && (m.ReplyTo == nil || m.ReplyTo.ID != opened.ID || m.ReplyTo.Excerpt != "is it the cache?") {
			t.Errorf("the reply quotes %+v", m.ReplyTo)
		}
	}
}

func TestListMentionablePeopleNeedsTheCommentAbilityAndPagesWithMore(t *testing.T) {
	r := newAccessRig(t)
	note, err := r.notes.Create(reading(t.Context()), adminCaller, annotations.CreateInput{Text: "deploy v2"})
	if err != nil {
		t.Fatal(err)
	}
	target := "annotation:" + note.ID
	for _, p := range []auth.Principal{adminCaller, editorCaller, commenterCaller} {
		r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $2 || '@example.com', $3)`, p.ID, p.Name, p.Name)
	}
	if res := r.call(t, viewerCaller, nameListMentionable, `{"target":"`+target+`"}`); !res.IsError || errorCode(t, res) != "forbidden_role" {
		t.Fatalf("a viewer = %s", textOf(t, res))
	}
	res := r.call(t, commenterCaller, nameListMentionable, `{"target":"`+target+`","limit":1}`)
	out, ok := res.Content.(mentionableOut)
	if !ok || len(out.People) != 1 || !out.More || out.People[0].ID == "" || out.People[0].ID == commenterCaller.ID {
		t.Fatalf("limit 1 = %s", textOf(t, res))
	}
	if res := r.call(t, commenterCaller, nameListMentionable, `{"target":"nope"}`); !res.IsError {
		t.Fatalf("a bad target = %s", textOf(t, res))
	}
}

func TestCommentItemNamesTheRunThatWroteTheReply(t *testing.T) {
	const run = "33333333-3333-4333-8333-333333333333"
	version := 1
	thread := threads.Thread{ID: dashA, Version: &version}
	reply := commentItem(commentRef{thread, threads.Message{ID: dashA, Body: "x", Author: actor.Actor{Name: "Observe Agent", RunID: run}}})
	person := commentItem(commentRef{thread, threads.Message{ID: dashA, Body: "x", Author: actor.Actor{Name: "Ada"}}})
	if reply.RunID != run || person.RunID != "" {
		t.Fatalf("reply run %q, person run %q", reply.RunID, person.RunID)
	}
	raw, err := json.Marshal(person)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["run_id"]; ok {
		t.Fatalf("a person's comment carries a top-level run: %s", raw)
	}
}

func TestCommentItemCarriesTheSameActorAsTheRESTAPI(t *testing.T) {
	const run = "33333333-3333-4333-8333-333333333333"
	thread := threads.Thread{ID: dashA}
	for name, author := range map[string]actor.Actor{
		"agent reply": {Kind: auth.KindUser, ID: dashB, Name: "Ada", RunID: run},
		"person":      {Kind: auth.KindUser, ID: dashB, Name: "Ada"},
	} {
		t.Run(name, func(t *testing.T) {
			out := commentItem(commentRef{thread, threads.Message{ID: dashA, Author: author}})
			if !reflect.DeepEqual(out.Actor, author.JSON()) {
				t.Fatalf("actor = %+v, want %+v", out.Actor, author.JSON())
			}
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Actor map[string]any `json:"actor"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			runID, ok := got.Actor["run_id"]
			if !ok || (author.RunID == "") != (runID == nil) {
				t.Fatalf("actor = %v", got.Actor)
			}
		})
	}
}

func TestARunReplyListsItsAnswerCappedAndAPersonsMessageNone(t *testing.T) {
	long := strings.Repeat("word ", 200)
	reply := commentItem(commentRef{threads.Thread{ID: dashA}, threads.Message{ID: dashB, Body: "Answered.", Answer: &threads.Answer{
		RunID: dashA, FindingID: dashB, Title: "Checkout errors doubled", Markdown: long, EvidencePanels: 2,
	}}})
	a := reply.Answer
	if a == nil || a.RunID != dashA || a.FindingID != dashB || a.Title != "Checkout errors doubled" || a.EvidencePanels != 2 {
		t.Fatalf("answer = %+v", a)
	}
	if !a.MarkdownTruncated || len(a.Markdown) > maxAnswerOutBytes+len("...") || !strings.HasPrefix(long, strings.TrimSuffix(a.Markdown, "...")) {
		t.Fatalf("markdown %d bytes, truncated %v, want it cut at %d", len(a.Markdown), a.MarkdownTruncated, maxAnswerOutBytes)
	}
	if a.SuggestedChange != nil || a.ObservationID != nil {
		t.Fatalf("unset suggested_change and observation_id = %v, %v, want null", a.SuggestedChange, a.ObservationID)
	}
	body, err := json.Marshal(commentItem(commentRef{threads.Thread{ID: dashA}, threads.Message{ID: dashA, Body: "is this real?"}}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"answer"`) {
		t.Fatalf("a person's message lists an answer: %s", body)
	}
}
