package tools

import (
	"testing"
)

func TestListToolsRejectFieldsTheirSchemaDoesNotDeclare(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input string
		field string
	}{
		{"metrics table", nameListMetrics, `{"datasource":"fake","table":"x"}`, "table"},
		{"metrics label", nameListMetrics, `{"datasource":"fake","label":"x"}`, "label"},
		{"metrics metric", nameListMetrics, `{"datasource":"fake","metric":"x"}`, "metric"},
		{"labels label", nameListLabels, `{"datasource":"fake","label":"x"}`, "label"},
		{"label values unknown", nameListLabelValues, `{"datasource":"fake","label":"a","bogus":1}`, "bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &fakeDS{}
			r := newTestRegistry(t, ds, nil)
			res, err := call(t, r, tt.tool, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != codeInvalidRequest {
				t.Fatalf("code = %q, want %q", got, codeInvalidRequest)
			}
			if got, want := res.Content.(errorBody).Message, `unknown field "`+tt.field+`"`; got != want {
				t.Fatalf("message = %q, want %q", got, want)
			}
			if len(ds.requests) != 0 {
				t.Fatalf("discovery ran for a rejected input: %#v", ds.requests)
			}
		})
	}
}

func TestListToolsPassTheirDeclaredFieldsToDiscovery(t *testing.T) {
	ds := &fakeDS{}
	r := newTestRegistry(t, ds, nil)
	for _, tt := range []struct{ tool, input string }{
		{nameListMetrics, `{"datasource":"fake","search":"http","limit":7}`},
		{nameListLabels, `{"datasource":"fake","metric":"m","table":"t","search":"s","limit":7}`},
		{nameListLabelValues, `{"datasource":"fake","label":"l","metric":"m","table":"t","search":"s","limit":7}`},
	} {
		res, err := call(t, r, tt.tool, tt.input)
		if err != nil || res.IsError {
			t.Fatalf("%s: res = %#v, err = %v", tt.tool, res, err)
		}
	}
	got := ds.requests
	if len(got) != 3 || got[0].Search != "http" || got[0].Limit != 8 ||
		got[1].Metric != "m" || got[1].Table != "t" ||
		got[2].Label != "l" || got[2].Search != "s" {
		t.Fatalf("discovery requests = %#v", got)
	}
}
