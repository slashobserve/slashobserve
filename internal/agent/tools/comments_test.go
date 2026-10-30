package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
)

const commentRejection = "comment tools are for MCP clients; the server answers and resolves threads after a run"

func commentCalls() map[string]string {
	return map[string]string{
		nameListComments:   `{"dashboard_id":"` + dashA + `"}`,
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
				if res.Step != "Rejected "+tool+" input: invalid_request" {
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
	if strings.Join(got, ",") != "get_spec_guide,list_comments,list_folders,reply_to_comment,resolve_comment" {
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
	tests := []struct {
		name    string
		input   string
		want    commentQuery
		wantMsg string
	}{
		{name: "defaults", input: dash + `}`, want: commentQuery{status: dashboards.CommentOpen, limit: defaultListComments}},
		{name: "status kept", input: dash + `,"status":"all"}`, want: commentQuery{status: "all", limit: defaultListComments}},
		{name: "panel kept", input: dash + `,"panel_id":"p1","limit":7,"cursor":"abc"}`, want: commentQuery{status: dashboards.CommentOpen, panelID: "p1", limit: 7, cursor: "abc"}},
		{name: "empty panel", input: dash + `,"panel_id":""}`, wantMsg: "panel_id must be at least 1 character"},
		{name: "long panel", input: dash + `,"panel_id":"` + strings.Repeat("p", maxPanelFilterBytes+1) + `"}`, wantMsg: "panel_id must be at most 64 bytes"},
		{name: "limit one", input: dash + `,"limit":1}`, want: commentQuery{status: dashboards.CommentOpen, limit: 1}},
		{name: "limit max", input: dash + `,"limit":100}`, want: commentQuery{status: dashboards.CommentOpen, limit: maxListComments}},
		{name: "limit zero", input: dash + `,"limit":0}`, wantMsg: "limit must be between"},
		{name: "limit over", input: dash + `,"limit":101}`, wantMsg: "limit must be between"},
		{name: "cursor at limit", input: dash + `,"cursor":"` + strings.Repeat("c", maxNameBytes) + `"}`, want: commentQuery{status: dashboards.CommentOpen, limit: defaultListComments, cursor: strings.Repeat("c", maxNameBytes)}},
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
			tt.want.dashboardID = mustUUID(t, dashA)
			if got != tt.want {
				t.Fatalf("query = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCommentItem(t *testing.T) {
	created := time.Date(2026, 10, 19, 12, 30, 0, 0, time.FixedZone("x", 3600))
	version := 4
	tests := []struct {
		name          string
		in            dashboards.Comment
		wantParent    bool
		wantPanel     bool
		wantTruncated bool
		wantBodyLen   int
	}{
		{name: "whole dashboard root", in: dashboards.Comment{Body: "hi"}, wantBodyLen: 2},
		{name: "panel reply", in: dashboards.Comment{ParentID: dashB, PanelID: "p1", PanelPresent: true, Body: "hi"}, wantParent: true, wantPanel: true, wantBodyLen: 2},
		{name: "body at the limit", in: dashboards.Comment{Body: strings.Repeat("a", maxCommentOutBytes)}, wantBodyLen: maxCommentOutBytes},
		{name: "body over the limit", in: dashboards.Comment{Body: strings.Repeat("a", maxCommentOutBytes+1)}, wantTruncated: true, wantBodyLen: maxCommentOutBytes + len("...")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.ID = dashA
			tt.in.CreatedAt = created
			tt.in.ResolvedByVersion = &version
			tt.in.Author = dashboards.Author{Name: "Ada", Email: "ada@example.com"}
			out := commentItem(tt.in)
			if (out.ParentID != nil) != tt.wantParent || (out.PanelID != nil) != tt.wantPanel || out.PanelPresent != tt.in.PanelPresent {
				t.Fatalf("out = %+v", out)
			}
			if out.BodyTruncated != tt.wantTruncated || len(out.Body) != tt.wantBodyLen {
				t.Fatalf("truncated = %v body length = %d", out.BodyTruncated, len(out.Body))
			}
			if out.CreatedAt != "2026-10-19T11:30:00Z" || out.Author != "Ada" || out.ResolvedByVersion != &version {
				t.Fatalf("out = %+v", out)
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
		})
	}
}

func TestCommentItemAuthorFallsBackToEmail(t *testing.T) {
	tests := []struct {
		author dashboards.Author
		want   string
	}{
		{dashboards.Author{Name: "Ada", Email: "ada@example.com"}, "Ada"},
		{dashboards.Author{Email: "ada@example.com"}, "ada@example.com"},
		{dashboards.Author{}, ""},
	}
	for _, tt := range tests {
		if got := commentItem(dashboards.Comment{Author: tt.author}).Author; got != tt.want {
			t.Errorf("author for %+v = %q, want %q", tt.author, got, tt.want)
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
		{name: "invalid input", err: &dashboards.InvalidInputError{Message: "body is empty"}, wantCode: codeInvalidRequest, wantMsg: "body is empty"},
		{name: "wrapped invalid input", err: fmt.Errorf("wrap: %w", &dashboards.InvalidInputError{Message: "body is empty"}), wantCode: codeInvalidRequest, wantMsg: "body is empty"},
		{name: "invalid cursor", err: dashboards.ErrInvalidCursor, wantCode: codeInvalidRequest, wantMsg: "cursor is not valid"},
		{name: "comment not found", err: dashboards.ErrCommentNotFound, wantCode: codeNotFound},
		{name: "dashboard not found", err: dashboards.ErrNotFound, wantCode: codeNotFound},
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

func TestWorstCaseCommentFitsTheListBudgetAtLimitOne(t *testing.T) {
	version := 2147483647
	author := strings.Repeat("<", 256)
	comment := dashboards.Comment{
		ID:                dashA,
		ParentID:          dashB,
		PanelID:           strings.Repeat("<", maxPanelFilterBytes),
		PanelPresent:      true,
		Version:           version,
		Status:            dashboards.CommentResolved,
		Author:            dashboards.Author{Name: author, Email: author},
		CreatedAt:         time.Date(2026, 10, 19, 12, 30, 0, 0, time.UTC),
		Body:              strings.Repeat("<", 2000),
		ResolvedByVersion: &version,
	}
	cursor := strings.Repeat("c", 256)
	out, _, cut := commentPage(dashA, []dashboards.Comment{comment}, cursor)
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
	list := make([]dashboards.Comment, 5)
	for i := range list {
		list[i] = dashboards.Comment{ID: dashA, Body: body}
	}
	out, kept, cut := commentPage(dashA, list, "next")
	if !cut || kept < 1 || kept >= len(list) || len(out.Comments) != len(list) {
		t.Fatalf("kept = %d cut = %v", kept, cut)
	}
	again, kept2, cut2 := commentPage(dashA, list[:kept], "next")
	encoded, err := json.Marshal(again)
	if cut2 || kept2 != kept || err != nil || len(encoded) > listBudget {
		t.Fatalf("second page kept = %d cut = %v size = %d err = %v", kept2, cut2, len(encoded), err)
	}
}
