package auditsource

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/logvolume"
)

type fakeStore struct {
	mu      sync.Mutex
	events  []trail.Event
	count   int64
	answer  func(filter.Query) trail.Aggregation
	pages   []filter.Page
	queries []filter.Query
	counts  []int
	windows []filter.Window
}

func (f *fakeStore) Rows(_ context.Context, q filter.Query, w filter.Window, page filter.Page) ([]trail.Event, error) {
	if _, err := q.Rows(w, page); err != nil {
		return nil, &trail.QueryRefused{Msg: err.Error()}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages, f.queries, f.windows = append(f.pages, page), append(f.queries, q), append(f.windows, w)
	from := min(page.Offset, len(f.events))
	return f.events[from:min(from+page.Limit, len(f.events))], nil
}

func (f *fakeStore) Count(_ context.Context, q filter.Query, w filter.Window, upTo int) (int64, error) {
	if _, err := q.Count(w, upTo); err != nil {
		return 0, &trail.QueryRefused{Msg: err.Error()}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts = append(f.counts, upTo)
	return min(f.count, int64(upTo)), nil
}

func (f *fakeStore) ReadAggregation(_ context.Context, p trail.Prepared) (trail.Aggregation, error) {
	agg, err := p.Query.Aggregate(p.Window)
	if err != nil {
		return trail.Aggregation{}, &trail.QueryRefused{Msg: err.Error()}
	}
	f.mu.Lock()
	f.queries = append(f.queries, p.Query)
	f.mu.Unlock()
	out := trail.Aggregation{Levels: agg.Levels, Metrics: agg.Metrics}
	if f.answer != nil {
		out.Rows = f.answer(p.Query).Rows
	}
	return out, nil
}

var (
	from = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	span = datasource.TimeRange{From: from, To: from.Add(5 * time.Minute)}
)

func open(t *testing.T, store *fakeStore) *elasticsearch.Datasource {
	t.Helper()
	in, err := Instance(store, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if in.UID != UID || in.Name != Name || in.Datasource.Type() != "opensearch" {
		t.Fatalf("instance = %+v", in)
	}
	return in.Datasource.(*elasticsearch.Datasource)
}

func text(s string) *string { return &s }

func sampleEvent() trail.Event {
	return trail.Event{
		ID: "0193a1f0-0000-7000-8000-000000000001", OccurredAt: "2026-11-01T10:01:00.123456Z",
		Category: "content", Action: "dashboard.update", Outcome: "denied", Reason: text("forbidden_role"),
		ActorKind: "user", ActorName: text("Riya"), Initiator: "person", Origin: "mcp",
		TargetKind: text("dashboard"), TargetName: text("Checkout"), TraceID: text("abc123"), Data: []byte(`{"version":3}`),
	}
}

func column(t *testing.T, f datasource.Frame, name string) []any {
	t.Helper()
	for _, field := range f.Fields {
		if field.Name == name {
			return field.Values
		}
	}
	t.Fatalf("frame has no %s column: %+v", name, f.Fields)
	return nil
}

func TestTheDatasourceIsHealthyAndListsTheAuditFields(t *testing.T) {
	ds := open(t, &fakeStore{})
	if err := ds.CheckHealth(t.Context()); err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	index, timeField := ds.Index()
	if index != Index || timeField != TimeField {
		t.Fatalf("index = %s %s", index, timeField)
	}
	m, err := ds.Mapping(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"outcome", "actor_name", "duration_ms", "client_ip", MessageField, TimeField} {
		if _, ok := m.Fields[name]; !ok {
			t.Errorf("mapping has no %s", name)
		}
	}
	if m.Fields["duration_ms"].Kind != elasticsearch.KindNumber || !m.Fields["outcome"].Aggregatable || m.Fields[MessageField].Aggregatable || m.Fields["target"].Aggregatable {
		t.Errorf("field kinds = %+v", m.Fields)
	}
	if m.Window != resultWindow {
		t.Errorf("result window = %d", m.Window)
	}
}

func TestLinesReadOldestFirstWithAMessageLine(t *testing.T) {
	store := &fakeStore{events: []trail.Event{sampleEvent()}}
	ds := open(t, store)
	frames, err := ds.Lines(t.Context(), datasource.LinesRequest{Query: datasource.Query{Text: "outcome:denied"}, Range: span, Direction: datasource.Forward, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := store.pages[0]; got != (filter.Page{Limit: 2, Ascending: true}) {
		t.Errorf("page = %+v", got)
	}
	if store.windows[0] != (filter.Window{From: span.From, To: span.To}) {
		t.Errorf("window = %+v", store.windows[0])
	}
	want := `dashboard.update denied (forbidden_role) by Riya via mcp on dashboard "Checkout"`
	if body := column(t, frames[0], "Body"); len(body) != 1 || body[0] != want {
		t.Errorf("body = %v, want %q", body, want)
	}
	if level := column(t, frames[0], "level"); level[0] != "denied" {
		t.Errorf("level = %v", level)
	}
	if times := column(t, frames[0], "TimeNs"); times[0] != "1793527260123456000" {
		t.Errorf("time ns = %v", times)
	}
	clause := store.queries[0].Clause
	if clause == nil || !strings.Contains(strings.Join(flatKeys(clause), ","), "outcome") {
		t.Errorf("clause = %v", clause)
	}
}

func flatKeys(v any) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			out = append(out, k)
			out = append(out, flatKeys(child)...)
		}
	case []any:
		for _, child := range x {
			out = append(out, flatKeys(child)...)
		}
	case string:
		out = append(out, x)
	}
	return out
}

func TestPagingSkipsAnOffsetAndCountsUpToTheResultWindow(t *testing.T) {
	store := &fakeStore{events: make([]trail.Event, 100), count: 60}
	for i := range store.events {
		store.events[i] = sampleEvent()
	}
	ds := open(t, store)
	_, info, err := ds.QueryPage(t.Context(), datasource.PageRequest{Query: datasource.Query{Text: ""}, Range: span, Page: 3, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if store.pages[0] != (filter.Page{Limit: 20, Offset: 40}) {
		t.Errorf("page = %+v", store.pages[0])
	}
	if !slices.Contains(store.counts, resultWindow+1) {
		t.Errorf("counts asked = %v", store.counts)
	}
	if info.Total != 60 || info.TotalRelation != datasource.TotalExact || info.HasNext {
		t.Errorf("page info = %+v", info)
	}
	store.count = resultWindow + 5
	_, info, err = ds.QueryPage(t.Context(), datasource.PageRequest{Range: span, Page: 1, PageSize: 20})
	if err != nil || info.Total != resultWindow || info.TotalRelation != datasource.TotalAtLeast {
		t.Errorf("capped page info = %+v, %v", info, err)
	}
}

func TestACountQueryFillsEveryStep(t *testing.T) {
	store := &fakeStore{answer: func(q filter.Query) trail.Aggregation {
		return trail.Aggregation{Rows: []trail.AggRow{{Keys: []any{"2026-11-01T10:02:00Z"}, DocCount: 3}}}
	}}
	ds := open(t, store)
	frames, err := ds.QueryRange(t.Context(), datasource.Query{Text: "outcome:denied"}, span, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %+v", frames)
	}
	values := frames[0].Fields[1].Values
	var total float64
	for _, v := range values {
		n, _ := v.(float64)
		total += n
	}
	if len(values) != 6 || total != 3 {
		t.Errorf("points = %v, want six steps holding three events", values)
	}
	agg := store.queries[0].Aggs[0]
	if agg.Type != dateHistogram || agg.Interval != time.Minute || store.queries[0].Clause == nil {
		t.Errorf("aggregation = %+v", store.queries[0])
	}
}

func TestTheVolumeCountsEachStepByOutcome(t *testing.T) {
	store := &fakeStore{answer: func(q filter.Query) trail.Aggregation {
		if len(q.Aggs[0].Aggs) == 0 {
			return trail.Aggregation{Rows: []trail.AggRow{{Keys: []any{"2026-11-01T10:01:00Z"}, DocCount: 7}}}
		}
		return trail.Aggregation{Rows: []trail.AggRow{
			{Keys: []any{"2026-11-01T10:01:00Z", "failed"}, DocCount: 2},
			{Keys: []any{"2026-11-01T10:01:00Z", "denied"}, DocCount: 1},
			{Keys: []any{"2026-11-01T10:01:00Z", "success"}, DocCount: 3},
			{Keys: []any{"2026-11-01T10:01:00Z", "unknown"}, DocCount: 1},
		}}
	}}
	ds := open(t, store)
	h, err := ds.LogVolume(t.Context(), datasource.VolumeRequest{Query: datasource.Query{Text: ""}, Range: span, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	want := logvolume.Bucket{Start: from.Add(time.Minute), Error: 2, Warning: 1, Info: 3, Other: 1}
	if got := h.Volume(); got.Total != 7 || got.Buckets[1] != want {
		t.Errorf("volume = %+v, want the second step to be %+v", got, want)
	}
	if len(store.queries) != 2 {
		t.Errorf("volume ran %d aggregations, want the time buckets and then the outcomes in them", len(store.queries))
	}
}

func TestFacetsCountEachFieldInItsOwnAggregation(t *testing.T) {
	store := &fakeStore{answer: func(q filter.Query) trail.Aggregation {
		a := q.Aggs[0]
		if a.Type != terms {
			return trail.Aggregation{Rows: []trail.AggRow{{DocCount: 9, Values: []*float64{ptr(2), ptr(3)}}}}
		}
		if a.Field == "origin" {
			return trail.Aggregation{Rows: []trail.AggRow{{Keys: []any{"web"}, DocCount: 6}, {Keys: []any{"mcp"}, DocCount: 3}}}
		}
		return trail.Aggregation{Rows: []trail.AggRow{{Keys: []any{"success"}, DocCount: 8}, {Keys: []any{"denied"}, DocCount: 1}}}
	}}
	ds := open(t, store)
	facets, err := ds.LogFacets(t.Context(), datasource.FacetRequest{Query: datasource.Query{Text: ""}, Range: span, Fields: []string{"outcome", "origin"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]datasource.FacetValue{}
	for _, f := range facets.Fields {
		got[f.Name] = f.Values
		if !f.Truncated {
			t.Errorf("%s is not truncated though a second value exists", f.Name)
		}
	}
	want := map[string][]datasource.FacetValue{"outcome": {{Value: "success", Count: 8}}, "origin": {{Value: "web", Count: 6}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("facets = %+v, want %+v", got, want)
	}
}

func TestTheDefaultFacetsOfEveryFieldStayWithinTheAggregationCap(t *testing.T) {
	store := &fakeStore{answer: func(q filter.Query) trail.Aggregation {
		if q.Aggs[0].Type == terms && q.Aggs[0].Field == "outcome" {
			return trail.Aggregation{Rows: []trail.AggRow{{Keys: []any{"denied"}, DocCount: 2}}}
		}
		return trail.Aggregation{}
	}}
	ds := open(t, store)
	facets, err := ds.LogFacets(t.Context(), datasource.FacetRequest{Query: datasource.Query{Text: ""}, Range: span, Limit: 5})
	if err != nil {
		t.Fatalf("LogFacets of every field: %v", err)
	}
	if len(facets.Fields) <= maxAggregations/2 {
		t.Fatalf("%d facet fields, want more than one request's worth", len(facets.Fields))
	}
	for _, f := range facets.Fields {
		if f.Name == "outcome" && (len(f.Values) != 1 || f.Values[0].Value != "denied") {
			t.Errorf("outcome facet = %+v", f.Values)
		}
	}
}

func ptr(v float64) *float64 { return &v }

func TestALuceneBodyIsReadByTheAuditParser(t *testing.T) {
	store := &fakeStore{}
	ds := open(t, store)
	_, err := ds.Lines(t.Context(), datasource.LinesRequest{Query: datasource.Query{Text: `{"query":{"query_string":{"query":"outcome:denied AND origin:mcp"}}}`}, Range: span, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filter.Parse("lucene:outcome:denied AND origin:mcp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(flatKeys(store.queries[0].Clause), ","), "mcp") || !reflect.DeepEqual(flatKeysSorted(store.queries[0].Clause), flatKeysSorted(map[string]any{"bool": map[string]any{"filter": []any{want.Clause}}})) {
		t.Errorf("clause = %v, want the Lucene parse %v", store.queries[0].Clause, want.Clause)
	}
}

func TestEveryFormOfTheLogsQueryBarReachesTheStore(t *testing.T) {
	for _, text := range []string{
		"checkout",
		"action:dashboard.* and not origin:web",
		`actor_name:"Sam Lee" or status >= 500`,
		"lucene: outcome:denied AND origin:(mcp OR cli)",
		"lucene: status:[500 TO 599] -action:auth.*",
	} {
		t.Run(text, func(t *testing.T) {
			store := &fakeStore{}
			if _, err := open(t, store).Lines(t.Context(), datasource.LinesRequest{Query: datasource.Query{Text: text}, Range: span, Limit: 5}); err != nil {
				t.Fatal(err)
			}
			if len(store.queries) != 1 || store.queries[0].Clause == nil {
				t.Fatalf("queries = %+v", store.queries)
			}
		})
	}
}

func flatKeysSorted(v any) []string {
	keys := flatKeys(v)
	slices.Sort(keys)
	return keys
}

func TestWhatTheAuditTrailCannotAnswerIsAnInvalidQueryNamingIt(t *testing.T) {
	ds := open(t, &fakeStore{})
	for _, tc := range []struct{ text, says string }{
		{`{"query":{"nested":{"path":"a","query":{}}}}`, `"nested"`},
		{`{"query":{"match_all":{}},"sort":[{"action":"asc"}]}`, "sorts only by occurred_at"},
		{`{"query":{"match_all":{}},"runtime_mappings":{"x":{"type":"keyword"}}}`, `"runtime_mappings"`},
		{`{"aggs":{"h":{"histogram":{"field":"status","interval":10}}}}`, "unsupported aggregation"},
		{`{"query":{"query_string":{"query":"no_such_field:x"}}}`, "unknown field"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			_, err := ds.QueryInstant(t.Context(), datasource.Query{Text: tc.text}, span)
			if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("error = %v, want an invalid query that says %s", err, tc.says)
			}
		})
	}
}

func TestLintChecksLuceneWithTheAuditParser(t *testing.T) {
	out, err := validate([]byte(`{"query":{"query_string":{"query":"outcom:denied"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if out["valid"] != false || !strings.Contains(out["explanations"].([]any)[0].(map[string]any)["error"].(string), "outcome") {
		t.Fatalf("validate = %v", out)
	}
	out, err = validate([]byte(`{"query":{"query_string":{"query":"outcome:denied"}}}`))
	if err != nil || out["valid"] != true {
		t.Fatalf("validate a good query = %v, %v", out, err)
	}
}

func TestTheWindowComesFromTheTimeRangeInEitherFormat(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  filter.Window
		rest  bool
	}{
		{`{"range":{"occurred_at":{"gte":1793527200000,"lte":1793527500000,"format":"epoch_millis"}}}`, filter.Window{From: from, To: from.Add(5*time.Minute + time.Millisecond)}, false},
		{`{"bool":{"filter":[{"range":{"occurred_at":{"gte":"2026-11-01T10:00:00Z","lt":"2026-11-01T10:05:00Z","format":"strict_date_optional_time_nanos"}}},{"term":{"outcome":"denied"}}]}}`, filter.Window{From: from, To: from.Add(5 * time.Minute)}, true},
	} {
		query, err := decodeBody([]byte(tc.query))
		if err != nil {
			t.Fatal(err)
		}
		w, rest, err := splitWindow(query)
		if err != nil || w != tc.want || (rest != nil) != tc.rest {
			t.Errorf("splitWindow(%s) = %+v, %v, %v", tc.query, w, rest, err)
		}
	}
	for _, query := range []string{
		`{"term":{"outcome":"denied"}}`,
		`{"range":{"occurred_at":{"gte":"2026-11-01T10:00:00Z","format":"strict_date_optional_time_nanos"}}}`,
		`{"range":{"occurred_at":{"gte":"now-1h","lt":"now","format":"date_math"}}}`,
	} {
		parsed, _ := decodeBody([]byte(query))
		if _, _, err := splitWindow(parsed); err == nil {
			t.Errorf("splitWindow(%s) accepted", query)
		}
	}
}

func TestBinsFollowTheZonesMidnight(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	bins, err := binsBetween(from, from.Add(48*time.Hour), 24*time.Hour, kolkata)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{
		time.Date(2026, 10, 31, 18, 30, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 18, 30, 0, 0, time.UTC),
		time.Date(2026, 11, 2, 18, 30, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(bins, want) {
		t.Errorf("bins = %v, want %v", bins, want)
	}
	if _, err := binsBetween(from, from.Add(time.Hour), time.Millisecond, time.UTC); err == nil {
		t.Error("binsBetween filled more than the bucket cap")
	}
}

func TestMetricsAnswerInTheOpenSearchShape(t *testing.T) {
	got := metricsOf([]filter.Metric{
		{Name: "p", Type: "percentiles", Key: "50"}, {Name: "p", Type: "percentiles", Key: "99.9"},
		{Name: "n", Type: "cardinality"}, {Name: "avg", Type: "avg"},
	}, []*float64{ptr(12), ptr(80), nil, nil})
	want := map[string]any{
		"p":   map[string]any{"values": map[string]any{"50.0": ptr(12), "99.9": ptr(80)}},
		"n":   map[string]any{"value": ptr(0)},
		"avg": map[string]any{"value": (*float64)(nil)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("metrics = %v", got)
	}
}

func TestTheMessageLineNamesTheActorAgentAndTarget(t *testing.T) {
	e := trail.Event{Action: "query.run", Outcome: "success", ActorKind: "user", ActorName: text("Ada"), Initiator: "agent", AgentName: text("Observe Agent"), Origin: "agent", DatasourceUID: text("prom")}
	if got, want := Line(e), `query.run success by Ada with agent "Observe Agent" via agent`; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	anonymous := trail.Event{Action: "auth.login_failed", Outcome: "denied", ActorKind: "anonymous", Origin: "web", TargetKind: text("session")}
	if got, want := Line(anonymous), "auth.login_failed denied by anonymous via web on session"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

func TestTheLintAcceptsEveryAliasTheQueryAccepts(t *testing.T) {
	ds := open(t, &fakeStore{})
	for _, f := range filter.Fields() {
		for _, alias := range f.Aliases {
			diags, err := ds.Lint(t.Context(), alias+":x*")
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) != 0 {
				t.Errorf("lint of the alias %s = %+v, want no diagnostics", alias, diags)
			}
		}
	}
	diags, err := ds.Lint(t.Context(), "actor.nmae:admin*")
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, `unknown field "actor.nmae"`) {
		t.Errorf("lint of an unknown field = %+v, want one unknown field warning", diags)
	}
}

func TestTheLintWarnsOfAFieldPatternExactlyWhenTheQueryRefusesIt(t *testing.T) {
	lintAgreesWithQuery(t, "actor.*", "target.*", "actor_*", "Actor_*", "mess*", "nosuch*", "actor.name", "message", "nosuch")
}

func TestTheLintIgnoresTheCaseOfAFieldNameAsTheQueryDoes(t *testing.T) {
	lintAgreesWithQuery(t, "ACTOR", "Target.Name", "Actor_Name", "NOSUCH")
}

func lintAgreesWithQuery(t *testing.T, fields ...string) {
	t.Helper()
	ds := open(t, &fakeStore{})
	for _, field := range fields {
		text := field + ": alice"
		diags, err := ds.Lint(t.Context(), text)
		if err != nil {
			t.Fatal(err)
		}
		_, queryErr := ds.QueryInstant(t.Context(), datasource.Query{Text: text}, span)
		if warned := len(diags) > 0; warned != (queryErr != nil) {
			t.Errorf("%s: lint %+v, query error %v; the lint must warn exactly when the query refuses the field", field, diags, queryErr)
		}
	}
}
