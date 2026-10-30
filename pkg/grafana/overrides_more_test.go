package grafana

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func overridePanel(typ, matcher string, properties ...string) string {
	return panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{},"overrides":[{"matcher":`+matcher+`,"properties":[`+strings.Join(properties, ",")+`]}]}`)
}

const byUp = `{"id":"byName","options":"up"}`

func property(id, value string) string {
	return fmt.Sprintf(`{"id":%q,"value":%s}`, id, value)
}

func clean(line PanelLine) bool {
	return line.Status == statusImported && len(line.Paths) == 0
}

func onlyOverride(t *testing.T, panel spec.Panel) spec.Override {
	t.Helper()
	if len(panel.Overrides) != 1 {
		t.Fatalf("overrides = %+v, want one", panel.Overrides)
	}
	return panel.Overrides[0]
}

func TestOverrideSeriesDrawingProperties(t *testing.T) {
	cases := []struct {
		name     string
		property string
		check    func(spec.FieldProps) bool
	}{
		{"negative-Y", property("custom.transform", `"negative-Y"`), func(p spec.FieldProps) bool { return p.ValueTransform == "negativeY" }},
		{"constant", property("custom.transform", `"constant"`), func(p spec.FieldProps) bool { return p.ValueTransform == "constant" }},
		{"hide from legend", property("custom.hideFrom", `{"legend":true,"tooltip":false,"viz":false}`), func(p spec.FieldProps) bool {
			return p.HideInLegend != nil && *p.HideInLegend && p.HideInTooltip == nil && p.Hidden == nil
		}},
		{"hide from tooltip", property("custom.hideFrom", `{"legend":false,"tooltip":true,"viz":false}`), func(p spec.FieldProps) bool {
			return p.HideInTooltip != nil && *p.HideInTooltip && p.HideInLegend == nil
		}},
		{"stacking group B", property("custom.stacking", `{"mode":"normal","group":"B"}`), func(p spec.FieldProps) bool { return p.Stack == "normal" && p.StackGroup == "B" }},
		{"stacking group A is the default", property("custom.stacking", `{"mode":"normal","group":"A"}`), func(p spec.FieldProps) bool { return p.Stack == "normal" && p.StackGroup == "" }},
		{"fill below to", property("custom.fillBelowTo", `"down"`), func(p spec.FieldProps) bool { return p.FillBelowTo == "down" }},
	}
	for _, typ := range []string{"timeseries", "trend"} {
		for _, tc := range cases {
			t.Run(typ+" "+tc.name, func(t *testing.T) {
				panel, line := convertedPanel(t, overridePanel(typ, byUp, tc.property))
				if !clean(line) {
					t.Fatalf("status %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
				}
				if got := onlyOverride(t, panel); !tc.check(got.FieldProps) {
					t.Fatalf("override props = %+v", got.FieldProps)
				}
			})
		}
	}
}

func TestSeriesDrawingPropertiesOnTheDefaultsOfTheTypesThatDrawThem(t *testing.T) {
	doc := func(typ, custom string) string {
		return panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{"custom":{`+custom+`}},"overrides":[]}`)
	}
	cases := []struct {
		name, custom string
		check        func(spec.Panel) bool
		drawnByBars  bool
	}{
		{"negative-Y", `"transform":"negative-Y"`, func(p spec.Panel) bool { return p.ValueTransform == "negativeY" }, false},
		{"hide from legend", `"hideFrom":{"legend":true,"tooltip":false,"viz":false}`, func(p spec.Panel) bool { return p.HideInLegend != nil && *p.HideInLegend }, true},
		{"stacking group", `"stacking":{"mode":"normal","group":"B"}`, func(p spec.Panel) bool { return p.StackGroup == "B" }, false},
		{"fill below to", `"fillBelowTo":"down"`, func(p spec.Panel) bool { return p.FillBelowTo == "down" }, false},
	}
	for _, tc := range cases {
		t.Run("timeseries "+tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, doc("timeseries", tc.custom))
			if !clean(line) || !tc.check(panel) {
				t.Fatalf("status %s, paths %v, panel %+v", line.Status, line.Paths, panel.FieldProps)
			}
		})
		if tc.drawnByBars {
			return
		}
		t.Run("barchart "+tc.name+" is reported", func(t *testing.T) {
			_, line := convertedPanel(t, doc("barchart", tc.custom))
			if line.Status != statusPartial || len(line.Paths) == 0 {
				t.Fatalf("status %s, paths %v, want the option reported", line.Status, line.Paths)
			}
		})
	}
}

func TestAnOverrideStackGroupResetsAPanelStackGroup(t *testing.T) {
	doc := panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{"custom":{"stacking":{"mode":"normal","group":"B"}}},"overrides":[{"matcher":`+byUp+`,"properties":[`+property("custom.stacking", `{"mode":"normal","group":"A"}`)+`]}]}`)
	panel, _ := convertedPanel(t, doc)
	if got := onlyOverride(t, panel); got.StackGroup != "A" {
		t.Fatalf("override stack group = %q, want A written explicitly", got.StackGroup)
	}
}

func TestAnOverrideHideFromResetsAPanelLevelHideOnly(t *testing.T) {
	reset := property("custom.hideFrom", `{"legend":false,"tooltip":false,"viz":false}`)
	plain := overridePanel("timeseries", byUp, reset, property("unit", `"bytes"`))
	panel, _ := convertedPanel(t, plain)
	if got := onlyOverride(t, panel); got.HideInLegend != nil || got.HideInTooltip != nil {
		t.Fatalf("override wrote %+v for a hide that was never on", got.FieldProps)
	}
	hidden := panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{"custom":{"hideFrom":{"legend":true,"tooltip":true,"viz":false}}},"overrides":[{"matcher":`+byUp+`,"properties":[`+reset+`]}]}`)
	panel, _ = convertedPanel(t, hidden)
	got := onlyOverride(t, panel)
	if got.HideInLegend == nil || *got.HideInLegend || got.HideInTooltip == nil || *got.HideInTooltip {
		t.Fatalf("override = %+v, want both resets written as false", got.FieldProps)
	}
}

func TestSeriesDrawingPropertiesThatCannotBeKeptAreReported(t *testing.T) {
	long := strings.Repeat("x", 51)
	cases := []struct{ name, property, path string }{
		{"unknown transform", property("custom.transform", `"log10"`), "fieldConfig.overrides[0].properties[0]"},
		{"stack group too long", property("custom.stacking", `{"mode":"normal","group":"`+long+`"}`), "fieldConfig.overrides[0].properties[0]"},
		{"fill below to too long", property("custom.fillBelowTo", `"`+strings.Repeat("x", 201)+`"`), "fieldConfig.overrides[0].properties[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, overridePanel("timeseries", byUp, tc.property, property("unit", `"bytes"`)))
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v, want %s reported", line.Status, line.Paths, tc.path)
			}
			got := onlyOverride(t, panel)
			if got.ValueTransform != "" || got.StackGroup != "" || got.FillBelowTo != "" {
				t.Fatalf("override wrote %+v for a value that cannot be kept", got.FieldProps)
			}
		})
	}
}

func TestByNamesBecomesASeriesRegex(t *testing.T) {
	cases := []struct{ name, options, want string }{
		{"include", `{"mode":"include","names":["up","down"]}`, `^(?:up|down)$`},
		{"include is the default mode", `{"names":["up"]}`, `^(?:up)$`},
		{"exclude", `{"mode":"exclude","names":["up","down"]}`, `/^(?!(?:up|down)$).*$/`},
		{"metacharacters and slashes are escaped", `{"mode":"exclude","names":["a.b","x/y","(z)+","c|d","e\\f"]}`, `/^(?!(?:a\.b|x\/y|\(z\)\+|c\|d|e\\f)$).*$/`},
		{"prefix and readOnly are consumed", `{"mode":"include","names":["up"],"prefix":"p","readOnly":true}`, `^(?:up)$`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matcher := `{"id":"byNames","options":` + tc.options + `}`
			panel, line := convertedPanel(t, overridePanel("timeseries", matcher, property("unit", `"bytes"`)))
			if !clean(line) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if got := onlyOverride(t, panel).Match; got.SeriesRegex != tc.want {
				t.Fatalf("match = %q, want %q", got.SeriesRegex, tc.want)
			}
		})
	}
}

func TestByNamesThatCannotBeExpressedIsReported(t *testing.T) {
	cases := []struct{ name, options string }{
		{"no names", `{"mode":"include","names":[]}`},
		{"unknown mode", `{"mode":"other","names":["up"]}`},
		{"a name that is not text", `{"mode":"include","names":[1]}`},
		{"too long for a pattern", `{"mode":"include","names":["` + strings.Repeat("a", 500) + `"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matcher := `{"id":"byNames","options":` + tc.options + `}`
			panel, line := convertedPanel(t, overridePanel("timeseries", matcher, property("unit", `"bytes"`)))
			if len(panel.Overrides) != 0 || line.Status != statusPartial {
				t.Fatalf("overrides = %+v, status %s, want the override reported", panel.Overrides, line.Status)
			}
		})
	}
}

func TestAHideSeriesToggleConvertsLikeAnyByNamesOverride(t *testing.T) {
	doc := panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{},"overrides":[{"__systemRef":"hideSeriesFrom","matcher":{"id":"byNames","options":{"mode":"exclude","names":["up"],"prefix":"All except:","readOnly":true}},"properties":[{"id":"custom.hideFrom","value":{"viz":true,"legend":false,"tooltip":false}}]}]}`)
	panel, line := convertedPanel(t, doc)
	got := onlyOverride(t, panel)
	if !clean(line) || got.Hidden == nil || !*got.Hidden || got.Match.SeriesRegex != `/^(?!(?:up)$).*$/` {
		t.Fatalf("status %s, paths %v, override %+v %+v", line.Status, line.Paths, got.Match, got.FieldProps)
	}
}

func TestByValueBecomesAValueMatch(t *testing.T) {
	for _, op := range []string{"eq", "neq", "lt", "lte", "gt", "gte"} {
		t.Run(op, func(t *testing.T) {
			matcher := `{"id":"byValue","options":{"reducer":"max","op":"` + op + `","value":100.5}}`
			panel, line := convertedPanel(t, overridePanel("timeseries", matcher, property("unit", `"bytes"`)))
			got := onlyOverride(t, panel).Match.Value
			if !clean(line) || got == nil || *got != (spec.ValueMatch{Reducer: "max", Op: op, Value: 100.5}) {
				t.Fatalf("status %s, paths %v, match %+v", line.Status, line.Paths, got)
			}
		})
	}
}

func TestByValueThatCannotBeExpressedIsReported(t *testing.T) {
	cases := []struct{ name, options string }{
		{"unknown reducer", `{"reducer":"p95","op":"gt","value":1}`},
		{"unknown operator", `{"reducer":"max","op":"between","value":1}`},
		{"value that is not a number", `{"reducer":"max","op":"gt","value":"high"}`},
		{"no options", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matcher := `{"id":"byValue","options":` + tc.options + `}`
			panel, line := convertedPanel(t, overridePanel("timeseries", matcher, property("unit", `"bytes"`)))
			if len(panel.Overrides) != 0 || line.Status != statusPartial {
				t.Fatalf("overrides = %+v, status %s, want the override reported", panel.Overrides, line.Status)
			}
		})
	}
}

func TestByFrameRefIDFollowsTheLetterGrafanaAssignsByPosition(t *testing.T) {
	cases := []struct {
		name, targets, refID string
		want                 string
	}{
		{"first target without a refId", `[{"expr":"up"}]`, "A", "A"},
		{"second target without a refId", `[{"refId":"A","expr":"up"},{"expr":"down"}]`, "B", "B"},
		{"a target after an explicit refId keeps its position", `[{"refId":"Q","expr":"up"},{"expr":"down"}]`, "B", "B"},
		{"a refId renamed by the importer", `[{"refId":"A","expr":"up"},{"refId":"A","expr":"down"}]`, "A", "A"},
		{"an explicit refId maps to its own query", `[{"refId":"Q","expr":"up"},{"expr":"down"}]`, "Q", "Q"},
		{"an explicit refId wins the letter an earlier target would imply", `[{"expr":"up"},{"refId":"A","expr":"down"}]`, "A", "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := panelDoc("timeseries", `"targets":`+tc.targets+`,"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byFrameRefID","options":"`+tc.refID+`"},"properties":[`+property("unit", `"bytes"`)+`]}]}`)
			panel, line := convertedPanel(t, doc)
			if got := onlyOverride(t, panel).Match.Query; got != tc.want || !clean(line) {
				t.Fatalf("match query = %q (want %q), status %s, paths %v", got, tc.want, line.Status, line.Paths)
			}
		})
	}
}

func TestByFrameRefIDOfAnImpliedLetterBeyondTheAlphabetIsReported(t *testing.T) {
	targets := make([]string, 27)
	for i := range targets {
		targets[i] = `{"expr":"up"}`
	}
	doc := panelDoc("timeseries", `"targets":[`+strings.Join(targets, ",")+`],"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byFrameRefID","options":"AA"},"properties":[`+property("unit", `"bytes"`)+`]}]}`)
	panel, line := convertedPanel(t, doc)
	if len(panel.Overrides) != 0 || line.Status != statusPartial {
		t.Fatalf("overrides = %+v, status %s, want the override reported", panel.Overrides, line.Status)
	}
}

func TestLegacySeriesOverrideKeysConvert(t *testing.T) {
	cases := []struct {
		name, member string
		check        func(spec.FieldProps) bool
	}{
		{"negative-Y", `"transform":"negative-Y"`, func(p spec.FieldProps) bool { return p.ValueTransform == "negativeY" }},
		{"constant", `"transform":"constant"`, func(p spec.FieldProps) bool { return p.ValueTransform == "constant" }},
		{"legend off", `"legend":false`, func(p spec.FieldProps) bool { return p.HideInLegend != nil && *p.HideInLegend }},
		{"legend on", `"legend":true`, func(p spec.FieldProps) bool { return p.HideInLegend == nil }},
		{"hide tooltip", `"hideTooltip":true`, func(p spec.FieldProps) bool { return p.HideInTooltip != nil && *p.HideInTooltip }},
		{"tooltip shown", `"hideTooltip":false`, func(p spec.FieldProps) bool { return p.HideInTooltip == nil }},
		{"fill below to", `"fillBelowTo":"down"`, func(p spec.FieldProps) bool { return p.FillBelowTo == "down" }},
		{"stack group B", `"stack":"B"`, func(p spec.FieldProps) bool { return p.Stack == "normal" && p.StackGroup == "B" }},
		{"stack group A", `"stack":"A"`, func(p spec.FieldProps) bool { return p.Stack == "normal" && p.StackGroup == "" }},
		{"stack flag", `"stack":false`, func(p spec.FieldProps) bool { return p.Stack == "none" }},
		{"draw order", `"zindex":3`, func(p spec.FieldProps) bool { return p.Stack == "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, `"seriesOverrides":[{"alias":"errors","color":"#ff0000",`+tc.member+`}]`)
			got := onlyOverride(t, panel)
			if !tc.check(got.FieldProps) || !clean(line) {
				t.Fatalf("override %+v, status %s, paths %v", got.FieldProps, line.Status, line.Paths)
			}
		})
	}
}

func TestLegacySeriesOverrideKeysThatCannotBeKeptAreReported(t *testing.T) {
	cases := []struct{ name, member, path string }{
		{"unknown transform", `"transform":"log"`, "seriesOverrides[0].transform"},
		{"legend that is not a flag", `"legend":"no"`, "seriesOverrides[0].legend"},
		{"fill below to that is not text", `"fillBelowTo":3`, "seriesOverrides[0].fillBelowTo"},
		{"stack group too long", `"stack":"` + strings.Repeat("x", 51) + `"`, "seriesOverrides[0].stack"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := graphPanel(t, `"seriesOverrides":[{"alias":"errors","color":"#ff0000",`+tc.member+`}]`)
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v, want %s reported", line.Status, line.Paths, tc.path)
			}
		})
	}
}
