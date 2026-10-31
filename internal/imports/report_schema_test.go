package imports_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/pkg/grafana"
)

func ptr[T any](v T) *T { return &v }

func reportBody(t *testing.T, report grafana.Report) map[string]any {
	t.Helper()
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func validateReport(t *testing.T, body any) error {
	t.Helper()
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return doc.Components.Schemas["ImportReport"].Value.VisitJSON(body)
}

func emptyReport() grafana.Report {
	return grafana.Report{
		ImporterVersion: 1,
		Panels:          []grafana.PanelLine{},
		Notes:           []grafana.Note{},
	}
}

func TestReportSchemaAcceptsAReportWithoutBlocks(t *testing.T) {
	body := reportBody(t, emptyReport())
	if _, has := body["blocks"]; has {
		t.Fatalf("a report without blocks carries a blocks key: %v", body)
	}
	if err := validateReport(t, body); err != nil {
		t.Fatalf("the report breaks ImportReport: %v", err)
	}
}

func TestReportSchemaAcceptsNestedBlocks(t *testing.T) {
	report := emptyReport()
	report.Blocks = []grafana.Block{
		{
			Kind: grafana.BlockKind("dashboard"),
			Options: []grafana.OptionLine{
				{Path: "title", Outcome: grafana.Outcome("mapped"), Spec: []string{"title"}},
				{Path: "id", Outcome: grafana.Outcome("bookkeeping"), Spec: []string{}},
			},
			Implicit: 1,
			Blocks:   []grafana.Block{},
		},
		{
			Kind:       grafana.BlockKind("panel"),
			Path:       "panels[0]",
			Title:      "Requests",
			SourceType: "timeseries",
			PanelID:    ptr("p1"),
			Options:    []grafana.OptionLine{},
			Blocks: []grafana.Block{
				{
					Kind:       grafana.BlockKind("query"),
					Path:       "panels[0].targets[1]",
					Title:      "B",
					SourceType: "prometheus",
					Outcome:    ptr(grafana.Outcome("excluded")),
					Reason:     ptr("The query is hidden in Grafana."),
					Options:    []grafana.OptionLine{},
					Blocks:     []grafana.Block{},
				},
			},
		},
	}
	body := reportBody(t, report)
	if err := validateReport(t, body); err != nil {
		t.Fatalf("the report breaks ImportReport: %v", err)
	}
}

func TestReportSchemaRefusesValuesOutsideTheEnums(t *testing.T) {
	cases := []struct {
		name   string
		report grafana.Report
	}{
		{
			name: "a block kind that is not defined",
			report: func() grafana.Report {
				r := emptyReport()
				r.Blocks = []grafana.Block{{Kind: grafana.BlockKind("widget"), Options: []grafana.OptionLine{}, Blocks: []grafana.Block{}}}
				return r
			}(),
		},
		{
			name: "implicit as an outcome, because it names a group of outcomes",
			report: func() grafana.Report {
				r := emptyReport()
				r.Blocks = []grafana.Block{{
					Kind:    grafana.BlockKind("dashboard"),
					Options: []grafana.OptionLine{{Path: "id", Outcome: grafana.Outcome("implicit"), Spec: []string{}}},
					Blocks:  []grafana.Block{},
				}}
				return r
			}(),
		},
		{
			name: "a whole-block outcome that was imported",
			report: func() grafana.Report {
				r := emptyReport()
				r.Blocks = []grafana.Block{{
					Kind:    grafana.BlockKind("dashboard"),
					Outcome: ptr(grafana.Outcome("mapped")),
					Options: []grafana.OptionLine{},
					Blocks:  []grafana.Block{},
				}}
				return r
			}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateReport(t, reportBody(t, tc.report)); err == nil {
				t.Fatal("the schema accepted the report")
			}
		})
	}
}

func TestReportSchemaAcceptsTheReportOfAConvertedDashboard(t *testing.T) {
	body := []byte(`{"title":"Shop","uid":"abc","schemaVersion":39,"panels":[
		{"id":1,"type":"timeseries","title":"Requests","gridPos":{"x":0,"y":0,"w":12,"h":8},
		 "datasource":{"type":"prometheus","uid":"prom"},
		 "targets":[{"refId":"A","expr":"up","legendFormat":"{{pod}}"}],
		 "fieldConfig":{"defaults":{"unit":"short"},"overrides":[{"matcher":{"id":"byName","options":"up"},"properties":[{"id":"unit","value":"percent"}]}]}}]}`)
	res, err := grafana.Convert(body, grafana.Options{Datasources: []grafana.Datasource{{UID: "prom", Type: "prometheus"}}})
	if err != nil {
		t.Fatal(err)
	}
	decoded := reportBody(t, res.Report)
	if blocks, _ := decoded["blocks"].([]any); len(blocks) == 0 {
		t.Fatalf("the converted report has no blocks: %v", decoded)
	}
	if err := validateReport(t, decoded); err != nil {
		t.Fatalf("the report breaks ImportReport: %v", err)
	}
}

func TestReplaceNULInReportCleansEveryBlockText(t *testing.T) {
	report := emptyReport()
	report.Blocks = []grafana.Block{{
		Kind:  grafana.BlockKind("panel"),
		Path:  "panels[0]",
		Title: "a\x00b",
		Options: []grafana.OptionLine{
			{Path: "options.k\x00ey", Outcome: grafana.Outcome("reported"), Spec: []string{}, Reason: ptr("why\x00")},
		},
		Reason: ptr("block\x00reason"),
		Blocks: []grafana.Block{{
			Kind:    grafana.BlockKind("query"),
			Path:    "panels[0].targets[\x000]",
			Title:   "A\x00",
			Options: []grafana.OptionLine{{Path: "e\x00xpr", Outcome: grafana.Outcome("mapped"), Spec: []string{}}},
			Blocks:  []grafana.Block{},
		}},
	}}
	imports.ReplaceNULInReport(&report)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `\u0000`) {
		t.Fatalf("a NUL is left in the report: %s", raw)
	}
	if got := report.Blocks[0].Blocks[0].Title; got != "A\uFFFD" {
		t.Fatalf("nested block title = %q, want it cleaned", got)
	}
	if got := *report.Blocks[0].Options[0].Reason; got != "why\uFFFD" {
		t.Fatalf("option reason = %q, want it cleaned", got)
	}
}
