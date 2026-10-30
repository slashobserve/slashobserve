package spec_test

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func expressionPanel(queries ...string) string {
	body := ""
	for i, q := range queries {
		if i > 0 {
			body += ","
		}
		body += q
	}
	return statPanelHead + `,"datasource":"prom","queries":[` + body + `]}`
}

const (
	sourceQuery = `{"ref":"A","query":"up"}`
	hiddenQuery = `{"ref":"B","query":"up","hide":true}`
)

func expressionOf(ref, expression string) string {
	return `{"ref":"` + ref + `","expression":` + expression + `}`
}

func TestMathRefs(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"$A * 2", []string{"A"}},
		{"abs($A) / $B", []string{"A", "B"}},
		{"${A} + ${B_1} + $A", []string{"A", "B_1"}},
		{"$A2*$b", []string{"A2", "b"}},
		{"1 + 2", nil},
		{"$ + ${ + ${A", nil},
		{"$1", nil},
		{"${A}${", []string{"A"}},
	}
	for _, tc := range cases {
		if got := spec.MathRefs(tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("MathRefs(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestExpressionInputs(t *testing.T) {
	cases := []struct {
		name string
		e    spec.Expression
		want []string
	}{
		{"math", spec.Expression{Type: "math", Math: "$A + $B"}, []string{"A", "B"}},
		{"reduce", spec.Expression{Type: "reduce", Input: "A"}, []string{"A"}},
		{"resample", spec.Expression{Type: "resample", Input: "B"}, []string{"B"}},
		{"threshold", spec.Expression{Type: "threshold", Input: "C"}, []string{"C"}},
	}
	for _, tc := range cases {
		if got := tc.e.Inputs(); !slices.Equal(got, tc.want) {
			t.Errorf("%s: Inputs() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestExpressionQueriesParse(t *testing.T) {
	cases := []struct {
		name   string
		panels string
	}{
		{"math over a hidden query", expressionPanel(sourceQuery, hiddenQuery, expressionOf("E", `{"type":"math","math":"$A * 2 + abs($B)"}`))},
		{"reduce", expressionPanel(sourceQuery, expressionOf("E", `{"type":"reduce","input":"A","reducer":"max"}`))},
		{"reduce replacing non numbers", expressionPanel(sourceQuery, expressionOf("E", `{"type":"reduce","input":"A","reducer":"mean","mode":"replaceNonNumbers","replaceWith":0}`))},
		{"reduce dropping non numbers", expressionPanel(sourceQuery, expressionOf("E", `{"type":"reduce","input":"A","reducer":"last","mode":"dropNonNumbers"}`))},
		{"resample", expressionPanel(sourceQuery, expressionOf("E", `{"type":"resample","input":"A","window":"1m","downsampler":"mean","upsampler":"pad"}`))},
		{"threshold", expressionPanel(sourceQuery, expressionOf("E", `{"type":"threshold","input":"A","evaluator":"gt","params":[0.5]}`))},
		{"threshold range", expressionPanel(sourceQuery, expressionOf("E", `{"type":"threshold","input":"A","evaluator":"outsideRangeIncluded","params":[1,2]}`))},
		{"expression over an expression", expressionPanel(sourceQuery, expressionOf("E", `{"type":"reduce","input":"A","reducer":"sum"}`), expressionOf("F", `{"type":"math","math":"$E / 2"}`))},
		{"hidden expression", expressionPanel(sourceQuery, `{"ref":"E","hide":true,"expression":{"type":"math","math":"$A"}}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := spec.Parse(dsDashboard("", tc.panels))
			if err != nil {
				t.Fatalf("Parse error = %v, want none", err)
			}
			queries := d.Panels[0].Queries
			if last := queries[len(queries)-1]; !last.IsExpression() || last.Query != "" {
				t.Fatalf("last query = %+v, want an expression without query text", last)
			}
			if queries[0].IsExpression() {
				t.Fatalf("first query %+v is an expression", queries[0])
			}
		})
	}
}

func TestExpressionSemanticIssues(t *testing.T) {
	cases := []struct {
		name   string
		panels string
		want   []spec.Issue
	}{
		{
			name:   "math names an unknown ref",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"math","math":"$A + $Z"}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression/math", Code: spec.CodeUnknownQueryRef}},
		},
		{
			name:   "math names an unknown ref in braces",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"math","math":"${Z}"}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression/math", Code: spec.CodeUnknownQueryRef}},
		},
		{
			name:   "reduce input is unknown",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"reduce","input":"Z","reducer":"max"}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression/input", Code: spec.CodeUnknownQueryRef}},
		},
		{
			name:   "resample input is unknown",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"resample","input":"Z","window":"1m","downsampler":"mean","upsampler":"pad"}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression/input", Code: spec.CodeUnknownQueryRef}},
		},
		{
			name:   "threshold input is unknown",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"threshold","input":"Z","evaluator":"lt","params":[1]}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression/input", Code: spec.CodeUnknownQueryRef}},
		},
		{
			name: "two expressions that depend on each other are one cycle, reported once",
			panels: expressionPanel(sourceQuery,
				expressionOf("E", `{"type":"math","math":"$F + 1"}`),
				expressionOf("F", `{"type":"math","math":"$E + 1"}`)),
			want: []spec.Issue{{Path: "/panels/0/queries/1/expression", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "an expression that names itself is a cycle",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"reduce","input":"E","reducer":"max"}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression", Code: spec.CodeInvalidReference}},
		},
		{
			name: "an expression that only reads a cycle is not reported",
			panels: expressionPanel(sourceQuery,
				expressionOf("E", `{"type":"math","math":"$F"}`),
				expressionOf("F", `{"type":"math","math":"$E"}`),
				expressionOf("G", `{"type":"math","math":"$E"}`)),
			want: []spec.Issue{{Path: "/panels/0/queries/1/expression", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "an expression ref repeats a query ref",
			panels: expressionPanel(sourceQuery, expressionOf("A", `{"type":"math","math":"1"}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/ref", Code: spec.CodeDuplicateQueryRef}},
		},
		{
			name:   "replaceNonNumbers without replaceWith",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"reduce","input":"A","reducer":"max","mode":"replaceNonNumbers"}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression", Code: spec.CodeSchema}},
		},
		{
			name:   "replaceWith without replaceNonNumbers",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"reduce","input":"A","reducer":"max","replaceWith":1}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression/replaceWith", Code: spec.CodeSchema}},
		},
		{
			name:   "gt with two params",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"threshold","input":"A","evaluator":"gt","params":[1,2]}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression/params", Code: spec.CodeSchema}},
		},
		{
			name:   "a range evaluator with one param",
			panels: expressionPanel(sourceQuery, expressionOf("E", `{"type":"threshold","input":"A","evaluator":"withinRange","params":[1]}`)),
			want:   []spec.Issue{{Path: "/panels/0/queries/1/expression/params", Code: spec.CodeSchema}},
		},
		{
			name: "an expression in a row child",
			panels: `{"id":"r1","type":"row","title":"Row","gridPos":{"x":0,"y":0,"w":24,"h":1},"panels":[` +
				`{"id":"c1","type":"stat","title":"Up","gridPos":{"x":0,"y":1,"w":6,"h":4},"datasource":"prom","queries":[` + sourceQuery + `,` + expressionOf("E", `{"type":"math","math":"$Z"}`) + `]}]}`,
			want: []spec.Issue{{Path: "/panels/0/panels/0/queries/1/expression/math", Code: spec.CodeUnknownQueryRef}},
		},
		{
			name:   "an override matching no query or expression ref",
			panels: statPanelHead + `,"datasource":"prom","queries":[` + sourceQuery + `,` + expressionOf("E", `{"type":"math","math":"$A"}`) + `],"overrides":[{"match":{"query":"X"},"color":"critical"}]}`,
			want:   []spec.Issue{{Path: "/panels/0/overrides/0/match/query", Code: spec.CodeUnknownQueryRef}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := parseIssues(t, dsDashboard("", tc.panels))
			got := make([]spec.Issue, len(issues))
			for i, is := range issues {
				got[i] = spec.Issue{Path: is.Path, Code: is.Code}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("issues = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestOverrideMayMatchAnExpressionRef(t *testing.T) {
	panel := statPanelHead + `,"datasource":"prom","queries":[` + sourceQuery + `,` + expressionOf("E", `{"type":"math","math":"$A"}`) + `],"overrides":[{"match":{"query":"E"},"color":"critical"}]}`
	if _, err := spec.Parse(dsDashboard("", panel)); err != nil {
		t.Fatalf("Parse error = %v, want none", err)
	}
}

func TestExpressionSchemaRejections(t *testing.T) {
	cases := []struct {
		name  string
		query string
		path  string
	}{
		{"neither query nor expression", `{"ref":"E"}`, "/panels/0/queries/1"},
		{"both query and expression", `{"ref":"E","query":"up","expression":{"type":"math","math":"1"}}`, "/panels/0/queries/1"},
		{"expression without ref", `{"expression":{"type":"math","math":"1"}}`, "/panels/0/queries/1"},
		{"datasource on an expression", `{"ref":"E","datasource":"prom","expression":{"type":"math","math":"1"}}`, "/panels/0/queries/1"},
		{"unknown expression type", `{"ref":"E","expression":{"type":"sql","sql":"select 1"}}`, "/panels/0/queries/1/expression"},
		{"unknown reducer", `{"ref":"E","expression":{"type":"reduce","input":"A","reducer":"p99"}}`, "/panels/0/queries/1/expression/reducer"},
		{"empty math", `{"ref":"E","expression":{"type":"math","math":""}}`, "/panels/0/queries/1/expression/math"},
		{"window without a unit", `{"ref":"E","expression":{"type":"resample","input":"A","window":"60","downsampler":"mean","upsampler":"pad"}}`, "/panels/0/queries/1/expression/window"},
		{"three threshold params", `{"ref":"E","expression":{"type":"threshold","input":"A","evaluator":"gt","params":[1,2,3]}}`, "/panels/0/queries/1/expression/params"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := parseIssues(t, dsDashboard("", expressionPanel(sourceQuery, tc.query)))
			if !slices.ContainsFunc(issues, func(is spec.Issue) bool { return is.Code == spec.CodeSchema && is.Path == tc.path }) {
				t.Fatalf("issues = %+v, want a schema issue at %s", issues, tc.path)
			}
		})
	}
}
