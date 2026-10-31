package grafana

import (
	"strconv"
	"strings"
	"testing"
)

func TestPanelTimezoneImportsASingleZone(t *testing.T) {
	cases := []struct {
		name, typ, zones, want string
	}{
		{"utc", "timeseries", `["utc"]`, "utc"},
		{"utc in capitals", "timeseries", `["UTC"]`, "utc"},
		{"browser", "timeseries", `["browser"]`, "browser"},
		{"named zone", "timeseries", `["America/New_York"]`, "America/New_York"},
		{"a single zone written as text", "timeseries", `"Asia/Kolkata"`, "Asia/Kolkata"},
		{"the same zone twice", "timeseries", `["utc","UTC"]`, "utc"},
		{"state timeline", "state-timeline", `["utc"]`, "utc"},
		{"status history", "status-history", `["utc"]`, "utc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc(tc.typ, upTarget+`,"options":{"timezone":`+tc.zones+`}`))
			requireImported(t, line)
			if panel.Timezone != tc.want {
				t.Fatalf("timezone = %q, want %q", panel.Timezone, tc.want)
			}
		})
	}
}

func TestPanelTimezoneOfTheDashboardIsSilent(t *testing.T) {
	for _, zones := range []string{`[""]`, `["default"]`, `[]`, `null`, `""`} {
		t.Run(zones, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("timeseries", upTarget+`,"options":{"timezone":`+zones+`}`))
			requireImported(t, line)
			if panel.Timezone != "" {
				t.Fatalf("timezone = %q, want none", panel.Timezone)
			}
		})
	}
}

func TestPanelTimezoneListsAndUnknownZonesAreReported(t *testing.T) {
	for _, zones := range []string{
		`["","utc"]`,
		`["default","America/New_York"]`,
		`["utc","browser"]`,
		`["Mars/Olympus_Mons"]`,
		`["not a zone"]`,
		`[1]`,
		`5`,
	} {
		t.Run(zones, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("timeseries", upTarget+`,"options":{"timezone":`+zones+`}`))
			requireReported(t, line, "options.timezone", timezoneListReason)
			if panel.Timezone != "" {
				t.Fatalf("timezone = %q, want none", panel.Timezone)
			}
		})
	}
}

func TestPanelTimezoneOnAPanelThatDrawsNoTimesIsLeftToTheReport(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("stat", upTarget+`,"options":{"timezone":["utc"]}`))
	if panel.Timezone != "" {
		t.Fatalf("timezone = %q, want none", panel.Timezone)
	}
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "options.timezone" {
		t.Fatalf("status = %s, paths %v, want options.timezone reported", line.Status, line.Paths)
	}
}

func TestDashboardAndPanelShareTheZoneRule(t *testing.T) {
	cases := []struct {
		zone, want string
	}{
		{"UTC", "utc"},
		{"utc", "utc"},
		{"Asia/Kolkata", "Asia/Kolkata"},
		{"browser", ""},
		{"default", ""},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.zone, func(t *testing.T) {
			doc := strings.Replace(panelDoc("stat", upTarget), `"schemaVersion":39`, `"schemaVersion":39,"timezone":`+strconv.Quote(tc.zone), 1)
			res := convertDoc(t, doc)
			if res.Spec.Timezone != tc.want {
				t.Fatalf("dashboard timezone = %q, want %q", res.Spec.Timezone, tc.want)
			}
		})
	}
}

func TestHideTimeOverrideImportsOnDataPanels(t *testing.T) {
	cases := []struct {
		name, typ, members string
		want               bool
	}{
		{"with a time from", "stat", `"timeFrom":"1h","hideTimeOverride":true`, true},
		{"with a time shift", "timeseries", `"timeShift":"1d","hideTimeOverride":true`, true},
		{"without an override", "stat", `"hideTimeOverride":true`, true},
		{"not hidden", "stat", `"timeFrom":"1h","hideTimeOverride":false`, false},
		{"absent", "stat", `"timeFrom":"1h"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc(tc.typ, upTarget+","+tc.members))
			requireImported(t, line)
			if panel.HideTimeOverride != tc.want {
				t.Fatalf("hideTimeOverride = %v, want %v", panel.HideTimeOverride, tc.want)
			}
		})
	}
}

func TestHideTimeOverrideOnATextPanelIsConsumedSilently(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("text", `"options":{"content":"hi"},"hideTimeOverride":true`))
	requireImported(t, line)
	if panel.HideTimeOverride {
		t.Fatal("a text panel has no time override to hide")
	}
}
