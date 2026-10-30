package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func logsPanel(t *testing.T, options string) (spec.Display, PanelLine) {
	t.Helper()
	panel, line := convertedPanel(t, panelDoc("logs", upTarget+`,"options":`+options))
	if panel.Display == nil {
		return spec.Display{}, line
	}
	return *panel.Display, line
}

func flagState(p *bool) string {
	switch {
	case p == nil:
		return "absent"
	case *p:
		return "true"
	default:
		return "false"
	}
}

func TestLogsToggleOptionsImportOnlyWhenTheyDifferFromTheSpecDefault(t *testing.T) {
	cases := []struct {
		option string
		pick   func(spec.Display) *bool
	}{
		{"showLabels", func(d spec.Display) *bool { return d.ShowLabels }},
		{"showCommonLabels", func(d spec.Display) *bool { return d.ShowCommonLabels }},
		{"prettifyLogMessage", func(d spec.Display) *bool { return d.Prettify }},
		{"showControls", func(d spec.Display) *bool { return d.Controls }},
		{"syntaxHighlighting", func(d spec.Display) *bool { return d.Highlight }},
		{"enableInfiniteScrolling", func(d spec.Display) *bool { return d.InfiniteScroll }},
	}
	for _, tc := range cases {
		t.Run(tc.option, func(t *testing.T) {
			d, line := logsPanel(t, `{"`+tc.option+`":true}`)
			if got := flagState(tc.pick(d)); got != "true" || line.Status != statusImported {
				t.Fatalf("on: %s = %s, status %s, paths %v", tc.option, got, line.Status, line.Paths)
			}
			d, line = logsPanel(t, `{"`+tc.option+`":false}`)
			if got := flagState(tc.pick(d)); got != "absent" || line.Status != statusImported {
				t.Fatalf("off: %s = %s, status %s, paths %v", tc.option, got, line.Status, line.Paths)
			}
		})
	}
}

func TestLogsDetailsOffImportsAndOnWritesNothing(t *testing.T) {
	d, line := logsPanel(t, `{"enableLogDetails":false}`)
	if flagState(d.Details) != "false" || line.Status != statusImported {
		t.Fatalf("display = %+v, status %s, paths %v", d, line.Status, line.Paths)
	}
	d, line = logsPanel(t, `{"enableLogDetails":true}`)
	if d.Details != nil || line.Status != statusImported {
		t.Fatalf("display = %+v, status %s, paths %v", d, line.Status, line.Paths)
	}
}

func TestLogsEnumOptionsImportAndTheirDefaultsWriteNothing(t *testing.T) {
	dedup := func(d spec.Display) string { return d.Dedup }
	font := func(d spec.Display) string { return d.FontSize }
	mode := func(d spec.Display) string { return d.DetailsMode }
	cases := []struct {
		name, options string
		pick          func(spec.Display) string
		want          string
	}{
		{"dedup exact", `{"dedupStrategy":"exact"}`, dedup, "exact"},
		{"dedup numbers", `{"dedupStrategy":"numbers"}`, dedup, "numbers"},
		{"dedup signature", `{"dedupStrategy":"signature"}`, dedup, "signature"},
		{"dedup none", `{"dedupStrategy":"none"}`, dedup, ""},
		{"small font", `{"fontSize":"small"}`, font, "small"},
		{"default font", `{"fontSize":"default"}`, font, ""},
		{"inline details", `{"detailsMode":"inline"}`, mode, "inline"},
		{"sidebar details", `{"detailsMode":"sidebar"}`, mode, "sidebar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := logsPanel(t, tc.options)
			if got := tc.pick(d); got != tc.want || line.Status != statusImported {
				t.Fatalf("got %q, status %s, paths %v, want %q imported", got, line.Status, line.Paths, tc.want)
			}
		})
	}
}

func TestLogsUnknownDedupStrategyIsReported(t *testing.T) {
	d, line := logsPanel(t, `{"dedupStrategy":"fuzzy"}`)
	if d.Dedup != "" || line.Status != statusPartial || !slices.Equal(line.Paths, []string{"options.dedupStrategy"}) {
		t.Fatalf("display = %+v, status %s, paths %v", d, line.Status, line.Paths)
	}
}

func TestLogsWithoutLineOptionsWritesNoneOfThem(t *testing.T) {
	d, line := logsPanel(t, `{"showTime":true}`)
	if d.ShowLabels != nil || d.ShowCommonLabels != nil || d.Details != nil || d.Prettify != nil || d.Controls != nil ||
		d.Highlight != nil || d.InfiniteScroll != nil || d.Dedup != "" || d.FontSize != "" || d.DetailsMode != "" {
		t.Fatalf("display = %+v", d)
	}
	if line.Status != statusImported {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
}

func TestLogsOptionsWithoutASpecKeyAreStillReported(t *testing.T) {
	cases := []struct{ option, value string }{
		{"showFieldSelector", `true`},
		{"showLogContextToggle", `true`},
		{"timestampResolution", `"ns"`},
		{"showLogAttributes", `false`},
		{"displayedFields", `["level"]`},
	}
	for _, tc := range cases {
		t.Run(tc.option, func(t *testing.T) {
			_, line := logsPanel(t, `{"`+tc.option+`":`+tc.value+`}`)
			if line.Status != statusPartial || !slices.Equal(line.Paths, []string{"options." + tc.option}) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}
