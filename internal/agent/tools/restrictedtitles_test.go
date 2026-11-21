package tools

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestOnlyAnAdminsPrivateRunLeavesOutTitlesInAdminsOnlyFolders(t *testing.T) {
	in := func(by auth.Principal, audience datasource.Audience) Run {
		return Run{Access: datasource.Access{By: by, Audience: audience}}
	}
	cases := []struct {
		name    string
		run     Run
		inRun   bool
		include bool
		want    titleRule
	}{
		{"outside a run", Run{}, false, false, listEveryTitle},
		{"outside a run, asking for them", Run{}, false, true, listEveryTitle},
		{"an Admin's private chat", in(adminCaller, datasource.AudienceCaller), true, false, leaveOutRestrictedTitles},
		{"an Admin's private chat, asking for them", in(adminCaller, datasource.AudienceCaller), true, true, listRestrictedTitles},
		{"an Editor's private chat", in(editorCaller, datasource.AudienceCaller), true, false, listEveryTitle},
		{"an Admin writing where only Admins read", in(adminCaller, datasource.AudienceAdmins), true, false, listEveryTitle},
		{"an Admin writing where others read", in(adminCaller, datasource.AudienceEveryone), true, false, listEveryTitle},
		{"an Admin writing where others read, asking for them", in(adminCaller, datasource.AudienceEveryone), true, true, refuseRestrictedTitles},
	}
	for _, c := range cases {
		if got := titleRuleFor(c.run, c.inRun, c.include); got != c.want {
			t.Errorf("%s: rule %d, want %d", c.name, got, c.want)
		}
	}
}
