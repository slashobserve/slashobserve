package threads

import (
	"context"
	"testing"
)

func TestWithRequestZoneAcceptsKnownZonesAndRefusesTheRest(t *testing.T) {
	named := func(s string) *string { return &s }
	for _, tc := range []struct {
		name string
		zone *string
		ok   bool
	}{
		{"absent", nil, true},
		{"utc", named("UTC"), true},
		{"kolkata", named("Asia/Kolkata"), true},
		{"unknown", named("Mars/Olympus"), false},
		{"local", named("Local"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := withRequestZone(context.Background(), tc.zone)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok %v", err, tc.ok)
			}
		})
	}
}
