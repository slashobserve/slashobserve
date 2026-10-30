package grafana

import (
	"slices"
	"strings"
	"testing"
)

func defaultsPanel(typ, defaults string) string {
	return panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":`+defaults+`,"overrides":[]}`)
}

func TestFieldMinMaxImportsOnTheTypesThatShareARange(t *testing.T) {
	for _, typ := range []string{"gauge", "bargauge"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := convertedPanel(t, defaultsPanel(typ, `{"fieldMinMax":true}`))
			if panel.FieldMinMax == nil || !*panel.FieldMinMax {
				t.Fatalf("fieldMinMax = %v, want true", panel.FieldMinMax)
			}
			if !clean(line) {
				t.Fatalf("line = %+v", line)
			}
		})
	}
}

func TestFieldMinMaxFalseIsTheDefault(t *testing.T) {
	panel, line := convertedPanel(t, defaultsPanel("gauge", `{"fieldMinMax":false}`))
	if panel.FieldMinMax != nil || !clean(line) {
		t.Fatalf("fieldMinMax = %v, line = %+v", panel.FieldMinMax, line)
	}
}

func TestFieldMinMaxIsInvisibleOnTypesThatRangeEachFieldAlready(t *testing.T) {
	for _, typ := range []string{"timeseries", "stat", "table", "barchart"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := convertedPanel(t, defaultsPanel(typ, `{"fieldMinMax":true}`))
			if panel.FieldMinMax != nil {
				t.Fatalf("fieldMinMax = %v, want it left out", *panel.FieldMinMax)
			}
			if !clean(line) {
				t.Fatalf("line = %+v", line)
			}
		})
	}
}

func TestFieldMinMaxOverrideOnlyResetsAPanelThatSetIt(t *testing.T) {
	off := property("fieldMinMax", "false")
	on := property("fieldMinMax", "true")

	t.Run("override turns it on", func(t *testing.T) {
		panel, line := convertedPanel(t, overridePanel("gauge", byUp, on))
		got := onlyOverride(t, panel)
		if got.FieldMinMax == nil || !*got.FieldMinMax || !clean(line) {
			t.Fatalf("override = %+v, line = %+v", got, line)
		}
	})
	t.Run("override off without a panel value is the default", func(t *testing.T) {
		panel, line := convertedPanel(t, overridePanel("gauge", byUp, off))
		if len(panel.Overrides) != 0 && onlyOverride(t, panel).FieldMinMax != nil {
			t.Fatalf("overrides = %+v", panel.Overrides)
		}
		if !clean(line) {
			t.Fatalf("line = %+v", line)
		}
	})
	t.Run("override off over a panel value resets it", func(t *testing.T) {
		doc := panelDoc("bargauge", upTarget+`,"fieldConfig":{"defaults":{"fieldMinMax":true},"overrides":[{"matcher":`+byUp+`,"properties":[`+off+`]}]}`)
		panel, line := convertedPanel(t, doc)
		got := onlyOverride(t, panel)
		if got.FieldMinMax == nil || *got.FieldMinMax || !clean(line) {
			t.Fatalf("override = %+v, line = %+v", got, line)
		}
	})
}

func linkMembers(oneClick string) string {
	return `"fieldConfig":{"defaults":{"links":[{"title":"Open","url":"https://example.com/${__value.raw}","targetBlank":true` + oneClick + `}]},"overrides":[]}`
}

func TestOneClickLinkImportsOnTheTypesThatDrawDataLinks(t *testing.T) {
	for _, typ := range []string{"stat", "gauge", "bargauge", "table"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc(typ, upTarget+","+linkMembers(`,"oneClick":true`)))
			if len(panel.Links) != 1 || !panel.Links[0].OneClick || !panel.Links[0].NewTab {
				t.Fatalf("links = %+v", panel.Links)
			}
			if !clean(line) {
				t.Fatalf("line = %+v", line)
			}
		})
	}
}

func TestOneClickLinkIsReportedWhereNoDataLinkIsDrawn(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("timeseries", upTarget+","+linkMembers(`,"oneClick":true`)))
	if len(panel.Links) != 1 || panel.Links[0].OneClick {
		t.Fatalf("links = %+v, want the link kept without oneClick", panel.Links)
	}
	if line.Status != statusPartial || !slices.ContainsFunc(line.Paths, func(p string) bool { return strings.HasSuffix(p, ".oneClick") }) {
		t.Fatalf("line = %+v", line)
	}
}

func TestLinkWithoutOneClickStaysAPlainLink(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("timeseries", upTarget+","+linkMembers("")))
	if len(panel.Links) != 1 || panel.Links[0].OneClick || !clean(line) {
		t.Fatalf("links = %+v, line = %+v", panel.Links, line)
	}
}

func TestDisplayNameTokens(t *testing.T) {
	cases := []struct {
		name, displayName string
		clean             bool
	}{
		{"series name", "${__series.name}", true},
		{"field name", "${__field.name}", true},
		{"field display name", "${__field.displayName}", true},
		{"label", "${__field.labels.pod}", true},
		{"ref id", "${__data.refId}", true},
		{"variable", "$env", true},
		{"braced variable", "${env}", true},
		{"bracketed variable", "[[env]]", true},
		{"formatted variable", "${env:pipe}", true},
		{"mixed", "$env ${__field.labels.pod} [[env]]", true},
		{"data field", "${__data.fields[0]}", false},
		{"interval", "$__interval", false},
		{"unknown global", "${__org.name}", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, defaultsPanel("timeseries", `{"displayName":"`+strings.ReplaceAll(tc.displayName, `"`, `\"`)+`"}`))
			if panel.DisplayName != tc.displayName {
				t.Fatalf("displayName = %q, want %q", panel.DisplayName, tc.displayName)
			}
			if clean(line) != tc.clean {
				t.Fatalf("clean = %v, want %v (line %+v)", clean(line), tc.clean, line)
			}
			if !tc.clean && !slices.Contains(line.Paths, "fieldConfig.defaults.displayName") {
				t.Fatalf("paths = %v", line.Paths)
			}
		})
	}
}
