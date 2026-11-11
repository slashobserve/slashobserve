package public

import "testing"

func TestLinkJSONStateFollowsExpiredAndLeavesTheFlagsAlone(t *testing.T) {
	tests := []struct {
		name string
		link Link
		want string
	}{
		{"live link", Link{Usable: true}, "active"},
		{"expired link", Link{Expired: true}, "expired"},
		{"unusable but not expired", Link{}, "active"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linkJSONOf(tt.link)
			if got.State != tt.want || got.Expired != tt.link.Expired || got.Usable != tt.link.Usable {
				t.Errorf("linkJSONOf = state %s expired %v usable %v, want state %s", got.State, got.Expired, got.Usable, tt.want)
			}
		})
	}
}
