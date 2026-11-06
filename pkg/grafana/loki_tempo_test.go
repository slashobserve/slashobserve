package grafana

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const lokiRef = `{"type":"loki","uid":"loki"}`

var promOnly = Options{Datasources: []Datasource{{UID: "prom", Type: "prometheus"}}}

func panelOn(ds, typ, targets string) string {
	return fmt.Sprintf(`{"title":"T","schemaVersion":39,"panels":[{"id":1,"title":"T","type":%q,"gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":%s,"targets":[%s]}]}`, typ, ds, targets)
}

func lokiLogsDoc() string {
	return panelOn(lokiRef, "logs", `{"refId":"A","expr":"{app=\"web\"}"}`)
}

func TestAKindWithNoConfiguredDatasourceImportsAsPlaceholders(t *testing.T) {
	res := convertWith(t, lokiLogsDoc(), promOnly)
	if !res.Ready() {
		t.Fatalf("result is not ready: %+v", res.Datasources)
	}
	entry, ok := res.Spec.FindPanel("p1")
	if !ok {
		t.Fatal("no panel p1")
	}
	if entry.Panel.Type != spec.Placeholder || entry.Panel.Reason != "No Loki datasource is configured." {
		t.Fatalf("panel = %+v, want a placeholder naming the missing kind", entry.Panel)
	}
	s := sourceNamed(t, res, "loki")
	if s.Kind == nil || *s.Kind != kindLoki || len(s.Candidates) != 0 || s.Candidates == nil || s.Match != nil || s.Unmatched() {
		t.Fatalf("source = %+v, want kind loki with no candidates and no match", s)
	}
}

func TestAKindWithOneConfiguredDatasourceMatchesIt(t *testing.T) {
	res := convertDoc(t, lokiLogsDoc())
	s := sourceNamed(t, res, "loki")
	if !res.Ready() || s.Match == nil || *s.Match != "loki" || *s.MatchedBy != "uid" {
		t.Fatalf("source = %+v, want matched", s)
	}
	other := panelOn(`{"type":"loki","uid":"graf-logs"}`, "logs", `{"refId":"A","expr":"{app=\"web\"}"}`)
	s = sourceNamed(t, convertDoc(t, other), "graf-logs")
	if s.Match == nil || *s.Match != "loki" || *s.MatchedBy != "only" {
		t.Fatalf("source = %+v, want the only Loki datasource", s)
	}
}

func TestAnAvailableKindWithTwoCandidatesStillBlocks(t *testing.T) {
	two := Options{Datasources: []Datasource{{UID: "a", Type: "loki"}, {UID: "b", Type: "loki"}}}
	res := convertWith(t, lokiLogsDoc(), two)
	s := sourceNamed(t, res, "loki")
	if res.Ready() || !s.Unmatched() {
		t.Fatalf("source = %+v, want an unmatched source that blocks", s)
	}
}

func TestMappingAKindWithNoCandidatesIsAnInputError(t *testing.T) {
	_, err := Convert([]byte(lokiLogsDoc()), Options{Datasources: promOnly.Datasources, Mapping: map[string]string{"loki": "prom"}})
	var input *InputError
	if !errors.As(err, &input) || input.Code != codeInvalidMapping {
		t.Fatalf("error = %v, want an invalid mapping", err)
	}
}

func TestUnmatchedNeedsAKindCandidatesAndNoMatch(t *testing.T) {
	kind, match := kindLoki, "loki"
	cases := []struct {
		name string
		d    SourceDatasource
		want bool
	}{
		{"no kind", SourceDatasource{Candidates: []string{"x"}}, false},
		{"no candidates", SourceDatasource{Kind: &kind, Candidates: []string{}}, false},
		{"matched", SourceDatasource{Kind: &kind, Candidates: []string{"loki"}, Match: &match}, false},
		{"open", SourceDatasource{Kind: &kind, Candidates: []string{"loki"}}, true},
	}
	for _, tc := range cases {
		if got := tc.d.Unmatched(); got != tc.want {
			t.Errorf("%s: Unmatched = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSourcesWithoutATypeInferLokiAndTempo(t *testing.T) {
	cases := []struct {
		name, target, want string
	}{
		{"log selector", `{"refId":"A","expr":"{app=\"web\"}"}`, "loki"},
		{"line limit", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","maxLines":100}`, "loki"},
		{"traceql", `{"refId":"A","queryType":"traceql","query":"{}"}`, "tempo"},
		{"trace id", `{"refId":"A","queryType":"traceId","query":"abc"}`, "tempo"},
		{"metric expression", `{"refId":"A","expr":"rate(up[5m])"}`, "prometheus"},
		{"clickhouse query type", `{"refId":"A","queryType":"table","rawSql":"select 1"}`, "grafana-clickhouse-datasource"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertDoc(t, panelOn(`"Legacy"`, "table", tc.target))
			if s := sourceNamed(t, res, "Legacy"); s.Type == nil || *s.Type != tc.want {
				t.Fatalf("source = %+v, want type %s", s, tc.want)
			}
		})
	}
}

func TestUnknownPluginsKeepTheirReason(t *testing.T) {
	res := convertDoc(t, panelOn(`{"type":"influxdb","uid":"flux"}`, "table", `{"refId":"A","query":"SELECT mean(value) FROM cpu"}`))
	entry, _ := res.Spec.FindPanel("p1")
	if entry.Panel.Type != spec.Placeholder || entry.Panel.Reason != "Influxdb is not supported yet." {
		t.Fatalf("panel = %+v", entry.Panel)
	}
	if s := sourceNamed(t, res, "flux"); s.Kind != nil || !slices.Equal(s.Candidates, []string{}) {
		t.Fatalf("source = %+v", s)
	}
}

const tempoRef = `{"type":"tempo","uid":"tempo"}`

func importedOn(t *testing.T, ds, typ string, targets ...string) (spec.Panel, PanelLine) {
	t.Helper()
	res := convertDoc(t, panelOn(ds, typ, strings.Join(targets, ",")))
	if !res.Ready() {
		t.Fatalf("result is not ready: %+v", res.Datasources)
	}
	entry, ok := res.Spec.FindPanel("p1")
	if !ok {
		t.Fatal("no panel p1")
	}
	return *entry.Panel, panelLine(t, res, "p1")
}

func onlyQuery(t *testing.T, panel spec.Panel) spec.Query {
	t.Helper()
	if len(panel.Queries) != 1 {
		t.Fatalf("queries = %+v, want one", panel.Queries)
	}
	return panel.Queries[0]
}

func wantClean(t *testing.T, line PanelLine) {
	t.Helper()
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("line = %+v, want imported with no report line", line)
	}
}

func wantReported(t *testing.T, line PanelLine, path, reason string) {
	t.Helper()
	if !slices.Contains(line.Paths, path) || !slices.Contains(line.Reasons, reason) {
		t.Fatalf("line paths %v reasons %q, want %s reported with %q", line.Paths, line.Reasons, path, reason)
	}
}

func TestLokiMetricQueriesImportTheirQueryOptions(t *testing.T) {
	cases := []struct {
		name, target string
		want         spec.Query
	}{
		{"range", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","legendFormat":"{{app}}"}`, spec.Query{Query: `sum(rate({app="web"}[5m]))`, Legend: "{{app}}"}},
		{"instant query type", `{"refId":"A","expr":"sum(count_over_time({app=\"web\"}[5m]))","queryType":"instant"}`, spec.Query{Query: `sum(count_over_time({app="web"}[5m]))`, Instant: true}},
		{"legacy instant", `{"refId":"A","expr":"sum(count_over_time({app=\"web\"}[5m]))","instant":true}`, spec.Query{Query: `sum(count_over_time({app="web"}[5m]))`, Instant: true}},
		{"query type wins over legacy instant", `{"refId":"A","expr":"sum(count_over_time({app=\"web\"}[5m]))","queryType":"range","instant":true,"range":false}`, spec.Query{Query: `sum(count_over_time({app="web"}[5m]))`}},
		{"resolution", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","resolution":5}`, spec.Query{Query: `sum(rate({app="web"}[5m]))`, IntervalFactor: 5}},
		{"default step", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","step":"$__interval"}`, spec.Query{Query: `sum(rate({app="web"}[5m]))`}},
		{"line limit on a metric query", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","maxLines":100,"direction":"forward"}`, spec.Query{Query: `sum(rate({app="web"}[5m]))`}},
		{"instant ignores resolution and step", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","queryType":"instant","resolution":5,"step":"1m"}`, spec.Query{Query: `sum(rate({app="web"}[5m]))`, Instant: true}},
		{"builder bookkeeping", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","editorMode":"builder","supportingQueryType":"logsVolume","direction":"backward"}`, spec.Query{Query: `sum(rate({app="web"}[5m]))`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := importedOn(t, lokiRef, "timeseries", tc.target)
			got := onlyQuery(t, panel)
			got.Ref, got.Datasource = "", ""
			if got != tc.want {
				t.Fatalf("query = %+v, want %+v", got, tc.want)
			}
			wantClean(t, line)
		})
	}
}

func TestLokiAutoIntervalFollowsTheQueryType(t *testing.T) {
	cases := []struct {
		name, target, want, rewrite string
	}{
		{"range", `{"refId":"A","expr":"sum(rate({app=\"web\"}[$__auto]))"}`, `sum(rate({app="web"}[$__interval]))`, "$__auto -> $__interval"},
		{"range braces", `{"refId":"A","expr":"sum(rate({app=\"web\"}[${__auto}])) / sum(rate({app=\"api\"}[$__auto]))"}`, `sum(rate({app="web"}[$__interval])) / sum(rate({app="api"}[$__interval]))`, "$__auto -> $__interval"},
		{"instant", `{"refId":"A","expr":"sum(count_over_time({app=\"web\"}[$__auto]))","queryType":"instant"}`, `sum(count_over_time({app="web"}[$__range]))`, "$__auto -> $__range"},
		{"instant braces", `{"refId":"A","expr":"sum(count_over_time({app=\"web\"}[${__auto}]))","instant":true}`, `sum(count_over_time({app="web"}[$__range]))`, "$__auto -> $__range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := importedOn(t, lokiRef, "timeseries", tc.target)
			if got := onlyQuery(t, panel).Query; got != tc.want {
				t.Fatalf("query = %q, want %q", got, tc.want)
			}
			if !slices.Equal(line.Queries[0].Rewrites, []string{tc.rewrite}) {
				t.Fatalf("rewrites = %v, want %s", line.Queries[0].Rewrites, tc.rewrite)
			}
		})
	}
}

func TestLokiLogQueriesReadTheWholeRange(t *testing.T) {
	panel, line := importedOn(t, lokiRef, "logs", `{"refId":"A","expr":"{app=\"web\"} |= \"error\"","legendFormat":"{{app}}","resolution":2,"step":"1m","direction":"backward"}`)
	if got := onlyQuery(t, panel); got.Query != `{app="web"} |= "error"` || got.Legend != "" || got.Instant || got.Interval != "" || got.IntervalFactor != 0 {
		t.Fatalf("query = %+v", got)
	}
	wantClean(t, line)

	panel, line = importedOn(t, lokiRef, "logs", `{"refId":"A","expr":"{app=\"web\"}","queryType":"instant"}`)
	if onlyQuery(t, panel).Instant {
		t.Fatal("a log query was imported as instant")
	}
	wantReported(t, line, "targets[0].queryType", lokiLogRangeReason)

	_, line = importedOn(t, lokiRef, "logs", `{"refId":"A","expr":"{app=\"web\"}","instant":true}`)
	wantReported(t, line, "targets[0].instant", lokiLogRangeReason)
}

func TestLokiOptionsWithoutACounterpartAreReported(t *testing.T) {
	cases := []struct {
		name, typ, target, path, reason string
	}{
		{"line limit", "logs", `{"refId":"A","expr":"{app=\"web\"}","maxLines":100}`, "targets[0].maxLines", decisionLokiLineLimit.reason},
		{"oldest first", "logs", `{"refId":"A","expr":"{app=\"web\"}","direction":"forward"}`, "targets[0].direction", decisionLokiDirection.reason},
		{"scan", "logs", `{"refId":"A","expr":"{app=\"web\"}","direction":"scan"}`, "targets[0].direction", decisionLokiDirection.reason},
		{"stream", "logs", `{"refId":"A","expr":"{app=\"web\"}","queryType":"stream"}`, "targets[0].queryType", decisionLive.reason},
		{"unknown query type", "timeseries", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","queryType":"sample"}`, "targets[0].queryType", "The query type sample is not supported yet."},
		{"resolution out of range", "timeseries", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","resolution":7.5}`, "targets[0].resolution", "The resolution factor of this query is not supported yet."},
		{"step", "timeseries", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","step":"1m"}`, "targets[0].step", lokiStepReason},
		{"step that cannot be kept", "timeseries", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","step":"$__rate_interval"}`, "targets[0].step", minStepLostReason("$__rate_interval")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := importedOn(t, lokiRef, tc.typ, tc.target)
			wantReported(t, line, tc.path, tc.reason)
		})
	}
}

func TestLokiResolutionSevenIsKept(t *testing.T) {
	panel, _ := importedOn(t, lokiRef, "timeseries", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","resolution":7}`)
	if got := onlyQuery(t, panel).IntervalFactor; got != 7 {
		t.Fatalf("intervalFactor = %d, want 7", got)
	}
}

func TestLokiStepBecomesTheSmallestStep(t *testing.T) {
	panel, _ := importedOn(t, lokiRef, "timeseries", `{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))","step":"1m"}`)
	if got := onlyQuery(t, panel).Interval; got != "1m" {
		t.Fatalf("interval = %q, want 1m", got)
	}
}

func TestLokiTargetsThatCannotRunAreDropped(t *testing.T) {
	panel, line := importedOn(t, lokiRef, "timeseries",
		`{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))"}`,
		`{"refId":"B","expr":"  "}`,
	)
	if len(panel.Queries) != 1 {
		t.Fatalf("queries = %+v, want the empty one dropped", panel.Queries)
	}
	wantClean(t, line)

	_, line = importedOn(t, lokiRef, "timeseries",
		`{"refId":"A","expr":"sum(rate({app=\"web\"}[5m]))"}`,
		`{"refId":"B","expr":"sum(rate({app=\"web\"}[$__rate_interval]))"}`,
	)
	wantReported(t, line, "targets[1]", "It uses $__rate_interval, which SlashObserve does not support.")
}

func TestTempoQueriesImport(t *testing.T) {
	cases := []struct {
		name, typ, target, want string
	}{
		{"trace id", "traces", `{"refId":"A","queryType":"traceId","query":"4bf92f3577b34da6a3ce929d0e0e4736"}`, "4bf92f3577b34da6a3ce929d0e0e4736"},
		{"trace id variable", "traces", `{"refId":"A","query":"${trace_id}"}`, "${trace_id}"},
		{"traceql table with defaults", "table", `{"refId":"A","queryType":"traceql","query":"{resource.service.name=\"checkout\"}","limit":20,"spss":3,"tableType":"traces"}`, `{resource.service.name="checkout"}`},
		{"search form state beside traceql", "table", `{"refId":"A","queryType":"traceql","query":"{}","filters":[{"id":"service-name","tag":"service.name","operator":"=","scope":"resource"}],"groupBy":[{"id":"g"}],"metricsQueryType":"range","step":"1m","exemplars":5}`, "{}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := panelOn(tempoRef, tc.typ, tc.target)
			if strings.Contains(tc.target, "trace_id") {
				doc = strings.Replace(doc, `"panels":`, `"templating":{"list":[{"name":"trace_id","type":"textbox","query":"abc"}]},"panels":`, 1)
			}
			res := convertDoc(t, doc)
			entry, _ := res.Spec.FindPanel("p1")
			if got := onlyQuery(t, *entry.Panel).Query; got != tc.want {
				t.Fatalf("query = %q, want %q", got, tc.want)
			}
			wantClean(t, panelLine(t, res, "p1"))
		})
	}
}

func TestTempoSearchLimitsAreReported(t *testing.T) {
	_, line := importedOn(t, tempoRef, "table", `{"refId":"A","queryType":"traceql","query":"{}","limit":50,"tableType":"spans"}`)
	wantReported(t, line, "targets[0].limit", decisionTempoSearchLimits.reason)
	wantReported(t, line, "targets[0].tableType", decisionTempoSearchLimits.reason)
}

func TestTempoTargetsOutsideTraceQLAreDropped(t *testing.T) {
	cases := []struct {
		name, target, reason string
	}{
		{"search builder", `{"refId":"A","queryType":"traceqlSearch","filters":[{"id":"x"}]}`, decisionTempoSearchBuilder.reason},
		{"native search", `{"refId":"A","queryType":"nativeSearch","serviceName":"checkout"}`, decisionTempoSearchBuilder.reason},
		{"service graph", `{"refId":"A","queryType":"serviceMap"}`, decisionTempoServiceGraph.reason},
		{"upload", `{"refId":"A","queryType":"upload"}`, decisionTraceUpload.reason},
		{"metrics", `{"refId":"A","queryType":"traceql","query":"{} | rate()"}`, decisionTraceQLMetrics.reason},
		{"metrics with spaces", `{"refId":"A","query":"{ status = error } |  quantile_over_time (duration, .9)"}`, decisionTraceQLMetrics.reason},
		{"unknown query type", `{"refId":"A","queryType":"flame","query":"{}"}`, "The query type flame is not supported yet."},
		{"filters only", `{"refId":"A","filters":[{"id":"x"}]}`, emptyQueryReason},
		{"clear", `{"refId":"A","queryType":"clear","query":"{}"}`, emptyQueryReason},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertDoc(t, panelOn(tempoRef, "traces", tc.target))
			entry, _ := res.Spec.FindPanel("p1")
			line := panelLine(t, res, "p1")
			if entry.Panel.Type != spec.Placeholder || !slices.Contains(line.Reasons, tc.reason) {
				t.Fatalf("panel = %+v, reasons %q, want a placeholder with %q", entry.Panel, line.Reasons, tc.reason)
			}
		})
	}
}

func TestTempoMetricsQueryTypeAloneKeepsTheQuery(t *testing.T) {
	panel, line := importedOn(t, tempoRef, "table", `{"refId":"A","queryType":"traceql","query":"{ duration > 1s }","metricsQueryType":"instant"}`)
	if onlyQuery(t, panel).Query != "{ duration > 1s }" {
		t.Fatalf("queries = %+v", panel.Queries)
	}
	wantClean(t, line)
}

func variablesOn(t *testing.T, opts Options, variables ...string) *Result {
	t.Helper()
	return convertWith(t, dashboardJSON(templating(variables...), panelJSON(1, 0, 0, 12, 4, "")), opts)
}

func wantNote(t *testing.T, res *Result, name, reason string) {
	t.Helper()
	notes := notesFor(res, "variable "+name)
	if !slices.ContainsFunc(notes, func(n Note) bool { return n.Reason == reason }) {
		t.Fatalf("notes = %+v, want %q", notes, reason)
	}
}

func hasVariableNamed(res *Result, name string) bool {
	return slices.ContainsFunc(res.Spec.Variables, func(v spec.Variable) bool { return v.Name == name })
}

func TestLokiQueryVariablesImport(t *testing.T) {
	res := convertVariables(t,
		`{"name":"pod","type":"query","datasource":`+lokiRef+`,"query":"label_values({app=\"web\"}, pod)"}`,
		`{"name":"app","type":"query","datasource":`+lokiRef+`,"query":{"type":1,"label":"app","stream":"{namespace=\"shop\"}","refId":"LokiVariableQueryEditor-VariableQuery"}}`,
		`{"name":"ns","type":"query","datasource":`+lokiRef+`,"query":{"type":1,"label":"namespace","stream":""}}`,
	)
	for name, want := range map[string]string{
		"pod": `label_values({app="web"}, pod)`,
		"app": `label_values({namespace="shop"}, app)`,
		"ns":  "label_values(namespace)",
	} {
		v := variableNamed(t, res, name)
		if v.Type != spec.QueryVariable || v.Datasource != "loki" || v.Query != want {
			t.Errorf("%s = %+v, want a query variable on loki with %q", name, v, want)
		}
	}
}

func TestLokiLabelNamesVariableKeepsItsSavedValues(t *testing.T) {
	res := convertVariables(t, `{"name":"labels","type":"query","datasource":`+lokiRef+`,"query":{"type":0},"options":[{"value":"app"},{"value":"pod"}]}`)
	if v := variableNamed(t, res, "labels"); v.Type != spec.CustomVariable || !slices.Equal(v.Options, spec.PlainOptions("app", "pod")) {
		t.Fatalf("variable = %+v, want the saved values", v)
	}
	wantNote(t, res, "labels", "This Loki variable query is not supported yet; its saved values are kept as a fixed list.")
}

func TestTempoQueryVariableKeepsItsSavedValues(t *testing.T) {
	res := convertVariables(t,
		`{"name":"svc","type":"query","datasource":`+tempoRef+`,"query":"x","options":[{"value":"checkout"}]}`,
		`{"name":"empty","type":"query","datasource":`+tempoRef+`,"query":"x"}`,
	)
	if v := variableNamed(t, res, "svc"); v.Type != spec.CustomVariable || !slices.Equal(v.Options, spec.PlainOptions("checkout")) {
		t.Fatalf("variable = %+v", v)
	}
	wantNote(t, res, "svc", "Query variables on Tempo are not supported yet; its saved values are kept as a fixed list.")
	wantNote(t, res, "empty", "Query variables on Tempo are not supported yet, and it has no saved values to keep.")
}

func TestQueryVariableOnAKindWithNoConfiguredDatasource(t *testing.T) {
	res := variablesOn(t, promOnly, `{"name":"pod","type":"query","datasource":`+lokiRef+`,"query":"label_values(pod)","options":[{"value":"web-0"}]}`)
	if v := variableNamed(t, res, "pod"); v.Type != spec.CustomVariable {
		t.Fatalf("variable = %+v, want the saved values", v)
	}
	wantNote(t, res, "pod", "It queries Loki, and no Loki datasource is configured; its saved values are kept as a fixed list.")
}

func TestQueryVariableOnAnUnsupportedPluginKeepsItsNote(t *testing.T) {
	res := convertVariables(t,
		`{"name":"f","type":"query","datasource":{"uid":"other","type":"influxdb"},"query":"x","options":[{"value":"a"}]}`,
		`{"name":"g","type":"query","datasource":{"uid":"other","type":"influxdb"},"query":"x"}`,
	)
	wantNote(t, res, "f", "It queries Influxdb, which is not supported yet; its saved values are kept as a fixed list.")
	wantNote(t, res, "g", "It queries Influxdb, which is not supported yet, and it has no saved values to keep.")
}

func TestAdhocFiltersOnLokiAreDropped(t *testing.T) {
	res := convertVariables(t, `{"name":"f","type":"adhoc","datasource":`+lokiRef+`,"filters":[]}`)
	if hasVariableNamed(res, "f") {
		t.Fatal("the Loki ad hoc variable was kept")
	}
	wantNote(t, res, "f", "Ad hoc filters on Loki are not supported yet.")
}

func TestLokiDatasourceVariable(t *testing.T) {
	variable := `{"name":"logs","type":"datasource","query":"loki"}`
	res := convertVariables(t, variable)
	v := variableNamed(t, res, "logs")
	if v.Type != spec.DatasourceVariable || v.DatasourceType != spec.DatasourceType(kindLoki) || v.Default == nil || !slices.Equal(v.Default.Values, []string{"loki"}) {
		t.Fatalf("variable = %+v, want a loki datasource variable defaulting to the match", v)
	}

	res = variablesOn(t, promOnly, variable)
	if hasVariableNamed(res, "logs") {
		t.Fatal("the datasource variable was kept with no Loki configured")
	}
	wantNote(t, res, "logs", "No Loki datasource is configured.")
}
