package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func statPanel(t *testing.T, options string) (spec.Display, PanelLine) {
	t.Helper()
	panel, line := convertedPanel(t, panelDoc("stat", upTarget+`,"options":`+options))
	if panel.Display == nil {
		return spec.Display{}, line
	}
	return *panel.Display, line
}

func TestStatJustifyModeCenterImportsAndAutoWritesNothing(t *testing.T) {
	cases := []struct{ name, options, want string }{
		{"center", `{"justifyMode":"center"}`, "center"},
		{"auto", `{"justifyMode":"auto"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := statPanel(t, tc.options)
			if d.Justify != tc.want || line.Status != statusImported {
				t.Fatalf("justify = %q, status %s, paths %v, want %q imported", d.Justify, line.Status, line.Paths, tc.want)
			}
		})
	}
}

func TestStatTextSizesImportAndOutOfRangeSizesAreReported(t *testing.T) {
	d, line := statPanel(t, `{"text":{"titleSize":20,"valueSize":40}}`)
	if d.TitleSize != 20 || d.ValueSize != 40 || line.Status != statusImported {
		t.Fatalf("display = %+v, status %s, paths %v", d, line.Status, line.Paths)
	}

	cases := []struct{ name, options, path string }{
		{"title below 6", `{"text":{"titleSize":5}}`, "options.text.titleSize"},
		{"title above 200", `{"text":{"titleSize":201}}`, "options.text.titleSize"},
		{"value fractional", `{"text":{"valueSize":12.5}}`, "options.text.valueSize"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := statPanel(t, tc.options)
			if line.Status != statusPartial || !slices.Equal(line.Paths, []string{tc.path}) {
				t.Fatalf("status = %s, paths %v, want partial at %s", line.Status, line.Paths, tc.path)
			}
			if d.TitleSize != 0 || d.ValueSize != 0 {
				t.Fatalf("display = %+v, want the size left out", d)
			}
		})
	}
}

func TestStatPercentChangeImportsWithItsColorMode(t *testing.T) {
	cases := []struct {
		name, options, wantColor string
		wantShown                bool
	}{
		{"shown, standard", `{"showPercentChange":true,"percentChangeColorMode":"standard"}`, "", true},
		{"shown, inverted", `{"showPercentChange":true,"percentChangeColorMode":"inverted"}`, "inverted", true},
		{"shown, same as value", `{"showPercentChange":true,"percentChangeColorMode":"same_as_value"}`, "value", true},
		{"hidden", `{"showPercentChange":false}`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := statPanel(t, tc.options)
			shown := d.PercentChange != nil && *d.PercentChange
			if shown != tc.wantShown || d.PercentChangeColor != tc.wantColor || line.Status != statusImported {
				t.Fatalf("display = %+v, status %s, paths %v", d, line.Status, line.Paths)
			}
		})
	}
}

func TestStatWideLayoutKeepsGrafanasValueEitherWay(t *testing.T) {
	for _, want := range []bool{true, false} {
		options := `{"wideLayout":false}`
		if want {
			options = `{"wideLayout":true}`
		}
		d, line := statPanel(t, options)
		if d.WideLayout == nil || *d.WideLayout != want || line.Status != statusImported {
			t.Fatalf("wideLayout %v: display = %+v, status %s, paths %v", want, d, line.Status, line.Paths)
		}
	}
}

func TestStatWithoutLayoutOptionsWritesNoneOfThem(t *testing.T) {
	d, line := statPanel(t, `{"graphMode":"none"}`)
	if d.Justify != "" || d.TitleSize != 0 || d.ValueSize != 0 || d.PercentChange != nil || d.PercentChangeColor != "" || d.WideLayout != nil {
		t.Fatalf("display = %+v", d)
	}
	if d.Graph == nil || *d.Graph || line.Status != statusImported {
		t.Fatalf("graph mode none: display = %+v, status %s", d, line.Status)
	}
}
