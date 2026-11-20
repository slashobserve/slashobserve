package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestZoneLabel(t *testing.T) {
	at := time.Date(2026, 11, 19, 14, 22, 0, 0, time.UTC)
	tests := map[string]string{
		"Asia/Kolkata":      "IST",
		"Asia/Dubai":        "UTC+04:00",
		"America/Sao_Paulo": "UTC-03:00",
		"Europe/London":     "GMT",
		"Asia/Kathmandu":    "UTC+05:45",
	}
	for zone, want := range tests {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		if got := ZoneLabel(loc, at); got != want {
			t.Errorf("ZoneLabel(%s) = %q, want %q", zone, got, want)
		}
	}
}

func TestAFindingInUTCIsSentBackWhenThePersonReadsAnotherZone(t *testing.T) {
	at := time.Date(2026, 11, 19, 14, 22, 0, 0, time.UTC)
	tests := []struct {
		name, zone, markdown string
		wrong                bool
	}{
		{"no zone known keeps UTC", "", "Errors rose at 14:22 UTC.", false},
		{"a UTC time for a person in Kolkata", "Asia/Kolkata", "Errors rose at 14:22 UTC.", true},
		{"an RFC 3339 time for a person in Kolkata", "Asia/Kolkata", "Errors rose at 2026-11-19T14:22:00Z.", true},
		{"the person's zone", "Asia/Kolkata", "Errors rose at 19:52 IST.", false},
		{"both zones", "Asia/Kolkata", "Errors rose at 19:52 IST (14:22 UTC).", false},
		{"an offset label", "Asia/Dubai", "Errors rose at 18:22 UTC+04:00.", false},
		{"a UTC time beside an offset label is still caught", "Asia/Kolkata", "Errors rose at 18:22 UTC+04:00 and 14:22 UTC.", true},
		{"no times at all", "Asia/Kolkata", "Errors rose after the deploy; the dashboard is set to UTC.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, wrong := zoneMismatch(tt.markdown, tt.zone, at)
			if wrong != tt.wrong {
				t.Fatalf("wrong = %v, want %v", wrong, tt.wrong)
			}
			if !wrong {
				return
			}
			body, err := json.Marshal(res.Content)
			if err != nil || !res.IsError || !strings.Contains(string(body), "Asia/Kolkata") || !strings.Contains(string(body), "14:05 IST") {
				t.Fatalf("res = %s, %v", body, err)
			}
		})
	}
}
