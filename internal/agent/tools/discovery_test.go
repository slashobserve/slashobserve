package tools

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
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

func TestListToolsReportABusyUpstreamAsTooManyRequests(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: &fakeDS{}})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(reg, logger, queryproxy.WithUpstreamWait(30*time.Millisecond))
	r := New(Deps{Datasources: reg, Runner: runner, Logger: logger})
	for range 32 {
		release, err := runner.AcquireUpstream(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
	}

	res, err := call(t, r, nameListMetrics, `{"datasource":"fake"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != "too_many_requests" {
		t.Fatalf("code = %q", got)
	}
	if got, want := res.Content.(errorBody).Message, "the server is making too many datasource calls right now; try again shortly"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if want := "Listing metrics on fake: too_many_requests"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}
