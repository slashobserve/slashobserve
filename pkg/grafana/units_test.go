package grafana

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func fieldConfigUnitDoc(t *testing.T, unit string) string {
	t.Helper()
	text, err := json.Marshal(unit)
	if err != nil {
		t.Fatal(err)
	}
	return panelDoc("timeseries", upTarget+","+fmt.Sprintf(`"fieldConfig":{"defaults":{"unit":%s},"overrides":[]}`, text))
}

func TestUnitsImportAsWrittenOrAsTheSuffixGrafanaShows(t *testing.T) {
	longText := strings.Repeat("x", 50)
	for _, tc := range []struct {
		name    string
		unit    string
		want    string
		reports bool
	}{
		{name: "a new id", unit: "radbq", want: "radbq"},
		{name: "an id with a non-ASCII letter", unit: "currencyμBTC", want: "currencyμBTC"},
		{name: "a time format", unit: "time:YYYY-MM-DD HH:mm:ss", want: "time:YYYY-MM-DD HH:mm:ss"},
		{name: "a time format with a leading space", unit: "time: YYYY-MM-DD HH:mm:ss", want: "time: YYYY-MM-DD HH:mm:ss"},
		{name: "an SI symbol", unit: "si:mF", want: "si:mF"},
		{name: "a count text", unit: "count:req", want: "count:req"},
		{name: "a currency symbol", unit: "currency:€", want: "currency:€"},
		{name: "a suffix", unit: "suffix:rds", want: "suffix:rds"},
		{name: "an unknown id", unit: "rds", want: "suffix:rds"},
		{name: "an unknown id with a slash", unit: "read/s", want: "suffix:read/s"},
		{name: "an unknown id with a colon inside", unit: "a:b", want: "suffix:a:b"},
		{name: "an unknown id of 20 characters", unit: strings.Repeat("x", 20), want: "suffix:" + strings.Repeat("x", 20)},
		{name: "an unknown id of 21 characters", unit: strings.Repeat("x", 21), reports: true},
		{name: "a text of 50 characters", unit: longText, reports: true},
		{name: "a time format of 41 characters", unit: "time:" + strings.Repeat("Y", 41), reports: true},
		{name: "an empty time format", unit: "time:", reports: true},
		{name: "an empty suffix", unit: "suffix:", reports: true},
		{name: "a suffix of 21 characters", unit: "suffix:" + strings.Repeat("x", 21), reports: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, fieldConfigUnitDoc(t, tc.unit))
			if tc.reports {
				if panel.Unit != "" || !slices.Contains(line.Paths, "fieldConfig.defaults.unit") {
					t.Fatalf("unit = %q, paths = %v", panel.Unit, line.Paths)
				}
				return
			}
			if panel.Unit != tc.want || len(line.Paths) != 0 {
				t.Fatalf("unit = %q, paths = %v, want %q", panel.Unit, line.Paths, tc.want)
			}
		})
	}
}

func TestLegacyUnitsFollowTheSameRules(t *testing.T) {
	for _, tc := range []struct {
		unit string
		want string
	}{
		{"radbq", "radbq"},
		{"time:YYYY-MM-DD", "time:YYYY-MM-DD"},
		{"rds", "suffix:rds"},
		{strings.Repeat("x", 50), ""},
	} {
		doc := panelDoc("singlestat", upTarget+","+fmt.Sprintf(`"format":%q`, tc.unit))
		panel, line := convertedPanel(t, doc)
		if panel.Unit != tc.want {
			t.Errorf("format %q: unit = %q, want %q", tc.unit, panel.Unit, tc.want)
		}
		if reported := slices.Contains(line.Paths, "format"); reported != (tc.want == "") {
			t.Errorf("format %q: paths = %v", tc.unit, line.Paths)
		}
	}
}
