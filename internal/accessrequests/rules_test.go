package accessrequests

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestCleanNoteKeepsOneTrimmedLineOfAtMost200Characters(t *testing.T) {
	cases := []struct {
		raw, want string
		err       error
	}{
		{"", "", nil},
		{"  on call this week  ", "on call this week", nil},
		{strings.Repeat("é", 200), strings.Repeat("é", 200), nil},
		{strings.Repeat("x", 201), "", ErrInvalidNote},
		{"two\nlines", "", ErrInvalidNote},
		{"carriage\rreturn", "", ErrInvalidNote},
		{"bad \xff byte", "", ErrInvalidNote},
	}
	for _, tc := range cases {
		got, err := cleanNote(tc.raw)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("cleanNote(%q) = %q, %v; want %q, %v", tc.raw, got, err, tc.want, tc.err)
		}
	}
}

func TestMayAskOnlyWhoCanViewButNotComment(t *testing.T) {
	cases := []struct {
		name  string
		level folders.Level
		want  error
	}{
		{"cannot see the folder", folders.LevelNone, ErrNotFound},
		{"Viewer", folders.LevelView, nil},
		{"Commenter", folders.LevelComment, ErrCanComment},
		{"Editor", folders.LevelEdit, ErrCanComment},
		{"folder Admin", folders.LevelAdmin, ErrCanComment},
	}
	for _, tc := range cases {
		if got := mayAsk(folders.AbilitiesFor(tc.level)); !errors.Is(got, tc.want) {
			t.Errorf("%s: mayAsk = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestChooseGrantersPrefersWhoManagesTheFolderOverTheWorkspaceAdmins(t *testing.T) {
	creator := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	granted := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	viewerCreator := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	admin := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	admins := []pgtype.UUID{admin}

	cases := []struct {
		name       string
		candidates []candidate
		want       []pgtype.UUID
	}{
		{"an Editor who created the folder", []candidate{{id: creator, role: auth.RoleEditor, facts: folders.Facts{Creator: true}}}, []pgtype.UUID{creator}},
		{"an Admin entry on a restricted folder", []candidate{{id: granted, role: auth.RoleViewer, facts: folders.Facts{Restricted: true, Grants: []folders.Level{folders.LevelAdmin}}}}, []pgtype.UUID{granted}},
		{"a creator who is now a Viewer manages nothing", []candidate{{id: viewerCreator, role: auth.RoleViewer, facts: folders.Facts{Creator: true}}}, admins},
		{"no one manages the folder", nil, admins},
		{"both kinds of manager", []candidate{
			{id: creator, role: auth.RoleEditor, facts: folders.Facts{Creator: true}},
			{id: granted, role: auth.RoleCommenter, facts: folders.Facts{Restricted: true, Grants: []folders.Level{folders.LevelAdmin}}},
		}, []pgtype.UUID{creator, granted}},
	}
	for _, tc := range cases {
		if got := chooseGranters(tc.candidates, admins); !slices.Equal(got, tc.want) {
			t.Errorf("%s: granters = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCanCommentNowReadsTheRequestersCurrentAccess(t *testing.T) {
	cases := []struct {
		name string
		row  dbsqlc.AccessRequestsToSettleRow
		want bool
	}{
		{"a Viewer on an open folder", dbsqlc.AccessRequestsToSettleRow{Role: "viewer"}, false},
		{"raised to Commenter", dbsqlc.AccessRequestsToSettleRow{Role: "commenter"}, true},
		{"given Comment by an entry", dbsqlc.AccessRequestsToSettleRow{Role: "viewer", Listed: true, Grants: []string{"comment"}}, true},
		{"given View by an entry", dbsqlc.AccessRequestsToSettleRow{Role: "viewer", Listed: true, Grants: []string{"view"}}, false},
		{"a deactivated or deleted requester", dbsqlc.AccessRequestsToSettleRow{Role: ""}, false},
	}
	for _, tc := range cases {
		if got := canCommentNow(tc.row); got != tc.want {
			t.Errorf("%s: canCommentNow = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseKindTakesDashboardsAndObservationsOnly(t *testing.T) {
	for raw, want := range map[string]bool{"dashboard": true, "observation": true, "folder": false, "": false, "Dashboard": false} {
		if _, ok := ParseKind(raw); ok != want {
			t.Errorf("ParseKind(%q) ok = %v, want %v", raw, ok, want)
		}
	}
}

func TestAnOpenRequestMessageSaysSinceWhen(t *testing.T) {
	open := &OpenError{RequestedAt: time.Date(2026, 11, 20, 9, 0, 0, 0, time.FixedZone("IST", 5*3600+1800))}
	want := "you already asked for comment access here at 2026-11-20T03:30:00Z and the request is still open"
	if got := open.Message(); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}
