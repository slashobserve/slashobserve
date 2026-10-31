package grafana

import (
	"slices"
	"testing"
)

func TestSinglestatKeysTheStatMigrationLeavesBehindImportWithoutANote(t *testing.T) {
	cases := []struct{ name, members string }{
		{"connected nulls, colored postfix and a postfix beside a unit", `"format":"s","nullPointMode":"connected","colorPostfix":false,"colorPrefix":null,"postfix":"s","postfixFontSize":"50%"`},
		{"null mode", `"nullPointMode":"null","colorPrefix":true`},
		{"editor sizes and sparkline colors", `"valueFontSize":"80%","prefixFontSize":"50%","sparkline":{"show":true,"full":true,"fillColor":"rgba(31, 118, 189, 0.18)"}`},
		{"mapping editor choices", `"mappingTypes":[{"name":"value to text","value":1},{"name":"range to text","value":2}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := singlestatPanel(t, tc.members)
			if line.Status != statusImported || len(line.Paths) != 0 {
				t.Fatalf("status = %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
			}
		})
	}
}

func TestSinglestatPostfixBesideAUnitKeepsTheUnit(t *testing.T) {
	panel, _ := singlestatPanel(t, `"format":"s","postfix":"s","prefix":"~"`)
	if got := panel.Unit; got != "s" {
		t.Fatalf("unit = %q, want s", got)
	}
}

func TestSinglestatPostfixWithoutAUnitBecomesTheUnit(t *testing.T) {
	cases := []struct{ name, members, want string }{
		{"postfix", `"postfix":" req"`, "suffix: req"},
		{"prefix", `"format":"none","prefix":"~"`, "prefix:~"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := singlestatPanel(t, tc.members)
			if got := panel.Unit; got != tc.want {
				t.Fatalf("unit = %q, want %q", got, tc.want)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestSinglestatPrefixAndPostfixTogetherAreNoted(t *testing.T) {
	panel, line := singlestatPanel(t, `"prefix":"$","postfix":"/s"`)
	if got := panel.Unit; got != "suffix:/s" {
		t.Fatalf("unit = %q, want suffix:/s", got)
	}
	if !slices.Contains(line.Paths, "postfix") {
		t.Fatalf("paths = %v, want postfix", line.Paths)
	}
}

func TestSinglestatConnectedNullsMakeTheFirstValueTheFirstNonNull(t *testing.T) {
	cases := []struct{ name, members, want string }{
		{"connected first", `"valueName":"first","nullPointMode":"connected"`, "firstNotNull"},
		{"null first", `"valueName":"first","nullPointMode":"null"`, "first"},
		{"connected average", `"valueName":"avg","nullPointMode":"connected"`, "mean"},
		{"connected current", `"valueName":"current","nullPointMode":"connected"`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := singlestatPanel(t, tc.members)
			if got := displayOf(t, panel).Reduce; got != tc.want {
				t.Fatalf("reduce = %q, want %q", got, tc.want)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestSinglestatNullAsZeroIsReported(t *testing.T) {
	_, line := singlestatPanel(t, `"nullPointMode":"null as zero"`)
	if line.Status == statusImported || !slices.Contains(line.Paths, "nullPointMode") {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestGrafanaSinglestatPluginImportsLikeSinglestat(t *testing.T) {
	doc := panelDoc("grafana-singlestat-panel", upTarget+`,"sparkline":{"show":true,"lineColor":"#299c46"},"nullPointMode":"connected","colorPostfix":false`)
	_, line := convertedPanel(t, doc)
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}
