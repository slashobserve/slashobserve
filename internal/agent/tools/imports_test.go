package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
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
