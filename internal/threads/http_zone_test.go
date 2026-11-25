package threads

import (
	"context"
	"testing"
)

type zoneProbe struct{}

func TestWithRequestZoneKeepsTheContextForAnAbsentOrUnknownZone(t *testing.T) {
	ctx := context.WithValue(context.Background(), zoneProbe{}, 1)
	named := func(s string) *string { return &s }
	for _, tc := range []struct {
		name    string
		zone    *string
		changed bool
	}{
		{"absent", nil, false},
		{"utc", named("UTC"), true},
		{"kolkata", named("Asia/Kolkata"), true},
		{"unknown", named("Mars/Olympus"), false},
		{"local", named("Local"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := withRequestZone(ctx, tc.zone)
			if changed := got != ctx; changed != tc.changed {
				t.Fatalf("context changed = %v, want %v", changed, tc.changed)
			}
		})
	}
}
