package folders

import (
	"strings"
	"testing"
)

func TestARefusalCountsItsItemsInTheRightNumber(t *testing.T) {
	cases := []struct {
		unlisted, hidden int
		want             string
	}{
		{1, 1, "1 more is not listed; 1 of them is in people's My Dashboards, which only its owner can open"},
		{2, 3, "2 more are not listed; 3 of them are in people's My Dashboards, which only their owners can open"},
	}
	for _, tc := range cases {
		got := RestrictedAudience("audit", nil, tc.unlisted, tc.hidden).Error()
		if !strings.Contains(got, tc.want) {
			t.Errorf("unlisted %d hidden %d: %q lacks %q", tc.unlisted, tc.hidden, got, tc.want)
		}
	}
}
