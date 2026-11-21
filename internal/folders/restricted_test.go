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
		{1, 1, "1 more is not listed; 1 of them is in people's My Dashboards, which only its owner can open, so the owner must move or remove it; you cannot see which"},
		{2, 3, "2 more are not listed; 3 of them are in people's My Dashboards, which only their owners can open, so the owners must move or remove them; you cannot see which"},
	}
	for _, tc := range cases {
		got := RestrictedAudience("audit", nil, tc.unlisted, tc.hidden).Error()
		if !strings.Contains(got, tc.want) {
			t.Errorf("unlisted %d hidden %d: %q lacks %q", tc.unlisted, tc.hidden, got, tc.want)
		}
	}
}

func TestTheOpeningRefusalListsWhatItCanAndCountsTheRest(t *testing.T) {
	items := []ContentItem{{Kind: ContentDashboard, Title: "Salaries"}, {Kind: ContentSavedSearch, Title: "Pay"}}
	one := RestrictedOpening(items[:1], 0, "payroll").Error()
	many := RestrictedOpening(items, 3, "payroll").Error()
	if !strings.Contains(one, `the dashboard "Salaries" in this folder reads the datasource "payroll"`) {
		t.Errorf("one item: %q", one)
	}
	if !strings.HasPrefix(many, "5 items in this folder read datasources only Admins can query") {
		t.Errorf("many items: %q", many)
	}
}
