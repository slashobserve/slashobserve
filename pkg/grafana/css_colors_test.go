package grafana

import (
	"regexp"
	"testing"
)

func TestSeriesColorReadsTextCssNamesAndGrafanaNamesInThatPrecedence(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"text", "text", true},
		{" text ", "text", true},
		{"gray", "#808080", true},
		{"Grey", "#808080", true},
		{"darkSlateGrey", "#2f4f4f", true},
		{"rebeccapurple", "#663399", true},
		{"white", "#ffffff", true},
		{"purple", "#B877D9", true},
		{"green", "#73BF69", true},
		{"dark-green", "#37872D", true},
		{"#abc", "#aabbcc", true},
		{"rgb(1, 2, 3)", "#010203", true},
		{"Text", "", false},
		{"transparent", "", false},
		{"notacolor", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := seriesColor(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("seriesColor(%q) = %q, %v, want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestThresholdColorKeepsSemanticFamiliesBeforeCssNames(t *testing.T) {
	cases := []struct{ in, want string }{
		{"text", "text"},
		{"green", "ok"},
		{"dark-red", "critical"},
		{"orange", "warning"},
		{"blue", "info"},
		{"gray", "#808080"},
		{"crimson", "#dc143c"},
	}
	for _, tc := range cases {
		if got, ok := thresholdColor(tc.in); !ok || got != tc.want {
			t.Errorf("thresholdColor(%q) = %q, %v, want %q", tc.in, got, ok, tc.want)
		}
	}
}

func TestCssColorNamesAreTheLevelFourTable(t *testing.T) {
	if len(cssColorNames) != 148 {
		t.Errorf("%d names, want the 148 of CSS Color Level 4", len(cssColorNames))
	}
	lowerHex := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	lowerName := regexp.MustCompile(`^[a-z]+$`)
	for name, hex := range cssColorNames {
		if !lowerHex.MatchString(hex) || !lowerName.MatchString(name) {
			t.Errorf("%q maps to %q", name, hex)
		}
	}
	for _, pair := range [][2]string{{"gray", "grey"}, {"darkgray", "darkgrey"}, {"lightslategray", "lightslategrey"}, {"aqua", "cyan"}, {"fuchsia", "magenta"}} {
		if cssColorNames[pair[0]] != cssColorNames[pair[1]] {
			t.Errorf("%s and %s differ", pair[0], pair[1])
		}
	}
}

func TestTextColorImportsAsTheSemanticTextColorInThresholdsFixedColorsAndMappings(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("stat", upTarget+`,"fieldConfig":{"defaults":{"thresholds":{"mode":"absolute","steps":[{"color":"green","value":null},{"color":"text","value":10}]},"mappings":[{"type":"value","options":{"1":{"text":"Up","color":"text","index":0}}}]},"overrides":[]}`))
	requireImported(t, line)
	if len(panel.Thresholds) != 2 || panel.Thresholds[1].Color != "text" {
		t.Errorf("thresholds = %+v", panel.Thresholds)
	}
	if len(panel.Mappings) != 1 || panel.Mappings[0].Color != "text" {
		t.Errorf("mappings = %+v", panel.Mappings)
	}

	panel, line = colorPanel(t, "stat", `{"mode":"fixed","fixedColor":"text"}`, "")
	requireImported(t, line)
	if panel.Color != "text" {
		t.Errorf("color = %q", panel.Color)
	}

	panel, line = colorOverridePanel(t, `{"mode":"palette-classic"}`, `{"mode":"fixed","fixedColor":"gray"}`)
	requireImported(t, line)
	if len(panel.Overrides) != 1 || panel.Overrides[0].Color != "#808080" {
		t.Errorf("overrides = %+v", panel.Overrides)
	}
}

func TestActionColorImportsTheThemeTextColorLikeEveryOtherColor(t *testing.T) {
	cases := []struct {
		color, want string
		reported    bool
	}{
		{"gray", "#808080", false},
		{"text", "text", false},
	}
	for _, tc := range cases {
		panel, line := actionPanel(t, "stat", `"actions":[`+fieldActionEntry(`"style":{"backgroundColor":"`+tc.color+`"},`)+`]`, "")
		if len(panel.Actions) != 1 || panel.Actions[0].Color != tc.want {
			t.Errorf("%s: actions = %+v", tc.color, panel.Actions)
		}
		if got := clean(line); got == tc.reported {
			t.Errorf("%s: clean = %v, paths %v", tc.color, got, line.Paths)
		}
	}
}
