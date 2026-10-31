package grafana

import "testing"

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
