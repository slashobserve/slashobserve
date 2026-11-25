package tools

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/accessrequests"
	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func newAccessRequestRig(t *testing.T) activityRig {
	t.Helper()
	database := dbtest.New(t)
	for _, p := range []auth.Principal{viewerCaller, adminCaller, editorCaller} {
		database.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name, role) VALUES ($1::uuid, 'https://idp', $1, $2, $3, $4)`, p.ID, p.Email, p.Name, string(p.Role))
	}
	access := folders.NewAccess(database.DB)
	return activityRig{
		registry: New(Deps{
			Activity:       activity.NewStore(database.DB, access),
			AccessRequests: accessrequests.New(database.DB, access),
			Logger:         slog.New(slog.DiscardHandler),
		}),
		db: database,
	}
}

func callAs(t *testing.T, r activityRig, by auth.Principal, tool, input string) Result {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), by), tool, []byte(input))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

func TestRequestCommentAccessToolAsksOnceAndTheAdminListsIt(t *testing.T) {
	r := newAccessRequestRig(t)
	d := r.dashboard(t, pgtype.UUID{})
	input := `{"object_kind":"dashboard","object_id":"` + d.String() + `","note":"on call"}`

	res := callAs(t, r, viewerCaller, nameRequestCommentAccess, input)
	created, ok := res.Content.(accessrequests.RequestJSON)
	if res.IsError || !ok || created.Object.Title != "Checkout" || created.Note != "on call" || created.RequestedBy.Name != "Vic" || !created.Folder.General {
		t.Fatalf("first ask = %#v", res.Content)
	}

	res = callAs(t, r, viewerCaller, nameRequestCommentAccess, input)
	open, ok := res.Content.(requestOpenBody)
	if !res.IsError || !ok || open.Code != accessrequests.CodeRequestOpen || open.RequestedAt != created.CreatedAt || !strings.Contains(open.Message, created.CreatedAt[:19]) {
		t.Fatalf("second ask = %#v, want request_open at %s", res.Content, created.CreatedAt)
	}

	res = callAs(t, r, adminCaller, nameListAccessRequests, `{}`)
	list, ok := res.Content.(accessRequestListOut)
	if res.IsError || !ok || len(list.Items) != 1 || list.Items[0].ID != created.ID || list.Items[0].Folder != created.Folder {
		t.Fatalf("Admin's list = %#v", res.Content)
	}
	res = callAs(t, r, viewerCaller, nameListAccessRequests, `{}`)
	if list, ok := res.Content.(accessRequestListOut); res.IsError || !ok || len(list.Items) != 0 {
		t.Fatalf("Viewer's list = %#v, want empty", res.Content)
	}
}

func TestRequestCommentAccessToolRefusals(t *testing.T) {
	r := newAccessRequestRig(t)
	d := r.dashboard(t, pgtype.UUID{})
	unknown := "99999999-9999-4999-8999-999999999999"
	cases := []struct {
		name  string
		by    auth.Principal
		input string
		code  string
	}{
		{"an unknown dashboard", viewerCaller, `{"object_kind":"dashboard","object_id":"` + unknown + `"}`, codeNotFound},
		{"an unknown observation", viewerCaller, `{"object_kind":"observation","object_id":"` + unknown + `"}`, codeNotFound},
		{"an Editor", editorCaller, `{"object_kind":"dashboard","object_id":"` + d.String() + `"}`, accessrequests.CodeCanComment},
		{"a two-line note", viewerCaller, `{"object_kind":"dashboard","object_id":"` + d.String() + `","note":"a\nb"}`, accessrequests.CodeInvalidNote},
		{"a folder", viewerCaller, `{"object_kind":"folder","object_id":"` + d.String() + `"}`, "invalid_request"},
	}
	for _, tc := range cases {
		res := callAs(t, r, tc.by, nameRequestCommentAccess, tc.input)
		if !res.IsError || errorCodeOf(t, res) != tc.code {
			t.Errorf("%s: %#v, want %s", tc.name, res.Content, tc.code)
		}
	}
}
