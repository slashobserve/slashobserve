package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/pkg/grafana"
)

func TestAttentionOf(t *testing.T) {
	report := grafana.Report{Panels: []grafana.PanelLine{
		{PanelID: "p1", Title: "Requests", Status: "imported"},
		{PanelID: "p2", Title: "Latency", Status: "partial", Reasons: []string{"first", "second"}},
		{PanelID: "p10", Title: "Requests by pod", Status: "placeholder", Reasons: []string{"Grafana panel type barchart is not supported yet."}},
		{PanelID: "p11", Title: "No reason", Status: "unverified"},
	}}
	want := []Attention{
		{PanelID: "p2", Title: "Latency", Status: "partial", Reason: "first"},
		{PanelID: "p10", Title: "Requests by pod", Status: "placeholder", Reason: "Grafana panel type barchart is not supported yet."},
		{PanelID: "p11", Title: "No reason", Status: "unverified"},
	}
	if got := attentionOf(report); !reflect.DeepEqual(got, want) {
		t.Fatalf("attention = %+v, want %+v", got, want)
	}
	if got := attentionOf(grafana.Report{}); got == nil || len(got) != 0 {
		t.Fatalf("empty report attention = %#v, want an empty non-nil list", got)
	}
}

func TestConversionAttentionDropsUnverified(t *testing.T) {
	report := grafana.Report{Panels: []grafana.PanelLine{
		{PanelID: "p1", Title: "Requests", Status: "imported"},
		{PanelID: "p2", Title: "Latency", Status: "partial", Reasons: []string{"first", "second"}},
		{PanelID: "p10", Title: "Requests by pod", Status: "placeholder", Reasons: []string{"Grafana panel type barchart is not supported yet."}},
		{PanelID: "p11", Title: "No reason", Status: "unverified"},
	}}
	want := []Attention{
		{PanelID: "p2", Title: "Latency", Status: "partial", Reason: "first"},
		{PanelID: "p10", Title: "Requests by pod", Status: "placeholder", Reason: "Grafana panel type barchart is not supported yet."},
	}
	if got := ConversionAttention(report); !reflect.DeepEqual(got, want) {
		t.Fatalf("conversion attention = %+v, want %+v", got, want)
	}
	if got := ConversionAttention(grafana.Report{}); got == nil || len(got) != 0 {
		t.Fatalf("empty report conversion attention = %#v, want an empty non-nil list", got)
	}
}

func TestUnresolvedRows(t *testing.T) {
	str := func(s string) *string { return &s }
	list := []grafana.SourceDatasource{
		{Key: "zzz", Name: str("Mimir"), Type: str("prometheus"), Kind: str("prometheus"), Panels: 3, Candidates: []string{"prom", "prom_tight"}},
		{Key: "matched", Kind: str("prometheus"), Match: str("prom"), Candidates: []string{"prom"}},
		{Key: "-- Grafana --", Candidates: []string{}},
	}
	want := []unresolvedRowOut{
		{Key: "zzz", Name: str("Mimir"), Type: str("prometheus"), Kind: str("prometheus"), Panels: 3, Candidates: []string{"prom", "prom_tight"}},
	}
	if got := unresolvedRows(list); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
}

func TestImportInAgentRunNeedsGrafanaJSONFromTheRequest(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
	dashboard := `{"title":"Checkout","panels":[]}`
	prompt := "Import this please: " + dashboard
	tests := []struct {
		name     string
		prompt   string
		json     string
		rejected bool
	}{
		{"json written by the model", "Build a dashboard for checkout", dashboard, true},
		{"empty json", prompt, "", true},
		{"json from the request", prompt, dashboard, false},
		{"json from the request with spaces around", prompt, "\n " + dashboard + " ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"json": tt.json})
			if err != nil {
				t.Fatal(err)
			}
			res, err := r.Call(WithRun(ctx, Run{Mode: ModeCreate, Prompt: tt.prompt}), nameImportGrafana, input)
			rejected := err == nil && res.IsError && errorCode(t, res) == codeInvalidRequest
			if rejected != tt.rejected {
				t.Fatalf("rejected = %v, want %v (res = %#v, err = %v)", rejected, tt.rejected, res, err)
			}
		})
	}
}

func TestImportedOutNamesTheChosenFolder(t *testing.T) {
	note := `Folder "Secret" is not available to you, so the dashboard went to "General".`
	tests := []struct {
		name       string
		folder     string
		folderNote string
	}{
		{"folder the dashboard was asked for", "Checkout", ""},
		{"default folder with a note", "General", note},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imported := imports.Imported{
				Dashboard:  dashboards.Dashboard{ID: dashA, Title: "Shop", Folder: folders.Folder{Title: tt.folder}},
				Version:    dashboards.Version{Number: 1},
				FolderNote: tt.folderNote,
			}
			body, err := json.Marshal(newImportedOut(&url.URL{Scheme: "https", Host: "so.example"}, imported))
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if got["folder"] != tt.folder {
				t.Errorf("folder = %v, want %q", got["folder"], tt.folder)
			}
			gotNote, hasNote := got["folder_note"]
			if (tt.folderNote != "") != hasNote || (hasNote && gotNote != tt.folderNote) {
				t.Errorf("folder_note = %v (present %t), want %q", gotNote, hasNote, tt.folderNote)
			}
		})
	}
}

func TestImportedOutStaysInsideTheResultCapWithALongFolderTitle(t *testing.T) {
	report := grafana.Report{}
	for i := range 400 {
		report.Panels = append(report.Panels, grafana.PanelLine{
			PanelID: fmt.Sprintf("p%d", i),
			Title:   strings.Repeat("t", 100),
			Status:  "partial",
			Reasons: []string{strings.Repeat("r", 200)},
		})
	}
	title := strings.Repeat("<", 100)
	imported := imports.Imported{
		Dashboard:  dashboards.Dashboard{ID: dashA, Title: strings.Repeat("&", 200), Folder: folders.Folder{Title: title}},
		FolderNote: `Folder "` + title + `" is not available to you, so the dashboard went to "` + title + `".`,
		Record:     imports.Record{Report: report},
	}
	out := newImportedOut(&url.URL{Scheme: "https", Host: "so.example"}, imported)
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > maxContentBytes {
		t.Fatalf("result is %d bytes, over %d", len(body), maxContentBytes)
	}
	if !out.Truncated || len(out.Attention) == 0 || len(out.Attention) >= 400 {
		t.Fatalf("attention rows = %d, truncated = %t, want a cut list", len(out.Attention), out.Truncated)
	}
}

func TestImportRefusesContradictoryOrMalformedUpdateInput(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
	tests := []struct {
		name  string
		input map[string]any
	}{
		{"update with import_as_new", map[string]any{"json": `{"title":"x"}`, "update_dashboard": dashA, "import_as_new": true}},
		{"update that is not an id", map[string]any{"json": `{"title":"x"}`, "update_dashboard": "checkout"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			res, err := r.Call(ctx, nameImportGrafana, input)
			if err != nil || !res.IsError || errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("res = %#v, err = %v, want invalid_request", res, err)
			}
		})
	}
}

func TestImportFailedNamesTheDashboardAnEarlierImportMade(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	r.deps.PublicURL = &url.URL{Scheme: "https", Host: "so.example"}
	tests := []struct {
		name      string
		canUpdate bool
		want      []string
	}{
		{"editable", true, []string{"update_dashboard argument to " + dashA, "import_as_new"}},
		{"read only", false, []string{"cannot edit it", "import_as_new"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := r.importFailed(t.Context(), &imports.AlreadyImportedError{Existing: imports.Existing{DashboardID: dashA, Title: "Shop", Version: 3, CanUpdate: tt.canUpdate}})
			if err != nil || !res.IsError {
				t.Fatalf("res = %#v, err = %v", res, err)
			}
			body, ok := res.Content.(alreadyImportedOut)
			if !ok || body.Code != codeAlreadyImported || body.Existing.DashboardID != dashA || body.Existing.Version != 3 || body.Existing.CanUpdate != tt.canUpdate || !strings.HasPrefix(body.Existing.URL, "https://so.example/") {
				t.Fatalf("body = %#v", res.Content)
			}
			for _, want := range tt.want {
				if !strings.Contains(body.Message, want) {
					t.Errorf("error %q lacks %q", body.Message, want)
				}
			}
		})
	}
	res, err := r.importFailed(t.Context(), imports.ErrNotFound)
	if body, ok := res.Content.(importErrorOut); err != nil || !ok || body.Code != codeNotImported {
		t.Fatalf("res = %#v, err = %v, want not_imported", res, err)
	}
}

func TestAnImportRunMayNotUpdateAnExistingDashboard(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
	dashboard := `{"title":"Checkout","panels":[]}`
	input, err := json.Marshal(map[string]any{"json": dashboard, "update_dashboard": dashA})
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Call(WithRun(ctx, Run{Mode: ModeCreate, Prompt: "Import this: " + dashboard}), nameImportGrafana, input)
	if err != nil || !res.IsError || errorCode(t, res) != codeInvalidRequest {
		t.Fatalf("res = %#v, err = %v, want invalid_request before any save", res, err)
	}
}

func TestAnImportRunDoesNotAskTheServerToRefuseAnImportedFile(t *testing.T) {
	in := importArgs{JSON: "{}"}
	for _, tt := range []struct {
		name   string
		in     importArgs
		scoped bool
		want   bool
	}{
		{"a connected client", in, false, true},
		{"a connected client importing as new", importArgs{JSON: "{}", ImportAsNew: true}, false, false},
		{"a run that creates a dashboard", in, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, _, ok := tt.in.request("", pgtype.UUID{}, tt.scoped)
			if !ok || req.RefuseExisting != tt.want {
				t.Fatalf("RefuseExisting = %v (ok %v), want %v", req.RefuseExisting, ok, tt.want)
			}
		})
	}
}

func TestImportFailedNamesBothUIDsOfAMismatchedUpdate(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := r.importFailed(t.Context(), &imports.UIDMismatchError{DashboardID: dashA, Held: "abc", Given: "zzz"})
	body, ok := res.Content.(importErrorOut)
	if err != nil || !ok || body.Code != codeGrafanaUIDMismatch || !strings.Contains(body.Message, `"abc"`) || !strings.Contains(body.Message, `"zzz"`) || !strings.Contains(body.Message, "new dashboard") {
		t.Fatalf("res = %#v, err = %v", res.Content, err)
	}
}

func TestImportContentRefusalRefusesWhatTheAgentCannotWrite(t *testing.T) {
	frame := `"gridPos":{"x":0,"y":0,"w":6,"h":4}`
	tests := []struct {
		name   string
		panel  string
		refuse bool
	}{
		{"html text panel", `{"id":1,"title":"T",` + frame + `,"type":"text","options":{"mode":"html","content":"<b>Hi</b>"}}`, true},
		{"unsupported panel kept as a placeholder", `{"id":1,"title":"T",` + frame + `,"type":"no-such-grafana-panel"}`, true},
		{"markdown text panel", `{"id":1,"title":"T",` + frame + `,"type":"text","options":{"mode":"markdown","content":"Hi"}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted, err := grafana.Convert([]byte(`{"title":"D","schemaVersion":39,"panels":[`+tt.panel+`]}`), grafana.Options{})
			if err != nil {
				t.Fatal(err)
			}
			res, refused := importContentRefusal(converted.Spec)
			if refused != tt.refuse {
				t.Fatalf("refused = %v, want %v (res = %#v)", refused, tt.refuse, res)
			}
			if body, ok := res.Content.(importErrorOut); refused && (!res.IsError || !ok || body.Code != codeImportContent) {
				t.Fatalf("res = %#v, want an error with code %q", res, codeImportContent)
			}
			if body, _ := res.Content.(importErrorOut); refused && (strings.Contains(body.Message, "observe import") || !strings.Contains(body.Message, "ask a person")) {
				t.Fatalf("refusal = %q, want it to send the agent to a person, not to another import path", body.Message)
			}
		})
	}
}
