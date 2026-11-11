package webhooks

import "testing"

func TestStateOfFollowsEnabledThenFailures(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		failures int
		want     string
	}{
		{"enabled with no failures", true, 0, "sending"},
		{"enabled after a failed delivery", true, 1, "retrying"},
		{"enabled after many failures", true, 4, "retrying"},
		{"disabled with no failures", false, 0, "disabled"},
		{"disabled beats retrying", false, 3, "disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stateOf(tt.enabled, tt.failures); got != tt.want {
				t.Errorf("stateOf = %s, want %s", got, tt.want)
			}
		})
	}
}
