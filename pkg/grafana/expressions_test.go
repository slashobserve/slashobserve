package grafana

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy/expr"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const exprDatasource = `"datasource":{"type":"__expr__","uid":"__expr__"}`

func expressionTarget(refID, members string) string {
	return `{"refId":"` + refID + `",` + exprDatasource + `,` + members + `}`
}

func expressionPanel(t *testing.T, targets ...string) (spec.Panel, PanelLine) {
	t.Helper()
	res := convertDoc(t, panelDoc("timeseries", `"targets":[`+strings.Join(targets, ",")+`]`))
	if !res.Ready() {
		t.Fatalf("result is not ready: %+v", res.Datasources)
	}
	data, err := json.Marshal(res.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Parse(data); err != nil {
		t.Fatalf("the converted Spec does not parse: %v\n%s", err, data)
	}
	entry, ok := res.Spec.FindPanel("p1")
	if !ok {
		t.Fatalf("no panel p1 in the Spec\n%s", data)
	}
	return *entry.Panel, panelLine(t, res, "p1")
}

func floats(values ...float64) []float64 { return values }

func TestExpressionsImportAsSpecExpressions(t *testing.T) {
	replace := 7.0
	zero := 0.0
	cases := []struct {
		name    string
		members string
		want    spec.Expression
	}{
		{"math keeps its references", `"type":"math","expression":"$A * 2"`, spec.Expression{Type: "math", Math: "$A * 2"}},
		{"math drops the braces around a reference", `"type":"math","expression":"${A} * 2 + abs($A)"`, spec.Expression{Type: "math", Math: "$A * 2 + abs($A)"}},
		{"math with a comparison", `"type":"math","expression":"$A > 0.5"`, spec.Expression{Type: "math", Math: "$A > 0.5"}},
		{"math with an exponent number", `"type":"math","expression":"$A * 1e3"`, spec.Expression{Type: "math", Math: "$A * 1e3"}},
		{"math with every supported function", `"type":"math","expression":"ceil(floor(round(sqrt(exp(log(is_null($A))))))) + is_nan($A) + is_number($A) + is_inf($A)"`,
			spec.Expression{Type: "math", Math: "ceil(floor(round(sqrt(exp(log(is_null($A))))))) + is_nan($A) + is_number($A) + is_inf($A)"}},
		{"reduce strict", `"type":"reduce","expression":"A","reducer":"max","settings":{"mode":""}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "max"}},
		{"reduce without settings", `"type":"reduce","expression":"A","reducer":"last"`, spec.Expression{Type: "reduce", Input: "A", Reducer: "last"}},
		{"reduce reads a dollar input", `"type":"reduce","expression":"$A","reducer":"mean","settings":{"mode":""}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "mean"}},
		{"reduce reads a braced input", `"type":"reduce","expression":"${A}","reducer":"median","settings":{"mode":""}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "median"}},
		{"reduce count", `"type":"reduce","expression":"A","reducer":"count","settings":{"mode":""}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "count"}},
		{"reduce dropNN", `"type":"reduce","expression":"A","reducer":"max","settings":{"mode":"dropNN"}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "max", Mode: "dropNonNumbers"}},
		{"reduce replaceNN", `"type":"reduce","expression":"A","reducer":"sum","settings":{"mode":"replaceNN","replaceWithValue":7}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "sum", Mode: "replaceNonNumbers", ReplaceWith: &replace}},
		{"reduce replaceNN with zero", `"type":"reduce","expression":"A","reducer":"min","settings":{"mode":"replaceNN","replaceWithValue":0}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "min", Mode: "replaceNonNumbers", ReplaceWith: &zero}},
		{"reduce ignores a stray replace value", `"type":"reduce","expression":"A","reducer":"min","settings":{"mode":"dropNN","replaceWithValue":3}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "min", Mode: "dropNonNumbers"}},
		{"resample mean and pad", `"type":"resample","expression":"A","window":"1m","downsampler":"mean","upsampler":"pad"`, spec.Expression{Type: "resample", Input: "A", Window: "1m", Downsampler: "mean", Upsampler: "pad"}},
		{"resample last", `"type":"resample","expression":"A","window":"30s","downsampler":"last","upsampler":"pad"`, spec.Expression{Type: "resample", Input: "A", Window: "30s", Downsampler: "last", Upsampler: "pad"}},
		{"resample min", `"type":"resample","expression":"A","window":"5m","downsampler":"min","upsampler":"pad"`, spec.Expression{Type: "resample", Input: "A", Window: "5m", Downsampler: "min", Upsampler: "pad"}},
		{"resample max", `"type":"resample","expression":"A","window":"1h","downsampler":"max","upsampler":"pad"`, spec.Expression{Type: "resample", Input: "A", Window: "1h", Downsampler: "max", Upsampler: "pad"}},
		{"resample sum", `"type":"resample","expression":"A","window":"1m","downsampler":"sum","upsampler":"pad"`, spec.Expression{Type: "resample", Input: "A", Window: "1m", Downsampler: "sum", Upsampler: "pad"}},
		{"resample backfilling", `"type":"resample","expression":"A","window":"1m","downsampler":"mean","upsampler":"backfilling"`, spec.Expression{Type: "resample", Input: "A", Window: "1m", Downsampler: "mean", Upsampler: "backfill"}},
		{"resample fillna", `"type":"resample","expression":"A","window":"1m","downsampler":"mean","upsampler":"fillna"`, spec.Expression{Type: "resample", Input: "A", Window: "1m", Downsampler: "mean", Upsampler: "fillNA"}},
		{"threshold gt", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"gt","params":[1]}}]`, spec.Expression{Type: "threshold", Input: "A", Evaluator: "gt", Params: floats(1)}},
		{"threshold lt", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"lt","params":[2.5]}}]`, spec.Expression{Type: "threshold", Input: "A", Evaluator: "lt", Params: floats(2.5)}},
		{"threshold within_range", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"within_range","params":[1,5]}}]`, spec.Expression{Type: "threshold", Input: "A", Evaluator: "withinRange", Params: floats(1, 5)}},
		{"threshold outside_range", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"outside_range","params":[1,5]}}]`, spec.Expression{Type: "threshold", Input: "A", Evaluator: "outsideRange", Params: floats(1, 5)}},
		{"threshold within_range_included", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"within_range_included","params":[1,5]}}]`, spec.Expression{Type: "threshold", Input: "A", Evaluator: "withinRangeIncluded", Params: floats(1, 5)}},
		{"threshold outside_range_included", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"outside_range_included","params":[1,5]}}]`, spec.Expression{Type: "threshold", Input: "A", Evaluator: "outsideRangeIncluded", Params: floats(1, 5)}},
		{"threshold gte is a math comparison", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"gte","params":[1]}}]`, spec.Expression{Type: "math", Math: "$A >= 1"}},
		{"threshold lte is a math comparison", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"lte","params":[-2.5]}}]`, spec.Expression{Type: "math", Math: "$A <= -2.5"}},
		{"threshold eq is a math comparison", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"eq","params":[1]}}]`, spec.Expression{Type: "math", Math: "$A == 1"}},
		{"threshold ne is a math comparison", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"ne","params":[0]}}]`, spec.Expression{Type: "math", Math: "$A != 0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := expressionPanel(t, `{"refId":"A","expr":"up"}`, expressionTarget("B", tc.members))
			if len(panel.Queries) != 2 {
				t.Fatalf("queries = %+v, want the data query and the expression", panel.Queries)
			}
			got := panel.Queries[1]
			if got.Ref != "B" || got.Expression == nil || got.Query != "" || got.Datasource != "" {
				t.Fatalf("expression query = %+v, want ref B, an expression and no query text or datasource", got)
			}
			if !reflect.DeepEqual(*got.Expression, tc.want) {
				t.Fatalf("expression = %+v, want %+v", *got.Expression, tc.want)
			}
			if line.Status != statusImported || len(line.Paths) != 0 {
				t.Fatalf("status = %s, paths = %v, want imported with no report", line.Status, line.Paths)
			}
			if line.Queries[1].Dropped != nil || line.Queries[1].Datasource != nil {
				t.Fatalf("query line = %+v, want kept and with no datasource", line.Queries[1])
			}
			if panel.Datasource != "prom" || panel.Queries[0].Datasource != "" {
				t.Fatalf("panel datasource = %q, first query datasource = %q, want prom from the data query", panel.Datasource, panel.Queries[0].Datasource)
			}
		})
	}
}

func TestExpressionsKeepTheirHiddenFlag(t *testing.T) {
	panel, _ := expressionPanel(t, `{"refId":"A","expr":"up"}`, expressionTarget("B", `"hide":true,"type":"math","expression":"$A * 2"`))
	if !panel.Queries[1].Hide {
		t.Fatalf("query = %+v, want hidden", panel.Queries[1])
	}
}

func TestExpressionsFollowARenamedTarget(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz"
	cases := []struct {
		name    string
		members string
		want    spec.Expression
	}{
		{"math", `"type":"math","expression":"$` + long + ` * 2 + ${` + long + `}"`, spec.Expression{Type: "math", Math: "$A * 2 + $A"}},
		{"reduce", `"type":"reduce","expression":"` + long + `","reducer":"max","settings":{"mode":""}`, spec.Expression{Type: "reduce", Input: "A", Reducer: "max"}},
		{"resample", `"type":"resample","expression":"` + long + `","window":"1m","downsampler":"mean","upsampler":"pad"`, spec.Expression{Type: "resample", Input: "A", Window: "1m", Downsampler: "mean", Upsampler: "pad"}},
		{"threshold", `"type":"threshold","expression":"` + long + `","conditions":[{"evaluator":{"type":"gt","params":[1]}}]`, spec.Expression{Type: "threshold", Input: "A", Evaluator: "gt", Params: floats(1)}},
		{"threshold as math", `"type":"threshold","expression":"` + long + `","conditions":[{"evaluator":{"type":"gte","params":[1]}}]`, spec.Expression{Type: "math", Math: "$A >= 1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := expressionPanel(t, `{"refId":"`+long+`","expr":"up"}`, expressionTarget("B", tc.members))
			if !reflect.DeepEqual(*panel.Queries[1].Expression, tc.want) {
				t.Fatalf("expression = %+v, want %+v", *panel.Queries[1].Expression, tc.want)
			}
			if !slices.Contains(line.Queries[0].Rewrites, "refId "+long+" -> A") {
				t.Fatalf("rewrites = %v, want the rename reported", line.Queries[0].Rewrites)
			}
		})
	}
}

func TestExpressionsFollowARenamedExpression(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz"
	panel, _ := expressionPanel(t,
		`{"refId":"A","expr":"up"}`,
		expressionTarget(long, `"type":"reduce","expression":"A","reducer":"max","settings":{"mode":""}`),
		expressionTarget("C", `"type":"math","expression":"$`+long+` + 1"`),
	)
	if panel.Queries[1].Ref != "B" || panel.Queries[2].Expression.Math != "$B + 1" {
		t.Fatalf("queries = %+v, want the second expression to read B", panel.Queries)
	}
}

func TestExpressionsReadATargetWithoutARefIDByItsPosition(t *testing.T) {
	panel, _ := expressionPanel(t,
		`{"expr":"up"}`,
		expressionTarget("B", `"type":"math","expression":"$A * 2"`),
	)
	if panel.Queries[1].Expression == nil || panel.Queries[1].Expression.Math != "$A * 2" {
		t.Fatalf("queries = %+v, want the expression to read the first query", panel.Queries)
	}
}

func TestExpressionsChainThroughOtherExpressions(t *testing.T) {
	panel, line := expressionPanel(t,
		`{"refId":"A","expr":"up"}`,
		expressionTarget("B", `"type":"reduce","expression":"A","reducer":"max","settings":{"mode":""}`),
		expressionTarget("C", `"type":"threshold","expression":"B","conditions":[{"evaluator":{"type":"gt","params":[1]}}]`),
	)
	if len(panel.Queries) != 3 || panel.Queries[2].Expression.Input != "B" {
		t.Fatalf("queries = %+v, want the threshold to read the reduce", panel.Queries)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, want imported", line.Status)
	}
}

func TestExpressionsThatCannotBeImportedAreDroppedWithAReason(t *testing.T) {
	cases := []struct {
		name    string
		members string
		reason  string
	}{
		{"classic conditions", `"type":"classic_conditions","conditions":[{"type":"query","query":{"params":["A"]},"reducer":{"type":"avg","params":[]},"evaluator":{"type":"gt","params":[5]},"operator":{"type":"and"}}]`,
			"Classic conditions belong to alert rules, which SlashObserve does not import."},
		{"sql", `"type":"sql","expression":"SELECT * FROM A LIMIT 10"`, "SQL expressions are not supported."},
		{"an unknown type", `"type":"fancy","expression":"A"`, "The expression type fancy is not supported yet."},
		{"no type", `"expression":"A"`, "The expression has no expression type."},
		{"an unknown math function", `"type":"math","expression":"sin($A) * 2"`, "The function sin is not supported yet."},
		{"a constant function", `"type":"math","expression":"$A + nan()"`, "The function nan is not supported yet."},
		{"a function name without a call", `"type":"math","expression":"$A + abs"`, "The function abs is not supported yet."},
		{"an unreadable reference", `"type":"math","expression":"$__interval * 2"`, "The math expression holds a reference SlashObserve cannot read."},
		{"an empty math expression", `"type":"math","expression":"  "`, "The math expression is empty."},
		{"an unknown reducer", `"type":"reduce","expression":"A","reducer":"avg"`, "The reducer avg is not supported yet."},
		{"no reducer", `"type":"reduce","expression":"A"`, "The expression has no reducer."},
		{"an unknown reduce mode", `"type":"reduce","expression":"A","reducer":"max","settings":{"mode":"other"}`, "The reduce mode other is not supported yet."},
		{"replaceNN without a value", `"type":"reduce","expression":"A","reducer":"max","settings":{"mode":"replaceNN"}`, "The reduce mode replaceNN has no value to replace with."},
		{"no input", `"type":"reduce","reducer":"max"`, "The expression has no input query."},
		{"a resample window with a variable", `"type":"resample","expression":"A","window":"$__interval","downsampler":"mean","upsampler":"pad"`, "The resample window $__interval is not supported yet."},
		{"an unknown downsampler", `"type":"resample","expression":"A","window":"1m","downsampler":"median","upsampler":"pad"`, "The downsampler median is not supported yet."},
		{"an unknown upsampler", `"type":"resample","expression":"A","window":"1m","downsampler":"mean","upsampler":"linear"`, "The upsampler linear is not supported yet."},
		{"a threshold hysteresis", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"gt","params":[5]},"unloadEvaluator":{"type":"lt","params":[3]}}]`, "A threshold with a separate unload condition is not supported."},
		{"an unknown threshold evaluator", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"near","params":[5]}}]`, "The threshold evaluator near is not supported yet."},
		{"a range with one number", `"type":"threshold","expression":"A","conditions":[{"evaluator":{"type":"within_range","params":[5]}}]`, "The threshold evaluator within_range needs 2 number(s)."},
		{"a threshold without conditions", `"type":"threshold","expression":"A"`, "The expression has no threshold evaluator."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := expressionPanel(t, `{"refId":"A","expr":"up"}`, expressionTarget("B", tc.members))
			if len(panel.Queries) != 1 || panel.Queries[0].Expression != nil {
				t.Fatalf("queries = %+v, want only the data query", panel.Queries)
			}
			if line.Status != statusPartial || line.Queries[1].Dropped == nil || *line.Queries[1].Dropped != tc.reason {
				t.Fatalf("status = %s, query line = %+v, want partial and dropped with %q", line.Status, line.Queries[1], tc.reason)
			}
			if !slices.Contains(line.Reasons, tc.reason) || !slices.Contains(line.Paths, "targets[1]") {
				t.Fatalf("reasons = %v, paths = %v, want %q at targets[1]", line.Reasons, line.Paths, tc.reason)
			}
		})
	}
}

func TestAnExpressionNamingADroppedQueryIsDropped(t *testing.T) {
	reason := "This expression uses query A, which could not be imported."
	panel, line := expressionPanel(t,
		`{"refId":"A","expr":""}`,
		`{"refId":"B","expr":"up"}`,
		expressionTarget("C", `"type":"math","expression":"$A + $B"`),
		expressionTarget("D", `"type":"reduce","expression":"A","reducer":"max","settings":{"mode":""}`),
		expressionTarget("E", `"type":"reduce","expression":"B","reducer":"max","settings":{"mode":""}`),
	)
	if len(panel.Queries) != 2 || panel.Queries[1].Expression == nil || panel.Queries[1].Expression.Input != "B" {
		t.Fatalf("queries = %+v, want the query B and the reduce over it", panel.Queries)
	}
	for _, i := range []int{2, 3} {
		if got := line.Queries[i].Dropped; got == nil || *got != reason && i == 3 {
			t.Fatalf("query line %d = %+v, want dropped", i, line.Queries[i])
		}
	}
	if got := line.Queries[2].Dropped; *got != reason {
		t.Fatalf("math dropped = %q, want %q", *got, reason)
	}
}

func TestAnExpressionNamingAnUnknownRefIsDropped(t *testing.T) {
	panel, line := expressionPanel(t,
		`{"refId":"A","expr":"up"}`,
		expressionTarget("B", `"type":"math","expression":"$Z * 2"`),
	)
	want := "This expression uses query Z, which could not be imported."
	if len(panel.Queries) != 1 || line.Queries[1].Dropped == nil || *line.Queries[1].Dropped != want {
		t.Fatalf("queries = %+v, line = %+v, want dropped with %q", panel.Queries, line.Queries[1], want)
	}
}

func TestAnExpressionNamingADroppedExpressionIsDropped(t *testing.T) {
	_, line := expressionPanel(t,
		`{"refId":"A","expr":"up"}`,
		expressionTarget("B", `"type":"sql","expression":"SELECT 1"`),
		expressionTarget("C", `"type":"reduce","expression":"B","reducer":"max","settings":{"mode":""}`),
	)
	want := "This expression uses query B, which could not be imported."
	if line.Queries[2].Dropped == nil || *line.Queries[2].Dropped != want {
		t.Fatalf("query line = %+v, want dropped with %q", line.Queries[2], want)
	}
}

func TestExpressionsThatDependOnEachOtherAreDropped(t *testing.T) {
	panel, line := expressionPanel(t,
		`{"refId":"A","expr":"up"}`,
		expressionTarget("B", `"type":"math","expression":"$C + 1"`),
		expressionTarget("C", `"type":"math","expression":"$B + 1"`),
		expressionTarget("D", `"type":"math","expression":"$D + 1"`),
	)
	if len(panel.Queries) != 1 {
		t.Fatalf("queries = %+v, want only the data query", panel.Queries)
	}
	for _, i := range []int{1, 2, 3} {
		if line.Queries[i].Dropped == nil {
			t.Fatalf("query line %d = %+v, want dropped", i, line.Queries[i])
		}
	}
	if got := *line.Queries[3].Dropped; got != "This expression depends on itself through its inputs." {
		t.Fatalf("self reference dropped = %q", got)
	}
}

func TestAHiddenDroppedExpressionIsNotReported(t *testing.T) {
	_, line := expressionPanel(t,
		`{"refId":"A","expr":"up"}`,
		expressionTarget("B", `"hide":true,"type":"classic_conditions","conditions":[]`),
	)
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths = %v, want imported with no report", line.Status, line.Paths)
	}
}

func TestAPanelOfOnlyExpressionsWithoutAQueryBecomesAPlaceholder(t *testing.T) {
	res := convertDoc(t, panelDoc("timeseries", `"targets":[`+expressionTarget("A", `"type":"math","expression":"1 + 1"`)+`]`))
	line := panelLine(t, res, "p1")
	if line.Type != string(spec.Placeholder) {
		t.Fatalf("line = %+v, want a placeholder", line)
	}
}

func TestConstantMathNextToADataQueryIsDroppedWhenNoQueryRemains(t *testing.T) {
	res := convertDoc(t, panelDoc("timeseries", `"targets":[{"refId":"A","expr":""},`+expressionTarget("B", `"type":"math","expression":"1 + 1"`)+`]`))
	line := panelLine(t, res, "p1")
	if line.Type != string(spec.Placeholder) {
		t.Fatalf("line = %+v, want a placeholder", line)
	}
	if got := line.Queries[1].Dropped; got == nil || *got != "This expression uses no query." {
		t.Fatalf("query line = %+v, want dropped for using no query", line.Queries[1])
	}
}

func TestExpressionsImportOnAStatPanel(t *testing.T) {
	res := convertDoc(t, panelDoc("stat", `"targets":[{"refId":"A","expr":"up"},`+expressionTarget("B", `"type":"reduce","expression":"A","reducer":"last","settings":{"mode":""}`)+`]`))
	entry, ok := res.Spec.FindPanel("p1")
	if !ok || len(entry.Panel.Queries) != 2 || entry.Panel.Queries[1].Expression == nil {
		t.Fatalf("panel = %+v, want the expression kept on a stat panel", entry.Panel)
	}
}

func TestMathFunctionsMatchTheQueryProxy(t *testing.T) {
	frame := datasource.Frame{Fields: []datasource.Field{{Name: "Value", Type: datasource.FieldNumber, Values: []any{1.0}}}}
	inputs := map[string][]datasource.Frame{"A": {frame}}
	evaluate := func(name string) error {
		_, err := expr.Eval(spec.Expression{Type: spec.ExpressionMath, Math: name + "($A)"}, inputs, datasource.TimeRange{}, datasource.Limits{})
		return err
	}
	for _, name := range mathFunctions {
		if err := evaluate(name); err != nil {
			t.Errorf("the proxy does not evaluate %s: %v", name, err)
		}
	}
	for _, name := range []string{"sin", "nan", "ln"} {
		err := evaluate(name)
		if err == nil || !errors.Is(err, expr.ErrExpression) || !strings.Contains(err.Error(), "unknown function") {
			t.Errorf("the proxy accepts %s: %v", name, err)
		}
		if _, _, reason := rewriteMath(name+"($A)", func(ref string) string { return ref }); reason == "" {
			t.Errorf("the importer accepts %s", name)
		}
	}
}
