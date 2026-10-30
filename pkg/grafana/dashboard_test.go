package grafana

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestUrlLinkKeepsTimeAndVariableOptions(t *testing.T) {
	links := `"links":[
		{"type":"link","title":"Docs","url":"https://example.com","keepTime":true,"includeVars":true},
		{"type":"link","title":"Plain","url":"https://example.com/plain","keepTime":false,"includeVars":false},
		{"type":"link","title":"Vars","url":"https://example.com/vars","includeVars":true,"targetBlank":true}
	]`
	res := convertDoc(t, dashboardJSON(links, panelJSON(1, 0, 0, 12, 4, "")))
	want := []spec.DashboardLink{
		{Title: "Docs", URL: "https://example.com", KeepTime: true, IncludeVars: true},
		{Title: "Plain", URL: "https://example.com/plain"},
		{Title: "Vars", URL: "https://example.com/vars", IncludeVars: true, NewTab: true},
	}
	if !reflect.DeepEqual(res.Spec.Links, want) {
		t.Fatalf("links = %+v, want %+v", res.Spec.Links, want)
	}
	if notes := notesFor(res, "dashboard links"); len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}
}

func TestLinksToDashboardsByTag(t *testing.T) {
	links := `"links":[
		{"type":"dashboards","title":"Kubernetes","tags":["Kubernetes-Mixin","prod"],"asDropdown":true,"keepTime":true,"includeVars":true,"targetBlank":true},
		{"type":"dashboards","title":"","tags":[]},
		{"type":"dashboards","title":"Repeated","tags":["prod","PROD"]},
		{"type":"dashboards","title":"Spaces","tags":["my tag"]},
		{"type":"dashboards","title":"Symbol first","tags":["-prod"]},
		{"type":"dashboards","title":"Not a tag","tags":[7]},
		{"type":"something","title":"Other"},
		"nope"
	]`
	res := convertDoc(t, dashboardJSON(links, panelJSON(1, 0, 0, 12, 4, "")))
	want := []spec.DashboardLink{
		{Type: "dashboards", Title: "Kubernetes", Tags: []string{"kubernetes-mixin", "prod"}, AsDropdown: true, KeepTime: true, IncludeVars: true, NewTab: true},
		{Type: "dashboards", Title: "Dashboards"},
		{Type: "dashboards", Title: "Repeated", Tags: []string{"prod"}},
		{Type: "dashboards", Title: "Spaces", Tags: []string{"my-tag"}},
	}
	if !reflect.DeepEqual(res.Spec.Links, want) {
		t.Fatalf("links = %+v, want %+v", res.Spec.Links, want)
	}
	notes := notesFor(res, "dashboard links")
	if len(notes) != 2 {
		t.Fatalf("notes = %+v, want one for unsupported links and one for tags", notes)
	}
	for _, path := range []string{"links[4]", "links[5]", "links[6]", "links[7]"} {
		if !hasPath(notes, path) {
			t.Errorf("no note for %s in %+v", path, notes)
		}
	}
	for _, note := range notes {
		if strings.Contains(note.Reason, "Only plain links") {
			t.Errorf("note still says plain links only: %q", note.Reason)
		}
	}
}

func TestLinksPastTheCapAreReported(t *testing.T) {
	var links []string
	for range maxLinks + 1 {
		links = append(links, `{"type":"dashboards","title":"All"}`)
	}
	res := convertDoc(t, dashboardJSON(`"links":[`+strings.Join(links, ",")+`]`, panelJSON(1, 0, 0, 12, 4, "")))
	if got := len(res.Spec.Links); got != maxLinks {
		t.Fatalf("links = %d, want %d", got, maxLinks)
	}
	if !hasPath(notesFor(res, "dashboard links"), "links[20]") {
		t.Fatalf("notes = %+v, want the 21st link reported", res.Report.Notes)
	}
}

func TestTimePicker(t *testing.T) {
	tests := []struct {
		name  string
		extra string
		want  *spec.TimePicker
		notes []string
	}{
		{name: "absent", extra: ``},
		{name: "empty", extra: `"timepicker":{}`},
		{name: "hidden", extra: `"timepicker":{"hidden":true}`, want: &spec.TimePicker{Hidden: true}},
		{name: "shown", extra: `"timepicker":{"hidden":false}`},
		{name: "refresh intervals", extra: `"timepicker":{"refresh_intervals":["5s","1m","2h","1d"]}`, want: &spec.TimePicker{RefreshIntervals: []string{"5s", "1m", "2h", "1d"}}},
		{name: "refresh intervals equal to the default list", extra: `"timepicker":{"refresh_intervals":["5s","10s","30s","1m","5m","15m","30m","1h","2h","1d"]}`},
		{name: "default refresh intervals with the picker hidden", extra: `"timepicker":{"hidden":true,"refresh_intervals":["5s","10s","30s","1m","5m","15m","30m","1h","2h","1d"]}`, want: &spec.TimePicker{Hidden: true}},
		{name: "default refresh intervals in another order", extra: `"timepicker":{"refresh_intervals":["10s","5s","30s","1m","5m","15m","30m","1h","2h","1d"]}`, want: &spec.TimePicker{RefreshIntervals: []string{"10s", "5s", "30s", "1m", "5m", "15m", "30m", "1h", "2h", "1d"}}},
		{name: "empty refresh intervals", extra: `"timepicker":{"refresh_intervals":[]}`},
		{name: "repeated refresh interval", extra: `"timepicker":{"refresh_intervals":["5s","5s"]}`, want: &spec.TimePicker{RefreshIntervals: []string{"5s"}}},
		{name: "refresh interval Grafana allows and the Spec cannot hold", extra: `"timepicker":{"refresh_intervals":["500ms","10s","0s","1w"]}`, want: &spec.TimePicker{RefreshIntervals: []string{"10s"}}, notes: []string{"timepicker.refresh_intervals[0]", "timepicker.refresh_intervals[2]", "timepicker.refresh_intervals[3]"}},
		{name: "quick ranges", extra: `"timepicker":{"quick_ranges":[{"display":"Last 2 days","from":"now-2d","to":"now"},{"display":"Epoch","from":"1700000000000","to":"1700003600000"}]}`, want: &spec.TimePicker{QuickRanges: []spec.QuickRange{{Display: "Last 2 days", From: "now-2d", To: "now"}, {Display: "Epoch", From: "2023-11-14T22:13:20Z", To: "2023-11-14T23:13:20Z"}}}},
		{name: "quick ranges with a name missing or a time the Spec cannot hold", extra: `"timepicker":{"quick_ranges":[{"display":"","from":"now-1h","to":"now"},{"display":"Odd","from":"yesterday","to":"now"},{"display":"Fine","from":"now-1h","to":"now"},"nope"]}`, want: &spec.TimePicker{QuickRanges: []spec.QuickRange{{Display: "Fine", From: "now-1h", To: "now"}}}, notes: []string{"timepicker.quick_ranges[0]", "timepicker.quick_ranges[1]", "timepicker.quick_ranges[3]"}},
		{name: "options Grafana writes by default", extra: `"timepicker":{"collapse":false,"enable":true,"type":"timepicker","notice":false,"status":"Stable","time_options":["5m","15m","1h","6h","12h","24h","2d","7d","30d"]}`},
		{name: "a changed option nothing here draws", extra: `"timepicker":{"collapse":true,"time_options":["5m"]}`, notes: []string{"timepicker.collapse", "timepicker.time_options"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := convertDoc(t, dashboardJSON(tt.extra, panelJSON(1, 0, 0, 12, 4, "")))
			if !reflect.DeepEqual(res.Spec.TimePicker, tt.want) {
				t.Fatalf("timePicker = %+v, want %+v", res.Spec.TimePicker, tt.want)
			}
			var got []string
			for _, n := range notesFor(res, "time picker") {
				got = append(got, n.Paths...)
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.notes) {
				t.Fatalf("time picker note paths = %v, want %v", got, tt.notes)
			}
			if notes := notesFor(res, "dashboard options"); len(notes) != 0 {
				t.Fatalf("dashboard options notes = %+v, want none", notes)
			}
		})
	}
}

func TestLiveNow(t *testing.T) {
	for _, tt := range []struct {
		extra string
		want  bool
	}{{``, false}, {`"liveNow":false`, false}, {`"liveNow":true`, true}} {
		res := convertDoc(t, dashboardJSON(tt.extra, panelJSON(1, 0, 0, 12, 4, "")))
		if res.Spec.LiveNow != tt.want {
			t.Errorf("%q: liveNow = %v, want %v", tt.extra, res.Spec.LiveNow, tt.want)
		}
		if notes := notesFor(res, "dashboard options"); len(notes) != 0 {
			t.Errorf("%q: dashboard options notes = %+v, want none", tt.extra, notes)
		}
	}
}

func TestRefreshIntervals(t *testing.T) {
	for _, tt := range []struct {
		refresh string
		want    string
		noted   bool
	}{
		{`"30s"`, "30s", false},
		{`"1d"`, "1d", false},
		{`"7d"`, "7d", false},
		{`"500ms"`, "", true},
		{`"1w"`, "", true},
		{`""`, "", false},
		{`false`, "", false},
		{`5`, "", true},
	} {
		res := convertDoc(t, dashboardJSON(`"refresh":`+tt.refresh, panelJSON(1, 0, 0, 12, 4, "")))
		if res.Spec.Refresh != tt.want {
			t.Errorf("refresh %s: got %q, want %q", tt.refresh, res.Spec.Refresh, tt.want)
		}
		if noted := len(notesFor(res, "refresh")) == 1; noted != tt.noted {
			t.Errorf("refresh %s: noted = %v, want %v", tt.refresh, noted, tt.noted)
		}
	}
}

func TestEpochMillisecondTimeRange(t *testing.T) {
	for _, tt := range []struct {
		name, time string
		from, to   string
		noted      bool
	}{
		{"both epoch", `{"from":"1700000000000","to":"1700003600000"}`, "2023-11-14T22:13:20Z", "2023-11-14T23:13:20Z", false},
		{"with milliseconds", `{"from":"1700000000123","to":"now"}`, "2023-11-14T22:13:20.123Z", "now", false},
		{"relative and epoch", `{"from":"now-1h","to":"1700003600000"}`, "now-1h", "2023-11-14T23:13:20Z", false},
		{"past the year 9999", `{"from":"999999999999999","to":"now"}`, "now-6h", "now", true},
		{"epoch in reverse order", `{"from":"1700003600000","to":"1700000000000"}`, "now-6h", "now", true},
		{"not a time", `{"from":"yesterday","to":"now"}`, "now-6h", "now", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := convertDoc(t, dashboardJSON(`"time":`+tt.time, panelJSON(1, 0, 0, 12, 4, "")))
			if res.Spec.Time.From != tt.from || res.Spec.Time.To != tt.to {
				t.Fatalf("time = %+v, want %s to %s", res.Spec.Time, tt.from, tt.to)
			}
			if noted := len(notesFor(res, "time")) > 0; noted != tt.noted {
				t.Fatalf("time notes = %+v, want noted %v", notesFor(res, "time"), tt.noted)
			}
		})
	}
}

func TestCursorSync(t *testing.T) {
	tests := []struct {
		name     string
		extra    string
		want     spec.CursorSync
		wantNote bool
	}{
		{name: "no setting", extra: ``, want: ""},
		{name: "tooltip 0", extra: `"graphTooltip":0`, want: ""},
		{name: "tooltip 1", extra: `"graphTooltip":1`, want: spec.CursorSyncCrosshair},
		{name: "tooltip 2", extra: `"graphTooltip":2`, want: spec.CursorSyncTooltip},
		{name: "legacy crosshair true", extra: `"sharedCrosshair":true`, want: spec.CursorSyncCrosshair},
		{name: "legacy crosshair false", extra: `"sharedCrosshair":false`, want: ""},
		{name: "tooltip wins over legacy crosshair", extra: `"sharedCrosshair":false,"graphTooltip":2`, want: spec.CursorSyncTooltip},
		{name: "unknown tooltip number", extra: `"graphTooltip":3`, want: "", wantNote: true},
		{name: "tooltip as string", extra: `"graphTooltip":"1"`, want: "", wantNote: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := convertDoc(t, dashboardJSON(tt.extra, panelJSON(1, 0, 0, 12, 4, "")))
			if res.Spec.CursorSync != tt.want {
				t.Fatalf("cursorSync = %q, want %q", res.Spec.CursorSync, tt.want)
			}
			notes := notesFor(res, "shared crosshair")
			if tt.wantNote != (len(notes) == 1 && hasPath(notes, "graphTooltip")) || len(notes) > 1 {
				t.Fatalf("shared crosshair notes = %+v, want note: %v", notes, tt.wantNote)
			}
			if notes := notesFor(res, "dashboard options"); len(notes) != 0 {
				t.Fatalf("dashboard options notes = %+v, want none", notes)
			}
		})
	}
}

func TestTimeSettings(t *testing.T) {
	tests := []struct {
		name      string
		extra     string
		week      spec.WeekStart
		fiscal    int
		delay     string
		zone      string
		noteTitle string
		notePath  string
	}{
		{name: "no settings", extra: ``},
		{name: "browser week start", extra: `"weekStart":""`},
		{name: "monday", extra: `"weekStart":"monday"`, week: spec.WeekStartMonday},
		{name: "sunday", extra: `"weekStart":"sunday"`, week: spec.WeekStartSunday},
		{name: "saturday", extra: `"weekStart":"saturday"`, week: spec.WeekStartSaturday},
		{name: "unknown week start", extra: `"weekStart":"friday"`, noteTitle: "week start", notePath: "weekStart"},
		{name: "week start as a number", extra: `"weekStart":1`, noteTitle: "week start", notePath: "weekStart"},
		{name: "fiscal year in January", extra: `"fiscalYearStartMonth":0`},
		{name: "fiscal year in April", extra: `"fiscalYearStartMonth":3`, fiscal: 3},
		{name: "fiscal year in December", extra: `"fiscalYearStartMonth":11`, fiscal: 11},
		{name: "fiscal year month past December", extra: `"fiscalYearStartMonth":12`, noteTitle: "fiscal year", notePath: "fiscalYearStartMonth"},
		{name: "negative fiscal year month", extra: `"fiscalYearStartMonth":-1`, noteTitle: "fiscal year", notePath: "fiscalYearStartMonth"},
		{name: "fiscal year month as text", extra: `"fiscalYearStartMonth":"3"`, noteTitle: "fiscal year", notePath: "fiscalYearStartMonth"},
		{name: "now delay in minutes", extra: `"timepicker":{"nowDelay":"1m"}`, delay: "1m"},
		{name: "now delay in seconds", extra: `"timepicker":{"nowDelay":"30s"}`, delay: "30s"},
		{name: "empty now delay", extra: `"timepicker":{"nowDelay":""}`},
		{name: "now delay in days", extra: `"timepicker":{"nowDelay":"1d"}`, noteTitle: "now delay", notePath: "timepicker.nowDelay"},
		{name: "zone without a slash", extra: `"timezone":"EST"`, zone: "EST"},
		{name: "zone with an offset name", extra: `"timezone":"Etc/GMT+5"`, zone: "Etc/GMT+5"},
		{name: "zone tzdata does not know", extra: `"timezone":"Mars/Base"`, noteTitle: "timezone", notePath: "timezone"},
		{name: "browser zone", extra: `"timezone":"browser"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := convertDoc(t, dashboardJSON(tt.extra, panelJSON(1, 0, 0, 12, 4, "")))
			got := res.Spec
			if got.WeekStart != tt.week || got.FiscalYearStartMonth != tt.fiscal || got.NowDelay != tt.delay || got.Timezone != tt.zone {
				t.Fatalf("week %q, fiscal %d, delay %q, zone %q; want %q, %d, %q, %q", got.WeekStart, got.FiscalYearStartMonth, got.NowDelay, got.Timezone, tt.week, tt.fiscal, tt.delay, tt.zone)
			}
			if notes := notesFor(res, "dashboard options"); len(notes) != 0 {
				t.Fatalf("dashboard options notes = %+v, want none", notes)
			}
			if tt.noteTitle == "" {
				for _, title := range []string{"week start", "fiscal year", "now delay", "timezone"} {
					if notes := notesFor(res, title); len(notes) != 0 {
						t.Fatalf("%s notes = %+v, want none", title, notes)
					}
				}
				return
			}
			if notes := notesFor(res, tt.noteTitle); len(notes) != 1 || !hasPath(notes, tt.notePath) {
				t.Fatalf("%s notes = %+v, want one for %s", tt.noteTitle, notes, tt.notePath)
			}
		})
	}
}

func TestTimeRangeWithFiscalRounding(t *testing.T) {
	res := convertDoc(t, dashboardJSON(`"time":{"from":"now-1y/fy","to":"now/fQ"}`, panelJSON(1, 0, 0, 12, 4, "")))
	if res.Spec.Time.From != "now-1y/fy" || res.Spec.Time.To != "now/fQ" {
		t.Fatalf("time = %+v, want the fiscal range kept", res.Spec.Time)
	}
	if notes := notesFor(res, "time"); len(notes) != 0 {
		t.Fatalf("time notes = %+v, want none", notes)
	}
}

func TestUnknownDashboardOptionIsReported(t *testing.T) {
	res := convertDoc(t, dashboardJSON(`"somethingNew":true`, panelJSON(1, 0, 0, 12, 4, "")))
	if notes := notesFor(res, "dashboard options"); len(notes) != 1 || !hasPath(notes, "somethingNew") {
		t.Fatalf("notes = %+v, want the unknown key reported", notes)
	}
}

func TestLinksWithForeignSchemesAreNotKept(t *testing.T) {
	urls := []struct {
		url  string
		kept bool
	}{
		{"https://example.com/a", true},
		{"HTTP://example.com/a", true},
		{"mailto:ops@example.com", true},
		{"/d/abc/other?orgId=1", true},
		{"${__url_time_range}", true},
		{"https://example.com/?q=a:b", true},
		{"javascript:alert(1)", false},
		{"JaVaScRiPt:alert(1)", false},
		{"java\\tscript:alert(1)", false},
		{" data:text/html,<script>alert(1)</script>", false},
		{"vbscript:msgbox(1)", false},
	}
	var dashLinks, panelLinks []string
	for _, u := range urls {
		dashLinks = append(dashLinks, `{"type":"link","title":"L","url":"`+u.url+`"}`)
		panelLinks = append(panelLinks, `{"title":"L","url":"`+u.url+`"}`)
	}
	res := convertDoc(t, dashboardJSON(`"links":[`+strings.Join(dashLinks, ",")+`]`, `{"id":1,"type":"timeseries","title":"P1","gridPos":{"x":0,"y":0,"w":12,"h":4},"datasource":{"type":"prometheus","uid":"prom"},`+upTarget+`,"links":[`+strings.Join(panelLinks, ",")+`]}`))
	wantKept := 0
	for _, u := range urls {
		if u.kept {
			wantKept++
		}
	}
	if got := len(res.Spec.Links); got != wantKept {
		t.Errorf("dashboard links kept = %d, want %d: %+v", got, wantKept, res.Spec.Links)
	}
	panel := res.Spec.AllPanels()[0].Panel
	if got := len(panel.Links); got != wantKept {
		t.Errorf("panel links kept = %d, want %d: %+v", got, wantKept, panel.Links)
	}
	var kept []string
	for _, link := range res.Spec.Links {
		kept = append(kept, link.URL)
	}
	for _, link := range panel.Links {
		kept = append(kept, link.URL)
	}
	for _, url := range kept {
		if strings.Contains(strings.ToLower(url), "script:") || strings.Contains(url, "data:") {
			t.Errorf("link %q was kept", url)
		}
	}
	if !hasPath(notesFor(res, "dashboard links"), "links[6]") {
		t.Errorf("the dropped dashboard links are not reported: %+v", res.Report.Notes)
	}
	if !slices.Contains(panelLine(t, res, "p1").Paths, "links[6]") {
		t.Errorf("the dropped panel links are not reported: %+v", panelLine(t, res, "p1"))
	}
}
