package grafana

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestTracesSpanFiltersImport(t *testing.T) {
	tagPaths := []string{"options.spanFilters.tags[0].key", "options.spanFilters.tags[0].operator", "options.spanFilters.tags[0].value"}
	cases := []struct {
		name, filters, display string
		paths                  []string
	}{
		{name: "search text", filters: `"query":"  checkout "`, display: `{"spanSearch":"checkout"}`},
		{name: "empty search is nothing", filters: `"query":""`, display: "null"},
		{name: "matches only", filters: `"matchesOnly":true`, display: `{"matchesOnly":true}`},
		{name: "critical path only", filters: `"criticalPathOnly":true`, display: `{"criticalPathOnly":true}`},
		{name: "false flags are Grafana's defaults", filters: `"matchesOnly":false,"criticalPathOnly":false`, display: "null"},
		{name: "services", filters: `"serviceName":["web","db","web"]`, display: `{"serviceNames":["web","db"]}`},
		{name: "a lone service name", filters: `"serviceName":"web"`, display: `{"serviceNames":["web"]}`},
		{name: "excluded services", filters: `"serviceName":["web"],"serviceNameOperator":"!="`, display: `{"serviceNames":["web"],"excludeServices":true}`},
		{name: "equal operator is the default", filters: `"serviceName":["web"],"serviceNameOperator":"="`, display: `{"serviceNames":["web"]}`},
		{name: "operator without names is kept", filters: `"spanNameOperator":"!="`, display: `{"excludeSpanNames":true}`},
		{name: "span names", filters: `"spanName":["GET /","db"],"spanNameOperator":"!="`, display: `{"spanNames":["GET /","db"],"excludeSpanNames":true}`},
		{name: "durations with units", filters: `"from":"10ms","to":"1.5s"`, display: `{"minSpanDuration":10,"maxSpanDuration":1500}`},
		{name: "microseconds and minutes", filters: `"from":"250us","to":"2m"`, display: `{"minSpanDuration":0.25,"maxSpanDuration":120000}`},
		{name: "every filter together", filters: `"query":"x","matchesOnly":true,"criticalPathOnly":true,"serviceName":["a"],"spanName":["b"],"from":"1ms","to":"1h"`, display: `{"spanSearch":"x","serviceNames":["a"],"spanNames":["b"],"minSpanDuration":1,"maxSpanDuration":3600000,"matchesOnly":true,"criticalPathOnly":true}`},
		{name: "unknown operator is reported", filters: `"serviceNameOperator":"=~"`, display: "null", paths: []string{"options.spanFilters.serviceNameOperator"}},
		{name: "duration without a unit is reported", filters: `"from":"10"`, display: "null", paths: []string{"options.spanFilters.from"}},
		{name: "duration beyond a day is reported", filters: `"to":"25h"`, display: "null", paths: []string{"options.spanFilters.to"}},
		{name: "names that are not text are reported", filters: `"spanName":["a",3]`, display: "null", paths: []string{"options.spanFilters.spanName"}},
		{name: "search that is too long is reported", filters: `"query":"` + strings.Repeat("a", 201) + `"`, display: "null", paths: []string{"options.spanFilters.query"}},
		{name: "tag filters are reported", filters: `"tags":[{"key":"http.status","operator":"=","value":"500"}]`, display: "null", paths: tagPaths},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("traces", upTarget+`,"options":{"spanFilters":{`+tc.filters+`}}`))
			if panel.Type != spec.Traces {
				t.Fatalf("type = %s", panel.Type)
			}
			if got := displayJSON(t, panel); got != tc.display {
				t.Errorf("display = %s, want %s", got, tc.display)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v (reasons %v)", line.Paths, tc.paths, line.Reasons)
			}
		})
	}
}

func TestTracesTooManyFilterNamesAreReported(t *testing.T) {
	names := make([]string, 51)
	for i := range names {
		names[i] = fmt.Sprintf("%q", fmt.Sprintf("service-%d", i))
	}
	list := "[" + strings.Join(names, ",") + "]"
	panel, line := convertedPanel(t, panelDoc("traces", upTarget+`,"options":{"spanFilters":{"serviceName":`+list+`}}`))
	if got := displayJSON(t, panel); got != "null" {
		t.Errorf("display = %s, want null", got)
	}
	if want := []string{"options.spanFilters.serviceName"}; !slices.Equal(line.Paths, want) {
		t.Errorf("paths = %v, want %v", line.Paths, want)
	}
}

func TestFlameGraphUnitAndDecimalsImportWithoutReports(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("flamegraph", upTarget+`,"fieldConfig":{"defaults":{"unit":"ms","decimals":2},"overrides":[]}`))
	if panel.Type != spec.FlameGraph {
		t.Fatalf("type = %s", panel.Type)
	}
	if len(line.Paths) != 0 {
		t.Errorf("paths = %v (reasons %v)", line.Paths, line.Reasons)
	}
}
