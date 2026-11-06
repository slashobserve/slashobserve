package grafana

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	esRef = `{"type":"elasticsearch","uid":"es"}`
	osRef = `{"type":"grafana-opensearch-datasource","uid":"os"}`

	countOverTime = `"metrics":[{"id":"1","type":"count"}],"bucketAggs":[{"id":"2","type":"date_histogram","field":"@timestamp","settings":{"interval":"auto","min_doc_count":"0","trimEdges":"0"}}]`
)

func esTarget(members string) string {
	return `{"refId":"A","timeField":"@timestamp",` + members + `}`
}

func TestElasticTargetsConvert(t *testing.T) {
	cases := []struct {
		name, ds, target string
		want             spec.Query
		note             string
	}{
		{"count over time is a lucene range query", esRef, esTarget(`"query":"service:web",` + countOverTime), spec.Query{Query: "lucene:service:web"}, ""},
		{"an empty query matches everything", esRef, esTarget(`"query":"",` + countOverTime), spec.Query{Query: "lucene:*"}, ""},
		{"a target without metrics or buckets is the default count", esRef, esTarget(`"query":"service:web"`), spec.Query{Query: "lucene:service:web"}, ""},
		{"a logs query type is lines", esRef, esTarget(`"query":"level:error","queryType":"logs","metrics":[{"id":"1","type":"logs","settings":{"limit":"500"}}],"bucketAggs":[]`), spec.Query{Query: "lucene:level:error", Instant: true}, ""},
		{"a raw data metric is lines", esRef, esTarget(`"query":"level:error","metrics":[{"id":"1","type":"raw_data","settings":{"size":"500"}}],"bucketAggs":[]`), spec.Query{Query: "lucene:level:error", Instant: true}, ""},
		{"a raw document is lines with a note", esRef, esTarget(`"query":"level:error","queryType":"raw_document","metrics":[{"id":"1","type":"raw_document"}],"bucketAggs":[]`), spec.Query{Query: "lucene:level:error", Instant: true}, rawDocumentReason},
		{"an OpenSearch logs query is lines", osRef, esTarget(`"query":"level:error","luceneQueryType":"Logs","format":"logs",` + countOverTime), spec.Query{Query: "lucene:level:error", Instant: true}, ""},
		{"a terms size of zero becomes 500 and the term order the key", esRef, esTarget(`"query":"*","metrics":[{"id":"1","type":"count"}],"bucketAggs":[{"id":"2","type":"terms","field":"host","settings":{"size":"0","order":"asc","orderBy":"_term","missing":"none"}}]`),
			spec.Query{Query: `{"aggs":{"2":{"terms":{"field":"host","missing":"none","order":{"_key":"asc"},"size":500}}},"query":{"query_string":{"analyze_wildcard":true,"query":"*"}}}`}, ""},
		{"filters label their queries", esRef, esTarget(`"query":"*","metrics":[{"id":"1","type":"count"}],"bucketAggs":[{"id":"2","type":"filters","settings":{"filters":[{"query":"status:5*","label":"errors"},{"query":"status:2*","label":""}]}}]`),
			spec.Query{Query: `{"aggs":{"2":{"filters":{"filters":{"errors":{"query_string":{"analyze_wildcard":true,"query":"status:5*"}},"status:2*":{"query_string":{"analyze_wildcard":true,"query":"status:2*"}}}}}},"query":{"query_string":{"analyze_wildcard":true,"query":"*"}}}`}, ""},
		{"a histogram keeps its interval", esRef, esTarget(`"query":"*","metrics":[{"id":"1","type":"count"}],"bucketAggs":[{"id":"2","type":"histogram","field":"bytes","settings":{"interval":"1000","min_doc_count":"0"}}]`),
			spec.Query{Query: `{"aggs":{"2":{"histogram":{"field":"bytes","interval":1000,"min_doc_count":0}}},"query":{"query_string":{"analyze_wildcard":true,"query":"*"}}}`}, ""},
		{"percentiles keep their percents and the field placeholder is literal", esRef, esTarget(`"query":"*","alias":"{{field}} {{metric}}","metrics":[{"id":"1","type":"percentiles","field":"duration","settings":{"percents":["50","95"]}}],"bucketAggs":[]`),
			spec.Query{Query: `{"aggs":{"1":{"percentiles":{"field":"duration","percents":[50,95]}}},"query":{"query_string":{"analyze_wildcard":true,"query":"*"}}}`, Legend: "duration {{metric}}"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := importedOn(t, tc.ds, "timeseries", tc.target)
			got := onlyQuery(t, panel)
			got.Ref = ""
			if got != tc.want {
				t.Fatalf("query = %+v\nwant    %+v", got, tc.want)
			}
			if tc.note == "" {
				wantClean(t, line)
				return
			}
			if line.Status != statusPartial || !slices.Contains(line.Reasons, tc.note) {
				t.Fatalf("line = %+v, want a note %q", line, tc.note)
			}
		})
	}
}

func TestElasticTargetsThatCannotConvertAreDropped(t *testing.T) {
	cases := []struct{ name, ds, target, reason string }{
		{"PPL", osRef, esTarget(`"queryType":"PPL","query":"source = logs | stats count() by host","format":"table"`), decisionElasticPPL.reason},
		{"a geohash grid", esRef, esTarget(`"query":"*","metrics":[{"id":"1","type":"count"}],"bucketAggs":[{"id":"2","type":"geohash_grid","field":"location","settings":{"precision":"3"}}]`), decisionElasticGeoNested.reason},
		{"a nested bucket", esRef, esTarget(`"query":"*","metrics":[{"id":"1","type":"count"}],"bucketAggs":[{"id":"2","type":"nested","field":"comments"}]`), decisionElasticGeoNested.reason},
		{"only a top_metrics metric", esRef, esTarget(`"query":"*","metrics":[{"id":"1","type":"top_metrics","settings":{"metrics":["bytes"],"order":"desc","orderBy":"@timestamp"}}],"bucketAggs":[]`), decisionElasticTopMetrics.reason},
		{"only a scripted metric", esRef, esTarget(`"query":"*","metrics":[{"id":"1","type":"avg","field":"bytes","settings":{"script":"_value * 8"}}],"bucketAggs":[]`), decisionElasticScripts.reason},
		{"a date histogram with no field and no time field", esRef, `{"refId":"A","query":"*","metrics":[{"id":"1","type":"sum","field":"bytes"}],"bucketAggs":[{"id":"2","type":"date_histogram"}]}`, noTimeFieldReason},
		{"an unknown bucket aggregation", esRef, esTarget(`"query":"*","metrics":[{"id":"1","type":"count"}],"bucketAggs":[{"id":"2","type":"range","field":"bytes"}]`), "The bucket aggregation range is not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := importedOn(t, tc.ds, "timeseries", tc.target)
			if panel.Type != spec.Placeholder || !strings.HasSuffix(panel.Reason, " "+tc.reason) {
				t.Fatalf("panel reason = %q, want a placeholder ending with %q", panel.Reason, tc.reason)
			}
			if line.Status != statusPlaceholder || len(line.Queries) != 1 || line.Queries[0].Dropped == nil || *line.Queries[0].Dropped != tc.reason {
				t.Fatalf("line = %+v, want the query dropped with %q", line, tc.reason)
			}
		})
	}
}

func TestElasticMetricsDroppedBesideOthersAreNoted(t *testing.T) {
	cases := []struct{ name, target, path, reason, want string }{
		{"top_metrics beside an average", esTarget(`"query":"*","metrics":[{"id":"1","type":"avg","field":"bytes"},{"id":"3","type":"top_metrics","settings":{"metrics":["bytes"]}}],"bucketAggs":[]`), "targets[0].metrics[1].type", decisionElasticTopMetrics.reason,
			`{"aggs":{"1":{"avg":{"field":"bytes"}}},"query":{"query_string":{"analyze_wildcard":true,"query":"*"}}}`},
		{"a script beside a sum", esTarget(`"query":"*","metrics":[{"id":"1","type":"sum","field":"bytes"},{"id":"3","type":"avg","field":"bytes","settings":{"script":"_value * 8"}}],"bucketAggs":[]`), "targets[0].metrics[1].settings.script", decisionElasticScripts.reason,
			`{"aggs":{"1":{"sum":{"field":"bytes"}}},"query":{"query_string":{"analyze_wildcard":true,"query":"*"}}}`},
		{"a hidden metric", esTarget(`"query":"*","metrics":[{"id":"1","type":"sum","field":"bytes","hide":true},{"id":"3","type":"count"}],"bucketAggs":[]`), "targets[0].metrics[0].hide", decisionElasticHiddenMetrics.reason,
			`{"aggs":{"1":{"sum":{"field":"bytes"}}},"query":{"query_string":{"analyze_wildcard":true,"query":"*"}}}`},
		{"an unknown setting", esTarget(`"query":"*","metrics":[{"id":"1","type":"sum","field":"bytes","settings":{"missing":"0"}}],"bucketAggs":[]`), "targets[0].metrics[0].settings.missing", genericQueryNote,
			`{"aggs":{"1":{"sum":{"field":"bytes"}}},"query":{"query_string":{"analyze_wildcard":true,"query":"*"}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := importedOn(t, esRef, "timeseries", tc.target)
			if got := onlyQuery(t, panel).Query; got != tc.want {
				t.Fatalf("query = %s\nwant    %s", got, tc.want)
			}
			status := statusPartial
			if isDecisionReason(tc.reason) {
				status = statusImported
			}
			if line.Status != status || !slices.Equal(line.Paths, []string{tc.path}) || !slices.Equal(line.Reasons, []string{tc.reason}) {
				t.Fatalf("line = %+v, want %s %s noted with %q", line, status, tc.path, tc.reason)
			}
		})
	}
}

func TestElasticKindsWithoutADatasourceArePlaceholders(t *testing.T) {
	cases := []struct{ name, ds, reason string }{
		{"elasticsearch", esRef, "No Elasticsearch datasource is configured."},
		{"opensearch", osRef, "No OpenSearch datasource is configured."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertWith(t, panelOn(tc.ds, "timeseries", esTarget(`"query":"*",`+countOverTime)), promOnly)
			entry, _ := res.Spec.FindPanel("p1")
			if !res.Ready() || entry.Panel.Type != spec.Placeholder || entry.Panel.Reason != tc.reason {
				t.Fatalf("panel = %+v, ready %v, want a placeholder with %q", entry.Panel, res.Ready(), tc.reason)
			}
		})
	}
}

func TestElasticDashboardImportsWithoutGaps(t *testing.T) {
	doc := `{"title":"Search","schemaVersion":39,"panels":[
		{"id":1,"type":"timeseries","title":"Requests","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":` + esRef + `,"targets":[` + esTarget(`"query":"service:web",`+countOverTime) + `]},
		{"id":2,"type":"table","title":"Hosts","gridPos":{"x":12,"y":0,"w":12,"h":8},"datasource":` + esRef + `,"targets":[` + esTarget(`"query":"*","metrics":[{"id":"1","type":"count"}],"bucketAggs":[{"id":"2","type":"terms","field":"host","settings":{"size":"10"}}]`) + `]},
		{"id":3,"type":"logs","title":"Errors","gridPos":{"x":0,"y":8,"w":24,"h":8},"datasource":` + osRef + `,"targets":[` + esTarget(`"query":"level:error","luceneQueryType":"Logs","metrics":[{"id":"1","type":"logs"}],"bucketAggs":[]`) + `]}
	]}`
	res := convertDoc(t, doc)
	if !res.Ready() {
		t.Fatalf("result is not ready: %+v", res.Datasources)
	}
	for _, line := range res.Report.Panels {
		if line.Status != statusImported || len(line.Paths) != 0 {
			t.Errorf("panel %s = %+v, want imported with nothing reported", line.PanelID, line)
		}
	}
	for _, panel := range res.Spec.Panels {
		if panel.Type == spec.Placeholder {
			t.Errorf("panel %s is a placeholder: %s", panel.ID, panel.Reason)
		}
	}
	if got := sourceNamed(t, res, "os"); got.Match == nil || *got.Match != "os" {
		t.Fatalf("source = %+v, want matched to os", got)
	}
}

func TestElasticQueryVariables(t *testing.T) {
	cases := []struct {
		name, query string
		want        string
		note        string
	}{
		{"terms with a query", `"{\"find\":\"terms\",\"field\":\"service.name\",\"query\":\"env:prod\"}"`, "label_values(lucene:env:prod, service.name)", ""},
		{"terms without a query", `"{\"find\":\"terms\",\"field\":\"service.name\"}"`, "label_values(service.name)", ""},
		{"the object form", `{"query":"{\"find\":\"terms\",\"field\":\"host\"}","refId":"ElasticsearchVariableQueryEditor-VariableQuery"}`, "label_values(host)", ""},
		{"a size is noted", `"{\"find\":\"terms\",\"field\":\"host\",\"size\":50}"`, "label_values(host)", "The size of a terms variable is not kept; the datasource's value limit applies."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertVariables(t, `{"name":"v","type":"query","datasource":`+esRef+`,"query":`+tc.query+`}`)
			v := variableNamed(t, res, "v")
			if v.Type != spec.QueryVariable || v.Query != tc.want || v.Datasource != "es" {
				t.Fatalf("variable = %+v, want query %q on es", v, tc.want)
			}
			if tc.note == "" {
				if notes := notesFor(res, "variable v"); len(notes) != 0 {
					t.Fatalf("notes = %+v, want none", notes)
				}
				return
			}
			wantNote(t, res, "v", tc.note)
		})
	}
}

func TestElasticVariablesWithoutAnEquivalent(t *testing.T) {
	res := convertVariables(t,
		`{"name":"fields","type":"query","datasource":`+esRef+`,"query":"{\"find\":\"fields\",\"type\":\"keyword\"}","options":[{"value":"host"}]}`,
		`{"name":"text","type":"query","datasource":`+osRef+`,"query":"host","options":[{"value":"a"}]}`,
		`{"name":"empty","type":"query","datasource":`+esRef+`,"query":"{\"find\":\"terms\"}"}`,
	)
	if hasVariableNamed(res, "fields") {
		t.Fatal("the field-name variable was kept")
	}
	wantNote(t, res, "fields", decisionElasticFieldVariables.reason)
	if v := variableNamed(t, res, "text"); v.Type != spec.CustomVariable || !slices.Equal(v.Options, spec.PlainOptions("a")) {
		t.Fatalf("variable = %+v, want the saved values as a custom variable", v)
	}
	wantNote(t, res, "text", "This OpenSearch variable query is not supported yet; its saved values are kept as a fixed list.")
	wantNote(t, res, "empty", "This Elasticsearch variable query is not supported yet, and it has no saved values to keep.")
}

func TestElasticAdhocFiltersConvertLikeClickHouse(t *testing.T) {
	res := convertVariables(t, adhocWith(esRef, `{"key":"service.name","operator":"=","value":"web"},{"key":"bytes","operator":">","value":"100","condition":"OR"}`))
	v := variableNamed(t, res, "v")
	want := []spec.AdhocFilter{{Key: "service.name", Operator: "=", Value: "web"}, {Key: "bytes", Operator: ">", Value: "100", Condition: "or"}}
	if !v.ApplyToAll || v.Datasource != "es" || v.Default == nil || !slices.Equal(v.Default.Filters, want) {
		t.Fatalf("variable = %+v, want applyToAll with %+v", v, want)
	}
	if notes := notesFor(res, "variable v"); len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}
	res = convertVariables(t, adhocWith(osRef, `{"key":"bytes","operator":"=|","value":"100"}`))
	notes := notesFor(res, "variable v")
	if len(notes) != 1 || !strings.Contains(notes[0].Reason, "on OpenSearch; saved filters that use =| are not kept.") {
		t.Fatalf("notes = %+v, want the OpenSearch operator refusal", notes)
	}
}

func TestInferTypeReadsBucketAggs(t *testing.T) {
	res := convertDoc(t, panelOn(`"Search"`, "timeseries", esTarget(`"query":"*",`+countOverTime)))
	s := sourceNamed(t, res, "Search")
	if s.Type == nil || *s.Type != "elasticsearch" || s.Kind == nil || *s.Kind != kindElastic {
		t.Fatalf("source = %+v, want elasticsearch inferred from bucketAggs", s)
	}
}
