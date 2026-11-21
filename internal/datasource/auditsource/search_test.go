package auditsource

import (
	"encoding/json"
	"testing"
)

func TestTrackTotalHitsCountsAtMostTheResultWindow(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want int
	}{
		{true, resultWindow},
		{false, 0},
		{json.Number("250"), 250},
		{json.Number("1000000000"), resultWindow},
	} {
		if got, err := trackedTotal(tc.in); err != nil || got != tc.want {
			t.Errorf("trackedTotal(%v) = %d, %v, want %d", tc.in, got, err, tc.want)
		}
	}
	if _, err := trackedTotal(json.Number("-1")); err == nil {
		t.Error("a negative track_total_hits was accepted")
	}
}
