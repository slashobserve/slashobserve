package grafana

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func overrideOf(properties string) string {
	return `{"matcher":{"id":"byName","options":"a"},"properties":[` + properties + `]}`
}

func firstOverrideProps(t *testing.T, panel spec.Panel) spec.FieldProps {
	t.Helper()
	if len(panel.Overrides) != 1 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	return panel.Overrides[0].FieldProps
}

func TestInsertNullsNumberConverts(t *testing.T) {
	panel, line := customPanel(t, `{"insertNulls":60000}`, "")
	requireImported(t, line)
	if panel.InsertNulls != 60000 {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestInsertNullsFalseIsSilent(t *testing.T) {
	panel, line := customPanel(t, `{"insertNulls":false}`, "")
	requireImported(t, line)
	if panel.InsertNulls != 0 {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestInsertNullsTrueIsReported(t *testing.T) {
	_, line := customPanel(t, `{"insertNulls":true}`, "")
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "fieldConfig.defaults.custom.insertNulls" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
	if !strings.Contains(strings.Join(line.Reasons, " "), insertNullsReason) {
		t.Fatalf("reasons = %v", line.Reasons)
	}
}

func TestInsertNullsOverrideSetsTheGap(t *testing.T) {
	panel, line := customPanel(t, `{}`, overrideOf(`{"id":"custom.insertNulls","value":30000}`))
	requireImported(t, line)
	if got := firstOverrideProps(t, panel); got.InsertNulls != 30000 {
		t.Fatalf("override props = %+v", got)
	}
}

func TestInsertNullsOverrideCannotTurnOffThePanelsNulls(t *testing.T) {
	_, line := customPanel(t, `{"insertNulls":60000}`, overrideOf(`{"id":"custom.insertNulls","value":false}`))
	if line.Status != statusPartial {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestInsertNullsIsNotConvertedOnOtherTypes(t *testing.T) {
	_, line := customPanelOf(t, "barchart", `{"insertNulls":60000}`, "")
	if line.Status != statusPartial {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestBarPropsConvertWhenBarsAreDrawn(t *testing.T) {
	cases := []struct {
		name, custom string
		check        func(spec.FieldProps) bool
	}{
		{"alignment before", `{"drawStyle":"bars","barAlignment":-1}`, func(p spec.FieldProps) bool { return p.BarAlignment == "before" }},
		{"alignment after", `{"drawStyle":"bars","barAlignment":1}`, func(p spec.FieldProps) bool { return p.BarAlignment == "after" }},
		{"alignment center is the default", `{"drawStyle":"bars","barAlignment":0}`, func(p spec.FieldProps) bool { return p.BarAlignment == "" }},
		{"width factor", `{"drawStyle":"bars","barWidthFactor":0.8}`, func(p spec.FieldProps) bool { return floatOf(p.BarWidthFactor) == 0.8 }},
		{"width factor is clamped", `{"drawStyle":"bars","barWidthFactor":0.001}`, func(p spec.FieldProps) bool { return floatOf(p.BarWidthFactor) == 0.05 }},
		{"canvas width factor is the default", `{"drawStyle":"bars","barWidthFactor":0.6}`, func(p spec.FieldProps) bool { return p.BarWidthFactor == nil }},
		{"max width", `{"drawStyle":"bars","barMaxWidth":20}`, func(p spec.FieldProps) bool { return p.BarMaxWidth != nil && *p.BarMaxWidth == 20 }},
		{"max width is clamped", `{"drawStyle":"bars","barMaxWidth":900}`, func(p spec.FieldProps) bool { return p.BarMaxWidth != nil && *p.BarMaxWidth == 200 }},
		{"canvas max width is the default", `{"drawStyle":"bars","barMaxWidth":64}`, func(p spec.FieldProps) bool { return p.BarMaxWidth == nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanel(t, tc.custom, "")
			requireImported(t, line)
			if !tc.check(panel.FieldProps) {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestBarPropsAreDroppedWhenNoBarsAreDrawn(t *testing.T) {
	panel, line := customPanel(t, `{"drawStyle":"line","barAlignment":1,"barWidthFactor":0.8,"barMaxWidth":20}`, "")
	requireImported(t, line)
	if !fieldPropsEmpty(panel.FieldProps) {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestBarPropsAreReportedOrIgnoredOnTypesThatDrawNone(t *testing.T) {
	panel, _ := customPanelOf(t, "stat", `{"barAlignment":1,"barWidthFactor":0.8}`, "")
	if panel.BarAlignment != "" || panel.BarWidthFactor != nil {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestBarPropsOverrideWritesExplicitValues(t *testing.T) {
	properties := `{"id":"custom.drawStyle","value":"bars"},{"id":"custom.barAlignment","value":0},{"id":"custom.barWidthFactor","value":0.6},{"id":"custom.barMaxWidth","value":64}`
	panel, line := customPanel(t, `{"drawStyle":"bars","barAlignment":1,"barWidthFactor":0.9,"barMaxWidth":30}`, overrideOf(properties))
	requireImported(t, line)
	got := firstOverrideProps(t, panel)
	if got.BarAlignment != "center" || floatOf(got.BarWidthFactor) != 0.6 || got.BarMaxWidth == nil || *got.BarMaxWidth != 64 {
		t.Fatalf("override props = %+v", got)
	}
}

func TestThresholdsStyleModes(t *testing.T) {
	for _, mode := range []string{"off", "line", "dashed", "area", "line+area", "dashed+area", "series"} {
		t.Run(mode, func(t *testing.T) {
			panel, line := customPanel(t, `{"thresholdsStyle":{"mode":"`+mode+`"}}`, "")
			requireImported(t, line)
			if panel.ThresholdsStyle != mode {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestThresholdsStyleUnknownModeIsReported(t *testing.T) {
	_, line := customPanel(t, `{"thresholdsStyle":{"mode":"glow"}}`, "")
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "fieldConfig.defaults.custom.thresholdsStyle.mode" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestThresholdsStyleOverride(t *testing.T) {
	panel, line := customPanel(t, `{}`, overrideOf(`{"id":"custom.thresholdsStyle","value":{"mode":"area"}}`))
	requireImported(t, line)
	if got := firstOverrideProps(t, panel); got.ThresholdsStyle != "area" {
		t.Fatalf("override props = %+v", got)
	}
}

func TestThresholdsStyleOnTypesThatDrawNoneStaysSilentOrReported(t *testing.T) {
	panel, _ := customPanelOf(t, "stat", `{"thresholdsStyle":{"mode":"area"}}`, "")
	if panel.ThresholdsStyle != "" {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestNullPointModeNullAsZeroConverts(t *testing.T) {
	panel, line := graphPanel(t, `"nullPointMode":"null as zero"`)
	requireImported(t, line)
	if panel.NullAsZero == nil || !*panel.NullAsZero {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestSeriesOverrideNullAsZeroConverts(t *testing.T) {
	panel, line := graphPanel(t, `"seriesOverrides":[{"alias":"a","nullPointMode":"null as zero"}]`)
	requireImported(t, line)
	got := firstOverrideProps(t, panel)
	if got.NullAsZero == nil || !*got.NullAsZero {
		t.Fatalf("override props = %+v", got)
	}
}

func TestSeriesOverrideResetsThePanelsNullAsZero(t *testing.T) {
	for _, mode := range []string{"connected", "null"} {
		t.Run(mode, func(t *testing.T) {
			panel, line := graphPanel(t, `"nullPointMode":"null as zero","seriesOverrides":[{"alias":"a","nullPointMode":"`+mode+`"}]`)
			requireImported(t, line)
			got := firstOverrideProps(t, panel)
			if got.NullAsZero == nil || *got.NullAsZero {
				t.Fatalf("override props = %+v", got)
			}
		})
	}
}

func TestSeriesOverrideWithoutPanelNullAsZeroWritesNoReset(t *testing.T) {
	panel, line := graphPanel(t, `"seriesOverrides":[{"alias":"a","nullPointMode":"null"}]`)
	requireImported(t, line)
	if got := firstOverrideProps(t, panel); got.NullAsZero != nil {
		t.Fatalf("override props = %+v", got)
	}
}

func candlestickPanel(t *testing.T, options string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("candlestick", upTarget+`,"options":{`+options+`}`))
}

func TestCandlestickIncludeAllFields(t *testing.T) {
	panel, line := candlestickPanel(t, `"includeAllFields":true`)
	requireImported(t, line)
	if panel.Display == nil || panel.Display.IncludeAllFields == nil || !*panel.Display.IncludeAllFields {
		t.Fatalf("display = %+v", panel.Display)
	}
}

func TestCandlestickIncludeAllFieldsOffIsSilent(t *testing.T) {
	panel, line := candlestickPanel(t, `"includeAllFields":false`)
	requireImported(t, line)
	if panel.Display != nil && panel.Display.IncludeAllFields != nil {
		t.Fatalf("display = %+v", panel.Display)
	}
}
