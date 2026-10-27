package tools

import (
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/grafana"
)

func TestAttentionOf(t *testing.T) {
	report := grafana.Report{Panels: []grafana.PanelLine{
		{PanelID: "p1", Title: "Requests", Status: "imported"},
		{PanelID: "p2", Title: "Latency", Status: "partial", Reasons: []string{"first", "second"}},
		{PanelID: "p10", Title: "Requests by pod", Status: "placeholder", Reasons: []string{"Grafana panel type barchart is not supported yet."}},
		{PanelID: "p11", Title: "No reason", Status: "unverified"},
	}}
	want := []attentionOut{
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
