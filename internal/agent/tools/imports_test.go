package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

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
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
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
