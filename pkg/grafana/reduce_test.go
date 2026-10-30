package grafana

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

var reducePanelTypes = []string{"stat", "gauge", "bargauge", "piechart"}

func reducePanel(t *testing.T, typ, reduceOptions string) (spec.Display, PanelLine) {
	t.Helper()
	panel, line := convertedPanel(t, panelDoc(typ, upTarget+`,"options":{"reduceOptions":`+reduceOptions+`}`))
	if panel.Display == nil {
		return spec.Display{}, line
	}
	return *panel.Display, line
}

func TestReduceOptionsImportOnEveryReducingPanel(t *testing.T) {
	for _, typ := range reducePanelTypes {
		t.Run(typ, func(t *testing.T) {
			d, line := reducePanel(t, typ, `{"values":true,"limit":10,"fields":"/^cpu/","calcs":["mean"]}`)
			if d.AllValues == nil || !*d.AllValues || d.Limit != 10 || d.Fields != "/^cpu/" || d.Reduce != "mean" {
				t.Fatalf("display = %+v", d)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
			}
		})
	}
}

func TestReduceOptionsAtGrafanaDefaultsWriteNothing(t *testing.T) {
	cases := []struct{ name, options string }{
		{"values false", `{"values":false}`},
		{"empty fields", `{"fields":""}`},
		{"limit 5000", `{"values":true,"limit":5000}`},
		{"all defaults", `{"values":false,"fields":"","calcs":["lastNotNull"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := reducePanel(t, "stat", tc.options)
			if d.Limit != 0 || d.Fields != "" || d.Reduce != "" {
				t.Fatalf("display = %+v", d)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestReduceFieldsKeepTheNameOrPattern(t *testing.T) {
	cases := []struct{ name, fields string }{
		{"a field name", `Value`},
		{"every field", `/.*/`},
		{"a pattern with flags", `/^cpu_/i`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := reducePanel(t, "bargauge", `{"fields":"`+tc.fields+`"}`)
			if d.Fields != tc.fields || line.Status != statusImported {
				t.Fatalf("fields = %q, status %s, want %q imported", d.Fields, line.Status, tc.fields)
			}
		})
	}
}

func TestOnlyTheFirstReducerIsKeptAndNothingIsReported(t *testing.T) {
	d, line := reducePanel(t, "gauge", `{"calcs":["min","max"]}`)
	if d.Reduce != "min" {
		t.Fatalf("reduce = %q, want min", d.Reduce)
	}
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestInvalidRowLimitsAndLongFieldNamesAreReported(t *testing.T) {
	cases := []struct {
		name, options, path string
	}{
		{"limit zero", `{"values":true,"limit":0}`, "options.reduceOptions.limit"},
		{"limit above 5000", `{"values":true,"limit":5001}`, "options.reduceOptions.limit"},
		{"fractional limit", `{"values":true,"limit":2.5}`, "options.reduceOptions.limit"},
		{"fields over 200 characters", `{"fields":"` + strings.Repeat("a", 201) + `"}`, "options.reduceOptions.fields"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := reducePanel(t, "piechart", tc.options)
			if line.Status != statusPartial || !slices.Equal(line.Paths, []string{tc.path}) {
				t.Fatalf("status = %s, paths %v, want partial at %s", line.Status, line.Paths, tc.path)
			}
			if d.Limit != 0 || d.Fields != "" {
				t.Fatalf("display = %+v, want the option left out", d)
			}
		})
	}
}
