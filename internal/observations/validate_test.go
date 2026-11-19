package observations

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("error %v is not an invalid input", err)
	}
	return invalid.Code
}

func TestTitlesAreTrimmedAndBoundedInRunes(t *testing.T) {
	good := map[string]string{"  Checkout 5xx  ": "Checkout 5xx", strings.Repeat("é", 200): strings.Repeat("é", 200)}
	for in, want := range good {
		if got, err := validateTitle(in); err != nil || got != want {
			t.Errorf("%q: %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "   ", strings.Repeat("é", 201), "a\x00b"} {
		if _, err := validateTitle(in); err == nil || codeOf(t, err) != CodeInvalidTitle {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestSummariesAreOneLineAndBounded(t *testing.T) {
	if got, err := validateSummary("  rolled back v42  "); err != nil || got != "rolled back v42" {
		t.Errorf("trimmed: %q, %v", got, err)
	}
	if _, err := validateSummary(strings.Repeat("x", 500)); err != nil {
		t.Errorf("500 runes: %v", err)
	}
	for _, in := range []string{"", " ", strings.Repeat("x", 501), "one\ntwo", "one\rtwo", "a\x00"} {
		if _, err := validateSummary(in); err == nil || codeOf(t, err) != CodeInvalidSummary {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestARangeIsBothOrNeitherOrderedAndBounded(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	before, after := now.Add(-time.Hour), now
	tooWide := now.Add(-queryproxy.MaxRange - time.Second)
	widest := now.Add(-queryproxy.MaxRange)
	if err := validateRange(nil, nil); err != nil {
		t.Errorf("neither: %v", err)
	}
	if err := validateRange(&before, &after); err != nil {
		t.Errorf("ordered: %v", err)
	}
	if err := validateRange(&widest, &after); err != nil {
		t.Errorf("the widest allowed range: %v", err)
	}
	for name, c := range map[string][2]*time.Time{
		"only from": {&before, nil}, "only to": {nil, &after}, "reversed": {&after, &before}, "equal": {&after, &after}, "too wide": {&tooWide, &after},
	} {
		if err := validateRange(c[0], c[1]); err == nil || codeOf(t, err) != CodeInvalidRange {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestATextBodyIsMarkdownOfOneToEightThousandRunes(t *testing.T) {
	body := func(markdown string) json.RawMessage {
		raw, err := json.Marshal(map[string]string{"markdown": markdown})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if got, err := textBody(body(strings.Repeat("é", 8000))); err != nil || len([]rune(got.Markdown)) != 8000 {
		t.Errorf("8000 runes: %v", err)
	}
	for name, raw := range map[string]json.RawMessage{
		"8001 runes":     body(strings.Repeat("é", 8001)),
		"empty":          body(""),
		"blank":          body(" \n\t"),
		"NUL":            body("a\x00b"),
		"no markdown":    json.RawMessage(`{}`),
		"extra key":      json.RawMessage(`{"markdown":"x","html":"y"}`),
		"not an object":  json.RawMessage(`"x"`),
		"null":           json.RawMessage(`null`),
		"two values":     json.RawMessage(`{"markdown":"x"} {}`),
		"wrong type":     json.RawMessage(`{"markdown":1}`),
		"null markdown":  json.RawMessage(`{"markdown":null}`),
		"trailing comma": json.RawMessage(`{"markdown":"x",}`),
	} {
		if _, err := textBody(raw); err == nil || codeOf(t, err) != CodeInvalidBlock {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func panelJSON(kind string) string {
	switch kind {
	case "logs":
		return `{"type":"logs","title":"Errors","datasource":"loki","queries":[{"ref":"A","query":"{app=\"shop\"} |= \"error\""}]}`
	case "traces":
		return `{"type":"traces","title":"Slow","datasource":"tempo","queries":[{"ref":"A","query":"{ duration > 1s }"}]}`
	case "text":
		return `{"type":"text","title":"Notes","content":"hello"}`
	case "row":
		return `{"type":"row","title":"Row"}`
	}
	return fmt.Sprintf(`{"type":%q,"title":"Rate","datasource":"prom","queries":[{"ref":"A","query":"up"}]}`, kind)
}

func panelBlock(panel string) json.RawMessage {
	return json.RawMessage(`{"panel":` + panel + `}`)
}

func TestAPanelBodyAcceptsDataPanelsOfAnyDatasourceKind(t *testing.T) {
	for _, kind := range []string{"timeseries", "stat", "table", "logs", "traces"} {
		got, err := panelBody(panelBlock(panelJSON(kind)))
		if err != nil {
			t.Errorf("%s: %v", kind, err)
			continue
		}
		if !json.Valid(got.Panel) {
			t.Errorf("%s: stored panel is not JSON: %s", kind, got.Panel)
		}
	}
}

func TestAPanelBodyRefusesWhatIsNotADataPanel(t *testing.T) {
	text, err := panelBody(panelBlock(panelJSON("text")))
	if err == nil || codeOf(t, err) != CodeInvalidPanel {
		t.Fatalf("text panel: %v", err)
	}
	var invalid *InvalidInputError
	errors.As(err, &invalid)
	if len(invalid.Issues) != 1 || invalid.Issues[0].Path != "/panel/type" || !strings.Contains(invalid.Issues[0].Message, "text block") {
		t.Errorf("text panel issues %+v (%v)", invalid.Issues, text)
	}
	for _, kind := range []string{"row"} {
		if _, err := panelBody(panelBlock(panelJSON(kind))); err == nil || codeOf(t, err) != CodeInvalidPanel {
			t.Errorf("%s: %v", kind, err)
		}
	}
}

func TestAPanelBodyReportsSpecIssuesAtThePanelPath(t *testing.T) {
	_, err := panelBody(panelBlock(`{"type":"timeseries","queries":[{"ref":"A","query":"up"}],"unit":"nonsense-unit"}`))
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeInvalidPanel || len(invalid.Issues) == 0 {
		t.Fatalf("got %v", err)
	}
	for _, issue := range invalid.Issues {
		if strings.HasPrefix(issue.Path, "/panels/0") {
			t.Errorf("issue still points into the wrapper: %+v", issue)
		}
	}
}

func TestAPanelBodyRefusesNULAndOversizeAndBadShape(t *testing.T) {
	oversize := `{"type":"timeseries","title":"` + strings.Repeat("x", spec.MaxSuggestedPanelBytes) + `","queries":[{"ref":"A","query":"up"}]}`
	cases := map[string]struct {
		raw  json.RawMessage
		code string
	}{
		"NUL in a string":  {panelBlock(`{"type":"timeseries","title":"a\u0000b","datasource":"prom","queries":[{"ref":"A","query":"up"}]}`), CodeInvalidPanel},
		"over the cap":     {panelBlock(oversize), CodeInvalidPanel},
		"no panel":         {json.RawMessage(`{}`), CodeInvalidBlock},
		"panel not object": {json.RawMessage(`{"panel":[]}`), CodeInvalidBlock},
		"extra key":        {json.RawMessage(`{"panel":{},"x":1}`), CodeInvalidBlock},
		"only from":        {json.RawMessage(`{"panel":` + panelJSON("stat") + `,"from":"2026-11-01T09:00:00Z"}`), CodeInvalidRange},
		"bad time":         {json.RawMessage(`{"panel":` + panelJSON("stat") + `,"from":"yesterday","to":"today"}`), CodeInvalidBlock},
	}
	for name, c := range cases {
		if _, err := panelBody(c.raw); err == nil || codeOf(t, err) != c.code {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
}

func TestAPanelBodyKeepsItsRangeInUTC(t *testing.T) {
	raw := json.RawMessage(`{"panel":` + panelJSON("stat") + `,"from":"2026-11-01T09:00:00+02:00","to":"2026-11-01T10:00:00+02:00"}`)
	got, err := panelBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.From == nil || got.From.Location() != time.UTC || got.From.Hour() != 7 {
		t.Errorf("from %v", got.From)
	}
}

func TestTheSnapshotCapIsTwoMebibytes(t *testing.T) {
	if MaxSnapshotBytes != 2097152 {
		t.Errorf("cap %d", MaxSnapshotBytes)
	}
}

const servicePanel = `{"type":"timeseries","title":"Rate","datasource":"prom","queries":[{"ref":"A","query":"sum(rate(errors{service=~\"$service\"}[$__rate_interval]))"}]}`

func TestAPanelThatUsesAVariableNeedsItsValueAndNamesTheMissingOne(t *testing.T) {
	_, err := panelBody(panelBlock(servicePanel))
	if err == nil || codeOf(t, err) != CodeVariableMissing || !strings.Contains(err.Error(), `"service"`) {
		t.Fatalf("a panel with an undeclared variable: %v", err)
	}
	var refused *InvalidInputError
	if !errors.As(err, &refused) || len(refused.Issues) == 0 {
		t.Errorf("the spec issues are kept: %+v", err)
	}
}

func TestVariableValuesBecomeVariablesThatAreStoredWithThePanel(t *testing.T) {
	raw := json.RawMessage(`{"panel":` + servicePanel + `,"variable_values":{"service":["checkout","cart"]}}`)
	got, err := panelBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.VariableValues != nil || len(got.Variables) != 1 {
		t.Fatalf("variables %+v values %+v", got.Variables, got.VariableValues)
	}
	v := got.Variables[0]
	if v.Name != "service" || v.Type != spec.CustomVariable || !v.Multi || v.Default == nil || !v.Default.List || len(v.Default.Values) != 2 {
		t.Errorf("variable %+v", v)
	}
	stored, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "variable_values") {
		t.Errorf("stored body keeps the input form: %s", stored)
	}
	again, err := panelBody(stored)
	if err != nil || len(again.Variables) != 1 {
		t.Fatalf("a stored body reads back: %v %+v", err, again)
	}
}

func TestVariableValuesAreCheckedAndNotMixedWithVariables(t *testing.T) {
	for name, raw := range map[string]string{
		"an empty list":  `{"panel":` + servicePanel + `,"variable_values":{"service":[]}}`,
		"an empty value": `{"panel":` + servicePanel + `,"variable_values":{"service":[""]}}`,
		"both forms":     `{"panel":` + servicePanel + `,"variable_values":{"service":["a"]},"variables":[{"name":"service","type":"constant","value":"a"}]}`,
	} {
		if _, err := panelBody(json.RawMessage(raw)); err == nil || codeOf(t, err) != CodeInvalidBlock {
			t.Errorf("%s: %v", name, err)
		}
	}
	many := make(map[string][]string)
	for i := range MaxBlockVariables + 1 {
		many[fmt.Sprintf("v%d", i)] = []string{"x"}
	}
	body, _ := json.Marshal(map[string]any{"panel": json.RawMessage(servicePanel), "variable_values": many})
	if _, err := panelBody(body); err == nil || codeOf(t, err) != CodeInvalidBlock {
		t.Errorf("too many variables: %v", err)
	}
}

func TestADashboardVariableWithADefaultIsKeptAsSent(t *testing.T) {
	raw := json.RawMessage(`{"panel":` + servicePanel + `,"variables":[{"name":"service","type":"custom","multi":true,"includeAll":true,"options":[{"text":"checkout","value":"checkout"},{"text":"cart","value":"cart"}],"default":"$__all"}]}`)
	got, err := panelBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Variables) != 1 || got.Variables[0].Default == nil || got.Variables[0].Default.Values[0] != spec.SelectAll {
		t.Errorf("variables %+v", got.Variables)
	}
}
