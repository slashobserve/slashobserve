package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func TestAnnotationQueryBoundsTheInput(t *testing.T) {
	got, _, ok := annotationQuery(listAnnotationsInput{})
	if !ok || got.Limit != defaultListAnnotations || got.DashboardID.Valid || got.From != nil || got.To != nil || got.After != nil {
		t.Fatalf("defaults = %+v, ok %v", got, ok)
	}
	cursor := httpapi.EncodeTimeCursor(time.Unix(1700000000, 0), "0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61")
	got, _, ok = annotationQuery(listAnnotationsInput{
		DashboardID: "0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61", From: "2026-11-06T09:00:00Z", To: "2026-11-06T10:00:00+01:00",
		Tags: []string{"deploy", "deploy", "checkout"}, Limit: ptr(100), Cursor: cursor,
	})
	if !ok || got.Limit != 100 || !got.DashboardID.Valid || got.From == nil || got.To == nil || got.After == nil || strings.Join(got.Tags, ",") != "deploy,checkout" {
		t.Fatalf("full input = %+v, ok %v", got, ok)
	}
	cases := []struct {
		name    string
		in      listAnnotationsInput
		message string
	}{
		{"limit 0", listAnnotationsInput{Limit: ptr(0)}, "limit must be between 1 and 100"},
		{"limit 101", listAnnotationsInput{Limit: ptr(101)}, "limit must be between 1 and 100"},
		{"dashboard not a uuid", listAnnotationsInput{DashboardID: "shop"}, "dashboard_id must be a UUID from list_dashboards or create_dashboard"},
		{"from not RFC 3339", listAnnotationsInput{From: "now-1h"}, "from must be an RFC 3339 time"},
		{"to not RFC 3339", listAnnotationsInput{To: "1700000000"}, "to must be an RFC 3339 time"},
		{"to before from", listAnnotationsInput{From: "2026-11-06T10:00:00Z", To: "2026-11-06T09:00:00Z"}, "to must not be before from"},
		{"bad tag", listAnnotationsInput{Tags: []string{"Deploy"}}, annotations.MessageInvalidTags},
		{"bad cursor", listAnnotationsInput{Cursor: "not a cursor"}, "cursor is not valid"},
	}
	for _, c := range cases {
		_, res, ok := annotationQuery(c.in)
		body, _ := res.Content.(errorBody)
		if ok || body.Code != codeInvalidRequest || body.Message != c.message {
			t.Errorf("%s: ok %v, %+v", c.name, ok, body)
		}
	}
}

func TestAnnotationOfFormatsTimesAndLeavesOutEmptyFields(t *testing.T) {
	from := time.Date(2026, 11, 6, 10, 0, 0, 0, time.FixedZone("CET", 3600))
	raw, err := json.Marshal(annotationOf(annotations.Annotation{ID: "a1", From: from, Text: "deploy v2"}))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":"a1","time_from":"2026-11-06T09:00:00Z","text":"deploy v2","tags":[]}`; string(raw) != want {
		t.Errorf("global marker = %s\nwant %s", raw, want)
	}
	to := from.Add(time.Hour)
	got := annotationOf(annotations.Annotation{ID: "a2", DashboardID: "d1", PanelID: "p1", From: from, To: &to, Text: "load test", Tags: []string{"test"}, CreatedByName: "ci"})
	if got.DashboardID != "d1" || got.PanelID != "p1" || got.TimeTo != "2026-11-06T10:00:00Z" || got.CreatedByName != "ci" || got.Tags[0] != "test" {
		t.Errorf("range = %+v", got)
	}
}

func TestAnnotationOfLabelsTheCreatorWhenItIsKnown(t *testing.T) {
	from := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	known := annotationOf(annotations.Annotation{
		ID: "a1", From: from, Text: "deploy", CreatedBy: "11111111-1111-4111-8111-111111111111",
		CreatedByKind: "service_account", CreatedByName: "deploy-bot", Origin: origin.Origin{Kind: origin.MCP, Client: "claude-code"},
	})
	if known.CreatedByLabel != "deploy-bot (via Claude Code)" || known.CreatedByName != "deploy-bot" {
		t.Errorf("known creator = %+v", known)
	}
	gone := annotationOf(annotations.Annotation{ID: "a2", From: from, Text: "deploy", CreatedByKind: "service_account"})
	raw, err := json.Marshal(gone)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "created_by_label") {
		t.Errorf("a creator that is gone still has a label: %s", raw)
	}
}

func TestAnnotationPageCarriesTheCursor(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	at := time.Unix(1700000000, 0)
	out := annotationPage(annotations.Page{Annotations: []annotations.Annotation{{ID: "a1", From: at}}, Next: &httpapi.TimeCursor{Time: at, ID: id}})
	if len(out.Annotations) != 1 || out.NextCursor == nil {
		t.Fatalf("page = %+v", out)
	}
	if back, ok := httpapi.DecodeTimeCursor(*out.NextCursor); !ok || back.ID != id || !back.Time.Equal(at) {
		t.Errorf("cursor decodes to %+v, ok %v", back, ok)
	}
	if end := annotationPage(annotations.Page{}); end.NextCursor != nil || end.Annotations == nil {
		t.Errorf("last page = %+v", end)
	}
}

func TestHealthAndAnnotationToolsAreOfferedOnlyWhereTheyBelong(t *testing.T) {
	byName := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		byName[tool.Name] = tool
	}
	cases := []struct {
		tool        string
		build, look bool
	}{
		{nameListDashboardHealth, false, false},
		{nameCreateAnnotation, false, false},
		{nameListAnnotations, false, true},
	}
	for _, c := range cases {
		tool := byName[c.tool]
		for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest} {
			if tool.OfferedIn(mode) != c.build {
				t.Errorf("%s offered in %s = %t", c.tool, mode, !c.build)
			}
		}
		if tool.OfferedIn(ModeInvestigate) != c.look {
			t.Errorf("%s offered in investigate = %t", c.tool, !c.look)
		}
	}
	if tool := byName[nameListAnnotations]; !tool.ReadOnly || tool.MCPOnly || tool.AgentOnly {
		t.Errorf("list_annotations flags = %+v", tool)
	}
}

func TestListAnnotationsIsRefusedOutsideInvestigateRuns(t *testing.T) {
	r := New(Deps{})
	editor := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: editorCaller.ID, Role: auth.RoleEditor})
	for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest} {
		res, err := r.Call(WithRun(editor, Run{Mode: mode}), nameListAnnotations, []byte(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := res.Content.(errorBody)
		if !res.IsError || body.Code != codeNotInThisRun || body.Message != "list_annotations is only for investigate runs" {
			t.Errorf("%s run: %#v", mode, res.Content)
		}
	}
}

func annotationRows(t *testing.T, r *accessRig) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM annotations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateAnnotationRefusesAGlobalAnnotationBelowEditor(t *testing.T) {
	r := newAccessRig(t)
	viewerKey := auth.Principal{Kind: auth.KindServiceAccount, ID: "0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61", Name: "viewer-key", Role: auth.RoleViewer}
	vera := r.person(t, "vera", auth.RoleViewer)
	for _, by := range []auth.Principal{viewerKey, vera} {
		res := r.call(t, by, nameCreateAnnotation, `{"text":"deploy v2"}`)
		if code := errorCode(t, res); code != codeForbiddenRole {
			t.Errorf("%s (%s): %s", by.Name, by.Kind, textOf(t, res))
		}
	}
	if n := annotationRows(t, r); n != 0 {
		t.Fatalf("%d annotations written, want 0", n)
	}
}

func TestCreateAnnotationOnADashboardFollowsFolderAccess(t *testing.T) {
	r := newAccessRig(t)
	vera := r.person(t, "vera", auth.RoleViewer)
	mine := r.createdSaved(t, r.create(t, vera, "Mine", pgtype.UUID{}))
	shared := r.seed(t, "Shared").String()

	if res := r.call(t, vera, nameCreateAnnotation, `{"text":"mine","dashboard_id":"`+mine+`"}`); res.IsError {
		t.Fatalf("annotating her own dashboard: %s", textOf(t, res))
	}
	res := r.call(t, vera, nameCreateAnnotation, `{"text":"shared","dashboard_id":"`+shared+`"}`)
	if code := errorCode(t, res); code != codeForbiddenRole {
		t.Fatalf("annotating a General dashboard: %s", textOf(t, res))
	}
	if n := annotationRows(t, r); n != 1 {
		t.Fatalf("%d annotations written, want 1", n)
	}
}

func TestCreateAnnotationIsRefusedInsideARun(t *testing.T) {
	r := New(Deps{})
	editor := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: editorCaller.ID, Role: auth.RoleEditor})
	for _, mode := range []Mode{ModeCreate, ModeInvestigate} {
		res, err := r.Call(WithRun(editor, Run{Mode: mode}), nameCreateAnnotation, []byte(`{"text":"deploy v2"}`))
		if err != nil {
			t.Fatal(err)
		}
		if body, _ := res.Content.(errorBody); !res.IsError || body.Code != codeInvalidRequest || !strings.Contains(body.Message, "for MCP clients") {
			t.Errorf("%s run: %#v", mode, res.Content)
		}
	}
}

func TestAnnotationFailureMapsServiceErrors(t *testing.T) {
	cases := []struct {
		err     error
		code    string
		message string
	}{
		{&annotations.InvalidError{Code: "invalid_text", Message: "text must be 1 to 2000 characters"}, codeInvalidRequest, "text must be 1 to 2000 characters"},
		{annotations.ErrNotFound, codeNotFound, "dashboard not found"},
		{annotations.ErrForbidden, codeForbiddenRole, roleDeniedMessage},
	}
	for _, c := range cases {
		res, err := annotationFailure(c.err)
		body, _ := res.Content.(errorBody)
		if err != nil || !res.IsError || body.Code != c.code || body.Message != c.message {
			t.Errorf("%v: %#v, %v", c.err, res.Content, err)
		}
	}
	boom := errors.New("boom")
	if _, err := annotationFailure(boom); !errors.Is(err, boom) {
		t.Errorf("an unexpected error became %v", err)
	}
}
