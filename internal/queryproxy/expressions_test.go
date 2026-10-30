package queryproxy

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy/expr"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func seriesFrame(values ...float64) datasource.Frame {
	times := make([]any, len(values))
	points := make([]any, len(values))
	for i, v := range values {
		times[i] = testNow.Add(time.Duration(i-len(values)) * time.Minute)
		points[i] = v
	}
	return datasource.Frame{Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: times},
		{Name: "Value", Type: datasource.FieldNumber, Values: points},
	}}
}

func answering(byText map[string][]datasource.Frame) func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	return func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		return byText[q.Text], nil
	}
}

func mathQuery(ref, text string, hide bool) spec.Query {
	return spec.Query{Ref: ref, Hide: hide, Expression: &spec.Expression{Type: spec.ExpressionMath, Math: text}}
}

func reduceQuery(ref, input, reducer string, hide bool) spec.Query {
	return spec.Query{Ref: ref, Hide: hide, Expression: &spec.Expression{Type: spec.ExpressionReduce, Input: input, Reducer: reducer}}
}

func lastValues(t *testing.T, f datasource.Frame) []any {
	t.Helper()
	return f.Fields[len(f.Fields)-1].Values
}

func TestRunPanelExpressionOverAHiddenQueryReturnsOnlyTheExpression(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(map[string][]datasource.Frame{"errors": {seriesFrame(1, 2, 3)}})
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "errors", Hide: true},
		mathQuery("B", "$A * 2", false),
	)})

	res, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange(), MaxDataPoints: 200})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if len(res.Queries) != 1 || res.Queries[0].Ref != "B" {
		t.Fatalf("queries = %+v, want only B", res.Queries)
	}
	got := lastValues(t, res.Queries[0].Frames[0])
	if !slices.Equal(got, []any{2.0, 4.0, 6.0}) {
		t.Errorf("values = %v, want [2 4 6]", got)
	}
	if fake.callCount() != 1 {
		t.Errorf("upstream calls = %d, want 1 (the hidden input runs because B names it)", fake.callCount())
	}
	if res.Queries[0].Kind != KindRange || res.Queries[0].Step != 108*time.Second {
		t.Errorf("kind = %q, step = %v, want range and the input's step", res.Queries[0].Kind, res.Queries[0].Step)
	}
}

func TestRunPanelDoesNotRunAHiddenQueryNoExpressionNames(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(map[string][]datasource.Frame{"a": {seriesFrame(1)}, "b": {seriesFrame(2)}})
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a", Hide: true},
		spec.Query{Ref: "B", Query: "b", Hide: true},
		reduceQuery("C", "A", "sum", false),
	)})
	if _, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()}); err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	texts := []string{}
	for _, c := range fake.recorded() {
		texts = append(texts, c.query.Text)
	}
	if !slices.Equal(texts, []string{"a"}) {
		t.Errorf("queries run = %v, want only a", texts)
	}
}

func TestRunPanelReturnsVisibleInputsAndExpressionsInPanelOrder(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(map[string][]datasource.Frame{"a": {seriesFrame(4, 6)}})
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a"},
		reduceQuery("B", "A", "mean", false),
	)})
	res, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	var refs []string
	for _, q := range res.Queries {
		refs = append(refs, q.Ref)
	}
	if !slices.Equal(refs, []string{"A", "B"}) {
		t.Fatalf("refs = %v, want [A B]", refs)
	}
	if got := lastValues(t, res.Queries[1].Frames[0]); !slices.Equal(got, []any{5.0}) {
		t.Errorf("reduce = %v, want [5]", got)
	}
}

func TestRunPanelEvaluatesExpressionsOnExpressionsWhateverTheirOrder(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(map[string][]datasource.Frame{"a": {seriesFrame(1, 3)}})
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		mathQuery("C", "$B + 1", false),
		reduceQuery("B", "M", "max", true),
		mathQuery("M", "$A * 10", true),
		spec.Query{Ref: "A", Query: "a", Hide: true},
	)})
	res, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if len(res.Queries) != 1 || res.Queries[0].Ref != "C" {
		t.Fatalf("queries = %+v, want only C", res.Queries)
	}
	if got := lastValues(t, res.Queries[0].Frames[0]); !slices.Equal(got, []any{31.0}) {
		t.Errorf("C = %v, want [31]", got)
	}
}

func TestRunPanelExpressionKindFollowsItsInputs(t *testing.T) {
	cases := []struct {
		name    string
		queries []spec.Query
		want    []Kind
	}{
		{"over a range query", []spec.Query{{Ref: "A", Query: "a"}, reduceQuery("B", "A", "sum", false)}, []Kind{KindRange, KindRange}},
		{"over an instant query", []spec.Query{{Ref: "A", Query: "a", Instant: true}, reduceQuery("B", "A", "sum", false)}, []Kind{KindInstant, KindInstant}},
		{"through a chain", []spec.Query{{Ref: "A", Query: "a"}, reduceQuery("B", "C", "sum", false), reduceQuery("C", "A", "sum", false)}, []Kind{KindRange, KindRange, KindRange}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := multiPanel(spec.Timeseries, tc.queries...)
			if got := queryKinds(&p); !slices.Equal(got, tc.want) {
				t.Errorf("kinds = %v, want %v", got, tc.want)
			}
		})
	}
	hidden := multiPanel(spec.Timeseries, spec.Query{Ref: "A", Query: "a", Instant: true}, reduceQuery("B", "A", "sum", false))
	if got := PanelKind(&hidden); got != KindInstant {
		t.Errorf("PanelKind = %q, want instant for an expression over an instant query", got)
	}
}

func TestRunPanelFailedInputFailsItsDependantsWithAnExpressionError(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		if q.Text == "bad" {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error")
		}
		return []datasource.Frame{seriesFrame(1)}, nil
	}
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "bad", Hide: true},
		mathQuery("B", "$A + 1", false),
		mathQuery("C", "$B + 1", false),
	)})
	_, _, errs, err := runner.runQueries(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()}, newVarState(), true)
	if err != nil {
		t.Fatalf("runQueries: %v", err)
	}
	if !errors.Is(errs[0], datasource.ErrInvalidQuery) {
		t.Errorf("A: %v, want the datasource error", errs[0])
	}
	for _, j := range []int{1, 2} {
		status, code, message := Classify(errs[j])
		if status != http.StatusUnprocessableEntity || code != string(CodeExpressionFailed) {
			t.Errorf("query %d: status %d code %q, want 422 expression_error", j, status, code)
		}
		var qe *QueryError
		if !errors.As(errs[j], &qe) || qe.Index != j {
			t.Errorf("query %d: %v is not a QueryError for it", j, errs[j])
		}
		if message == "" {
			t.Errorf("query %d: empty message", j)
		}
	}

	_, err = runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	var qe *QueryError
	if !errors.As(err, &qe) || qe.Ref != "A" {
		t.Errorf("RunPanel err = %v, want the lowest failing query, A", err)
	}
}

func TestRunPanelReportsAnExpressionThatCannotEvaluate(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a"},
		mathQuery("B", "sin($A)", false),
	)})
	_, err := runner.RunPanel(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()})
	if !errors.Is(err, expr.ErrExpression) {
		t.Fatalf("err = %v, want an expression error (the fake's frames carry no real times, so even A feeds it badly)", err)
	}
	if _, code, _ := Classify(err); code != string(CodeExpressionFailed) {
		t.Errorf("code = %q, want expression_error", code)
	}
}

func TestRunPanelStopsAnExpressionCycleInsteadOfHanging(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		reduceQuery("A", "B", "sum", false),
		reduceQuery("B", "A", "sum", false),
	)})
	_, _, errs, err := runner.runQueries(t.Context(), Request{Dashboard: d, PanelID: "p1", Range: testRange()}, newVarState(), true)
	if err != nil {
		t.Fatalf("runQueries: %v", err)
	}
	for j, qerr := range errs {
		if !errors.Is(qerr, expr.ErrExpression) {
			t.Errorf("query %d: %v, want an expression error", j, qerr)
		}
	}
	if fake.callCount() != 0 {
		t.Errorf("upstream calls = %d, want 0", fake.callCount())
	}
}

func TestRunPanelAdmitsVisibleExpressionsOnly(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(map[string][]datasource.Frame{"a": {seriesFrame(1, 2)}})
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a", Hide: true},
		mathQuery("B", "$A + 1", false),
		mathQuery("C", "$A + 2", true),
	)})
	var mu sync.Mutex
	admitted := map[int]string{}
	_, err := runner.RunPanel(t.Context(), Request{
		Dashboard: d, PanelID: "p1", Range: testRange(), IncludeHidden: true,
		Admit: func(index int, q QueryResult) error {
			mu.Lock()
			defer mu.Unlock()
			admitted[index] = q.Ref
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RunPanel: %v", err)
	}
	if len(admitted) != 1 || admitted[1] != "B" {
		t.Errorf("admitted = %v, want only the visible expression at index 1", admitted)
	}
}

func TestRunPanelAbortFromAdmittingAnExpressionEndsTheRequest(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(map[string][]datasource.Frame{"a": {seriesFrame(1, 2)}})
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a", Hide: true},
		mathQuery("B", "$A + 1", false),
	)})
	tooBig := errors.New("response too big")
	_, err := runner.RunPanel(t.Context(), Request{
		Dashboard: d, PanelID: "p1", Range: testRange(),
		Admit: func(int, QueryResult) error { return &AbortError{Err: tooBig} },
	})
	if !errors.Is(err, tooBig) {
		t.Fatalf("err = %v, want the abort's error", err)
	}
}

func TestCheckSpecAcceptsAnExpressionOverHiddenQueries(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(map[string][]datasource.Frame{"a": {seriesFrame(1, 2)}, "b": {seriesFrame(3, 4)}})
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a", Hide: true},
		spec.Query{Ref: "B", Query: "b", Hide: true},
		mathQuery("C", "$A / $B", false),
	)})
	issues, err := runner.CheckSpec(t.Context(), d, testNow, datasource.QueryUser{})
	if err != nil || issues != nil {
		t.Fatalf("CheckSpec = %+v, %v; want a clean save check", issues, err)
	}
	if fake.callCount() != 2 {
		t.Errorf("upstream calls = %d, want 2", fake.callCount())
	}
}

func TestCheckSpecRejectsAnExpressionWithNoData(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(nil)
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a", Hide: true},
		reduceQuery("B", "A", "sum", false),
	)})
	issues, err := runner.CheckSpec(t.Context(), d, testNow, datasource.QueryUser{})
	if err != nil {
		t.Fatalf("CheckSpec: %v", err)
	}
	assertIssues(t, issues, []wantIssue{{"/panels/0/queries", CodeQueryEmpty, ""}})
}

func TestCheckSpecReportsAFailedExpressionAtItsExpression(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = answering(map[string][]datasource.Frame{"a": {seriesFrame(1)}})
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a", Hide: true},
		mathQuery("B", "$A +", false),
	)})
	issues, err := runner.CheckSpec(t.Context(), d, testNow, datasource.QueryUser{})
	if err != nil {
		t.Fatalf("CheckSpec: %v", err)
	}
	assertIssues(t, issues, []wantIssue{{"/panels/0/queries/1/expression", CodeExpressionFailed, ""}})
}

func TestCheckSpecSkipsAnExpressionWhoseInputCannotRun(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := dashboardOf([]spec.Panel{multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a", Hide: true, Datasource: "missing"},
		mathQuery("B", "$A + 1", false),
	)})
	issues, err := runner.CheckSpec(t.Context(), d, testNow, datasource.QueryUser{})
	if err != nil {
		t.Fatalf("CheckSpec: %v", err)
	}
	assertIssues(t, issues, []wantIssue{{"/panels/0/queries/0/datasource", CodeUnknownDatasource, ""}})
	if fake.callCount() != 0 {
		t.Errorf("upstream calls = %d, want 0", fake.callCount())
	}
}

func TestPropagateSkipsMarksDependantsThroughChains(t *testing.T) {
	p := multiPanel(spec.Timeseries,
		spec.Query{Ref: "A", Query: "a"},
		spec.Query{Ref: "B", Query: "b"},
		mathQuery("C", "$A + 1", false),
		reduceQuery("D", "C", "sum", false),
		reduceQuery("E", "B", "sum", false),
	)
	got := propagateSkips(&p, []bool{true})
	want := []bool{true, false, true, true, false}
	if !slices.Equal(got, want) {
		t.Errorf("skips = %v, want %v", got, want)
	}
}
