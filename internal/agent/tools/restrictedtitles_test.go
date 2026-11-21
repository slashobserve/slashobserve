package tools

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestAnAdminsRunLeavesOutTitlesInAdminsOnlyFoldersUnlessOthersReadWhatItWrites(t *testing.T) {
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
		{"an Admin writing where only Admins read", in(adminCaller, datasource.AudienceAdmins), true, false, leaveOutRestrictedTitles},
		{"an Admin writing where only Admins read, asking for them", in(adminCaller, datasource.AudienceAdmins), true, true, listRestrictedTitles},
		{"an Admin writing where others read", in(adminCaller, datasource.AudienceEveryone), true, false, listEveryTitle},
		{"an Admin writing where others read, asking for them", in(adminCaller, datasource.AudienceEveryone), true, true, refuseRestrictedTitles},
	}
	for _, c := range cases {
		if got := titleRuleFor(c.run, c.inRun, c.include); got != c.want {
			t.Errorf("%s: rule %d, want %d", c.name, got, c.want)
		}
	}
}

func TestARunKeepsUnmarkedTheTitlesOfWhatItIsAboutAndOfTheFoldersItWritesTo(t *testing.T) {
	home, other := mustUUID(t, "11111111-1111-4111-8111-111111111111"), mustUUID(t, "22222222-2222-4222-8222-222222222222")
	about := "33333333-3333-4333-8333-333333333333"
	run := Run{DashboardID: about, WriteFolders: []pgtype.UUID{home}, Access: datasource.Access{By: adminCaller, Audience: datasource.AudienceAdmins}}
	titles := &restrictedTitles{rule: titleRuleFor(run, true, false), run: run, adminsOnly: map[pgtype.UUID]bool{home: true, other: true}}
	if !titles.keep(home, "44444444-4444-4444-8444-444444444444") || !titles.keep(other, about) || titles.listed || titles.leftOut != 0 {
		t.Fatalf("what the run is about and its own folder: left out %d, listed %v", titles.leftOut, titles.listed)
	}
	if titles.keep(other, "44444444-4444-4444-8444-444444444444") || titles.leftOut != 1 {
		t.Fatalf("another folder only Admins read: left out %d", titles.leftOut)
	}
}
