package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

type compareFakeDS struct {
	*traceFakeDS
	attributesFn func(ctx context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error)
	attributes   []datasource.TraceAttributesRequest
}

func (d *compareFakeDS) TraceAttributes(ctx context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
	d.mu.Lock()
	d.attributes = append(d.attributes, req)
	d.mu.Unlock()
	return d.attributesFn(ctx, req)
}

func compareSets(ids []string, pairs func(i int) []datasource.TraceAttribute) []datasource.TraceAttributeSet {
	sets := make([]datasource.TraceAttributeSet, len(ids))
	for i, id := range ids {
		sets[i] = datasource.TraceAttributeSet{ID: id, Attributes: pairs(i)}
	}
	return sets
}

func newCompareRig(t *testing.T) (*Registry, *compareFakeDS) {
	t.Helper()
	r, traceDS := newTraceRig(t)
	ds := &compareFakeDS{traceFakeDS: traceDS}
	ds.attributesFn = func(_ context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
		return compareSets(req.IDs, func(int) []datasource.TraceAttribute { return nil }), nil
	}
	reg, err := datasource.NewRegistry(r.deps.Logger,
		datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: &fakeDS{}},
		datasource.Instance{UID: "tempo", Name: "Tempo", Datasource: ds},
	)
	if err != nil {
		t.Fatal(err)
	}
	r.deps.Datasources = reg
	return r, ds
}

func compareTraceIDs(prefix byte, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%c%031x", prefix, i+1)
	}
	return ids
}

func compareInput(selected, baseline []string, extra string) string {
	sel, _ := json.Marshal(selected)
	base, _ := json.Marshal(baseline)
	return `{"datasource":"tempo","selected":` + string(sel) + `,"baseline":` + string(base) + extra + `}`
}

func TestCompareTracesRanksAndSummarizes(t *testing.T) {
	r, ds := newCompareRig(t)
	ds.attributesFn = func(_ context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
		if req.IDs[0][0] == 'b' {
			req.IDs = req.IDs[:len(req.IDs)-1]
		}
		return compareSets(req.IDs, func(i int) []datasource.TraceAttribute {
			pairs := []datasource.TraceAttribute{{Key: "kind", Value: "Server"}}
			if req.IDs[0][0] == 'a' || i == 0 {
				pairs = append(pairs, datasource.TraceAttribute{Key: "span.db.system", Value: "postgres"})
			}
			if req.IDs[0][0] == 'a' && i < 2 {
				pairs = append(pairs, datasource.TraceAttribute{Key: "status", Value: "Error"})
			}
			return pairs
		}), nil
	}
	selected := compareTraceIDs('a', 4)
	baseline := compareTraceIDs('b', 11)
	res, err := call(t, r, nameCompareTraces, compareInput(append(selected, strings.ToUpper(selected[0])), baseline, `,"from":"2026-11-06T10:00:00Z","to":"2026-11-06T11:00:00Z"`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %#v", res.Content)
	}
	if want := "Compared 4 selected with 10 other traces on tempo: 2 differences"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"selected":{"requested":4,"read":4},"baseline":{"requested":11,"read":10},"attributes":3,"partial":true,"differences":[` +
		`{"attribute":"span.db.system","value":"postgres","selected_share":1,"baseline_share":0.1,"selected_count":4,"baseline_count":1},` +
		`{"attribute":"status","value":"Error","selected_share":0.5,"baseline_share":0,"selected_count":2,"baseline_count":0}],"truncated":false}`
	if string(body) != want {
		t.Fatalf("body =\n%s\nwant\n%s", body, want)
	}
	calls := ds.attributes
	if len(calls) != 2 || fmt.Sprint(calls[0].IDs) != fmt.Sprint(selected) || fmt.Sprint(calls[1].IDs) != fmt.Sprint(baseline) {
		t.Fatalf("calls = %+v, want the lowercased selected ids then the baseline ids", calls)
	}
	for _, c := range calls {
		if c.MaxSpans != datasource.CompareSpanBudget || !c.Range.From.Equal(time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)) || c.Range.To.Sub(c.Range.From) != time.Hour {
			t.Fatalf("call = %+v", c)
		}
	}
	res, err = call(t, r, nameCompareTraces, compareInput(selected, baseline, `,"limit":1`))
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content.(traceCompareOut)
	if len(out.Differences) != 1 || !out.Truncated || res.Step != "Compared 4 selected with 10 other traces on tempo: 1 difference" {
		t.Fatalf("limited = %+v, step %q", out, res.Step)
	}
	if span := ds.attributes[2].Range.To.Sub(ds.attributes[2].Range.From); span < 24*time.Hour-time.Minute || span > 24*time.Hour+time.Minute {
		t.Fatalf("default range spans %s, want about 24h", span)
	}
}

func TestCompareTracesRefusesBadInputAndOtherDatasources(t *testing.T) {
	r, ds := newCompareRig(t)
	one, other := compareTraceIDs('a', 1), compareTraceIDs('b', 1)
	cases := []struct {
		name, input, code, message string
	}{
		{"empty selected", compareInput(nil, other, ""), codeInvalidRequest, "selected must hold from 1 to 50 trace ids"},
		{"51 selected", compareInput(compareTraceIDs('a', 51), other, ""), codeInvalidRequest, "selected must hold from 1 to 50 trace ids"},
		{"151 baseline", compareInput(one, compareTraceIDs('b', 151), ""), codeInvalidRequest, "baseline must hold from 1 to 150 trace ids"},
		{"bad id", compareInput([]string{"abc"}, other, ""), codeInvalidRequest, "selected must hold trace ids of 16 or 32 hex characters"},
		{"overlap", compareInput(one, []string{strings.ToUpper(one[0])}, ""), codeInvalidRequest, "a trace id cannot be in both selected and baseline"},
		{"limit zero", compareInput(one, other, `,"limit":0`), codeInvalidRequest, ""},
		{"limit over 100", compareInput(one, other, `,"limit":101`), codeInvalidRequest, ""},
		{"a bad time", compareInput(one, other, `,"from":"yesterday-ish"`), codeInvalidRequest, ""},
		{"an unknown field", compareInput(one, other, `,"query":"{}"`), codeInvalidRequest, ""},
		{"a Prometheus datasource", strings.Replace(compareInput(one, other, ""), `"tempo"`, `"fake"`, 1), codeInvalidRequest, compareSourcesText},
		{"an unknown datasource", strings.Replace(compareInput(one, other, ""), `"tempo"`, `"nope"`, 1), "unknown_datasource", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := call(t, r, nameCompareTraces, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != tc.code {
				t.Fatalf("code = %q, want %q (%#v)", got, tc.code, res.Content)
			}
			if tc.message != "" && res.Content.(errorBody).Message != tc.message {
				t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, tc.message)
			}
		})
	}
	if len(ds.attributes) != 0 {
		t.Fatalf("%d upstream calls for invalid input", len(ds.attributes))
	}
	ds.attributesFn = func(context.Context, datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "connection refused by 10.0.0.5")
	}
	res, err := call(t, r, nameCompareTraces, compareInput(one, other, ""))
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != "datasource_unavailable" {
		t.Fatalf("code = %q", got)
	}
}

func TestCompareTracesAnswersTooManyRequestsWhenUpstreamIsBusy(t *testing.T) {
	r, ds := newCompareRig(t)
	for range 32 {
		release, err := r.deps.Runner.AcquireUpstream(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
	}
	res, err := call(t, r, nameCompareTraces, compareInput(compareTraceIDs('a', 1), compareTraceIDs('b', 1), ""))
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != "too_many_requests" {
		t.Fatalf("code = %q", got)
	}
	if len(ds.attributes) != 0 {
		t.Fatalf("%d upstream calls while every slot is held", len(ds.attributes))
	}
}

func TestCompareTracesIsOfferedToMCPAndEveryRunMode(t *testing.T) {
	tools := New(Deps{}).List()
	i := slices.IndexFunc(tools, func(tool Tool) bool { return tool.Name == nameCompareTraces })
	if i < 0 {
		t.Fatal("compare_traces is not listed")
	}
	tool := tools[i]
	if !tool.ReadOnly || tool.MinRole != auth.RoleEditor || tool.MCPOnly || tool.AgentOnly {
		t.Errorf("tool = %+v", tool)
	}
	for _, mode := range []Mode{ModeCreate, ModeInvestigate} {
		if !tool.OfferedIn(mode) {
			t.Errorf("not offered in %s runs", mode)
		}
	}
}
