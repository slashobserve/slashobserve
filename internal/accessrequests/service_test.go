package accessrequests

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

var (
	ada    = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Email: "ada@example.com", Role: auth.RoleViewer}
	ben    = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Ben", Email: "ben@example.com", Role: auth.RoleEditor}
	cleo   = auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Name: "Cleo", Email: "cleo@example.com", Role: auth.RoleViewer}
	root   = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Root", Email: "root@example.com", Role: auth.RoleAdmin}
	editor = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Eve", Email: "eve@example.com", Role: auth.RoleEditor}
)

type rig struct {
	db      *dbtest.Database
	service *Service
	store   *activity.Store
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	for _, p := range []auth.Principal{ada, ben, cleo, root, editor} {
		d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name, role) VALUES ($1::uuid, 'https://idp', $1, $2, $3, $4)`, p.ID, p.Email, p.Name, string(p.Role))
	}
	access := folders.NewAccess(d.DB)
	return rig{db: d, service: New(d.DB, access), store: activity.NewStore(d.DB, access)}
}

func (r rig) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r rig) uuid(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r rig) folderBy(t *testing.T, title string, by auth.Principal) pgtype.UUID {
	t.Helper()
	f, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{
		Title:         title,
		CreatedByKind: pgtype.Text{String: string(by.Kind), Valid: true},
		CreatedByID:   r.uuid(t, by.ID),
		CreatedByName: pgtype.Text{String: by.Name, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}

func (r rig) grant(t *testing.T, folder pgtype.UUID, who auth.Principal, level string) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO folder_permissions (folder_id, user_id, level) VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (folder_id, user_id) WHERE user_id IS NOT NULL DO UPDATE SET level = EXCLUDED.level`, folder.String(), who.ID, level)
}

func (r rig) dashboard(t *testing.T, title string, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	id := r.uuid(t, r.scalar(t, `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ($1, 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, title))
	if folder.Valid {
		r.db.Exec(t, `UPDATE dashboards SET folder_id = $2::uuid WHERE id = $1::uuid`, id.String(), folder.String())
	}
	return id
}

func (r rig) observation(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	return r.uuid(t, r.scalar(t, `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name) VALUES (general_folder_id(), $1, 'user', $2::uuid, 'Ben') RETURNING id::text`, title, ben.ID))
}

func (r rig) ask(t *testing.T, by auth.Principal, obj Object, note string) Request {
	t.Helper()
	req, err := r.service.Request(t.Context(), by, obj, note)
	if err != nil {
		t.Fatalf("%s asks: %v", by.Name, err)
	}
	return req
}

func (r rig) granters(t *testing.T, id string) []string {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT unnest(granters)::text FROM access_requests WHERE id = $1::uuid ORDER BY 1`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			t.Fatal(err)
		}
		out = append(out, g)
	}
	return out
}

func (r rig) inbox(t *testing.T, by auth.Principal, triage activity.Triage) []activity.InboxItem {
	t.Helper()
	page, err := r.store.Inbox(t.Context(), by, activity.ViewForYou, triage, "", 100)
	if err != nil {
		t.Fatalf("inbox of %s: %v", by.Name, err)
	}
	return page.Items
}

func accessItems(items []activity.InboxItem) []activity.InboxItem {
	var out []activity.InboxItem
	for _, item := range items {
		if item.Reason == activity.ReasonAccessRequest {
			out = append(out, item)
		}
	}
	return out
}

func (r rig) open(t *testing.T, by auth.Principal) []string {
	t.Helper()
	items, err := r.service.List(t.Context(), by)
	if err != nil {
		t.Fatalf("list for %s: %v", by.Name, err)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func (r rig) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAViewerAsksAndTheFolderCreatorGetsTheRequestWithTheNote(t *testing.T) {
	r := newRig(t)
	folder := r.folderBy(t, "Payments", ben)
	checkout := r.dashboard(t, "Checkout", folder)

	req := r.ask(t, ada, Object{Kind: KindDashboard, ID: checkout}, "  on call this week ")
	if req.Title != "Checkout" || req.Note != "on call this week" || req.RequestedBy.Name != "Ada" || req.Object.Kind != KindDashboard {
		t.Fatalf("request = %+v", req)
	}
	if got := r.granters(t, req.ID); !slices.Equal(got, []string{ben.ID}) {
		t.Fatalf("granters = %v, want only the folder's creator", got)
	}

	items := accessItems(r.inbox(t, ben, activity.TriageOpen))
	if len(items) != 1 || items[0].Excerpt != "on call this week" || items[0].Type != activity.TypeAccessRequestOpened {
		t.Fatalf("Ben's access items = %+v, want one with the note", items)
	}
	if items[0].Actor.Name != "Ada" {
		t.Fatalf("actor = %+v, want Ada", items[0].Actor)
	}
	if got := accessItems(r.inbox(t, root, activity.TriageOpen)); len(got) != 0 {
		t.Fatalf("an Admin got %d items while the folder has a manager", len(got))
	}
	if got := accessItems(r.inbox(t, cleo, activity.TriageOpen)); len(got) != 0 {
		t.Fatalf("another Viewer got %d items", len(got))
	}
	if got := r.open(t, ben); !slices.Equal(got, []string{req.ID}) {
		t.Fatalf("Ben's open requests = %v", got)
	}
	if got := r.open(t, ada); len(got) != 0 {
		t.Fatalf("the requester lists %v, want nothing: she manages no folder", got)
	}
	if n := r.count(t, `SELECT count(*) FROM access_requests`); n != 1 {
		t.Fatalf("requests = %d", n)
	}
}

func TestAFolderWithNoManagerSendsTheRequestToTheWorkspaceAdmins(t *testing.T) {
	r := newRig(t)
	o := r.observation(t, "Checkout latency")
	req := r.ask(t, ada, Object{Kind: KindObservation, ID: o}, "")
	if got := r.granters(t, req.ID); !slices.Equal(got, []string{root.ID}) {
		t.Fatalf("granters = %v, want the Admin", got)
	}
	items := accessItems(r.inbox(t, root, activity.TriageOpen))
	if len(items) != 1 || items[0].TargetTitle != "Checkout latency" || items[0].Excerpt != "" {
		t.Fatalf("Admin's items = %+v", items)
	}
}

func TestAnAdminEntryOnARestrictedFolderNamesTheGranters(t *testing.T) {
	r := newRig(t)
	folder := r.folderBy(t, "Secret", editor)
	r.grant(t, folder, cleo, "admin")
	r.grant(t, folder, ada, "view")
	d := r.dashboard(t, "Ledger", folder)

	req := r.ask(t, ada, Object{Kind: KindDashboard, ID: d}, "")
	want := []string{cleo.ID, editor.ID}
	slices.Sort(want)
	if got := r.granters(t, req.ID); !slices.Equal(got, want) {
		t.Fatalf("granters = %v, want %v", got, want)
	}
}

func TestOnlyAViewerWhoCanSeeTheObjectMayAsk(t *testing.T) {
	r := newRig(t)
	hidden := r.folderBy(t, "Hidden", editor)
	r.grant(t, hidden, cleo, "view")
	secret := r.dashboard(t, "Secret", hidden)
	general := r.dashboard(t, "Checkout", pgtype.UUID{})
	unknown := r.uuid(t, "99999999-9999-4999-8999-999999999999")

	cases := []struct {
		name string
		by   auth.Principal
		obj  Object
		want error
	}{
		{"a dashboard in a folder she cannot see", ada, Object{Kind: KindDashboard, ID: secret}, ErrNotFound},
		{"an unknown dashboard", ada, Object{Kind: KindDashboard, ID: unknown}, ErrNotFound},
		{"an unknown observation", ada, Object{Kind: KindObservation, ID: unknown}, ErrNotFound},
		{"an Editor who can comment", ben, Object{Kind: KindDashboard, ID: general}, ErrCanComment},
		{"a Commenter by role", auth.Principal{Kind: auth.KindUser, ID: ada.ID, Name: "Ada", Role: auth.RoleCommenter}, Object{Kind: KindDashboard, ID: general}, ErrCanComment},
		{"a public link viewer has no principal id", auth.Principal{Kind: auth.KindUser, ID: "not-a-uuid", Role: auth.RoleViewer}, Object{Kind: KindDashboard, ID: general}, folders.ErrForbidden},
	}
	for _, tc := range cases {
		if _, err := r.service.Request(t.Context(), tc.by, tc.obj, ""); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if n := r.count(t, `SELECT count(*) FROM access_requests`); n != 0 {
		t.Fatalf("requests = %d, want none written", n)
	}
	if n := r.count(t, `SELECT count(*) FROM activity WHERE target_kind = 'access_request'`); n != 0 {
		t.Fatalf("activity rows = %d, want none written", n)
	}
	if _, err := r.service.Request(t.Context(), ada, Object{Kind: KindDashboard, ID: general}, "one\ntwo"); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("a two-line note: err = %v, want ErrInvalidNote", err)
	}
}

func TestAServiceAccountViewerMayAsk(t *testing.T) {
	r := newRig(t)
	account := r.scalar(t, `INSERT INTO service_accounts (name, role) VALUES ('ci', 'viewer') RETURNING id::text`)
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: account, Name: "ci", Role: auth.RoleViewer}
	d := r.dashboard(t, "Checkout", pgtype.UUID{})
	req := r.ask(t, key, Object{Kind: KindDashboard, ID: d}, "nightly report")
	if req.RequestedBy.Kind != "service_account" || req.RequestedBy.Name != "ci" {
		t.Fatalf("requested by = %+v", req.RequestedBy)
	}

	r.db.Exec(t, `UPDATE service_accounts SET role = 'commenter' WHERE id = $1::uuid`, account)
	before := dbsqlc.ServiceAccount{ID: r.uuid(t, account), Role: "viewer"}
	after := dbsqlc.ServiceAccount{ID: r.uuid(t, account), Role: "commenter"}
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return ChangeLog{ChangeLog: activity.AuthChanges{}, Requests: r.service}.ServiceAccountUpdated(t.Context(), q, root, before, after)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.open(t, root); len(got) != 0 {
		t.Fatalf("open after the role change = %v, want the request granted", got)
	}
}

func TestAskingAgainWhileARequestIsOpenSaysSoUntilItCloses(t *testing.T) {
	r := newRig(t)
	folder := r.folderBy(t, "Payments", ben)
	d := r.dashboard(t, "Checkout", folder)
	obj := Object{Kind: KindDashboard, ID: d}
	first := r.ask(t, ada, obj, "")

	_, err := r.service.Request(t.Context(), ada, obj, "again")
	var open *OpenError
	if !errors.As(err, &open) || !open.RequestedAt.Equal(first.CreatedAt) {
		t.Fatalf("second ask: err = %v, want OpenError at %v", err, first.CreatedAt)
	}
	r.ask(t, cleo, obj, "")

	items := accessItems(r.inbox(t, ben, activity.TriageOpen))
	if len(items) != 2 {
		t.Fatalf("Ben's items = %d, want one per requester", len(items))
	}
	var adaItem activity.InboxItem
	for _, item := range items {
		if item.Actor.Name == "Ada" {
			adaItem = item
		}
	}
	if err := r.store.SetItemDone(t.Context(), ben, adaItem.Seq, true); err != nil {
		t.Fatal(err)
	}
	if got := accessItems(r.inbox(t, ben, activity.TriageOpen)); len(got) != 1 || got[0].Actor.Name != "Cleo" {
		t.Fatalf("open items after dismissing Ada's = %+v", got)
	}
	if got := accessItems(r.inbox(t, ben, activity.TriageDone)); len(got) != 1 || got[0].Seq != adaItem.Seq {
		t.Fatalf("done items = %+v, want Ada's", got)
	}
	if got := len(r.open(t, ben)); got != 1 {
		t.Fatalf("open requests = %d, want Cleo's only", got)
	}

	second := r.ask(t, ada, obj, "please")
	if second.ID == first.ID {
		t.Fatal("asking after a dismissal reused the closed request")
	}

	r.db.Exec(t, `UPDATE access_requests SET created_at = now() - interval '31 days' WHERE id = $1::uuid`, second.ID)
	r.ask(t, ada, obj, "")
}

func TestUndoingADismissalAfterAskingAgainLeavesOnlyTheNewerRequestOpen(t *testing.T) {
	r := newRig(t)
	folder := r.folderBy(t, "Payments", ben)
	obj := Object{Kind: KindDashboard, ID: r.dashboard(t, "Checkout", folder)}
	first := r.ask(t, ada, obj, "")
	items := accessItems(r.inbox(t, ben, activity.TriageOpen))
	if len(items) != 1 {
		t.Fatalf("Ben's items = %d, want 1", len(items))
	}
	if err := r.store.SetItemDone(t.Context(), ben, items[0].Seq, true); err != nil {
		t.Fatal(err)
	}
	second := r.ask(t, ada, obj, "please")

	if err := r.store.SetItemDone(t.Context(), ben, items[0].Seq, false); err != nil {
		t.Fatal(err)
	}
	if got := r.open(t, ben); len(got) != 1 || got[0] != second.ID {
		t.Fatalf("open requests = %v, want only the newer %s, not %s", got, second.ID, first.ID)
	}
	if got := accessItems(r.inbox(t, ben, activity.TriageOpen)); len(got) != 1 || got[0].Seq == items[0].Seq {
		t.Fatalf("open inbox items = %+v, want only the newer request's", got)
	}
	_, err := r.service.Request(t.Context(), ada, obj, "again")
	var open *OpenError
	if !errors.As(err, &open) || !open.RequestedAt.Equal(second.CreatedAt) {
		t.Fatalf("third ask: err = %v, want OpenError at the newer request's %v", err, second.CreatedAt)
	}
	mine, err := r.service.Mine(t.Context(), ada)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]State{}
	for _, m := range mine {
		states[m.ID] = m.State
	}
	if states[first.ID] != StateDismissed || states[second.ID] != StateOpen {
		t.Fatalf("Ada's own states = %v, want the earlier dismissed and the newer open", states)
	}
}

func TestDismissingClosesTheRequestForEveryGranter(t *testing.T) {
	r := newRig(t)
	o := r.observation(t, "Checkout latency")
	second := auth.Principal{Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Ops", Email: "ops@example.com", Role: auth.RoleAdmin}
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name, role) VALUES ($1::uuid, 'https://idp', $1, $2, $3, 'admin')`, second.ID, second.Email, second.Name)
	r.ask(t, ada, Object{Kind: KindObservation, ID: o}, "")

	items := accessItems(r.inbox(t, root, activity.TriageOpen))
	if len(items) != 1 || len(accessItems(r.inbox(t, second, activity.TriageOpen))) != 1 {
		t.Fatal("both Admins should have the request")
	}
	if err := r.store.SetItemDone(t.Context(), second, items[0].Seq, true); err != nil {
		t.Fatal(err)
	}
	if got := accessItems(r.inbox(t, root, activity.TriageOpen)); len(got) != 0 {
		t.Fatalf("the other Admin still has %d items", len(got))
	}
}

func TestGivingCommentOnTheFolderGrantsTheRequestAndRecordsWhoDid(t *testing.T) {
	r := newRig(t)
	folder := r.folderBy(t, "Payments", ben)
	r.grant(t, folder, ada, "view")
	r.grant(t, folder, cleo, "view")
	d := r.dashboard(t, "Checkout", folder)
	obj := Object{Kind: KindDashboard, ID: d}
	adas := r.ask(t, ada, obj, "")
	r.ask(t, cleo, obj, "")

	r.grant(t, folder, ada, "comment")
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return r.service.SettleFolder(t.Context(), q, ben, folder)
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := r.open(t, ben); len(got) != 1 || got[0] == adas.ID {
		t.Fatalf("open after granting Ada = %v, want only Cleo's", got)
	}
	if got := accessItems(r.inbox(t, ben, activity.TriageOpen)); len(got) != 1 || got[0].Actor.Name != "Cleo" {
		t.Fatalf("Ben's items = %+v, want only Cleo's", got)
	}
	if n := r.count(t, `SELECT count(*) FROM activity WHERE type = 'access_request.granted' AND actor_id = $1::uuid AND target_id = $2::uuid AND data->'requested_by'->>'name' = 'Ada' AND data->>'title' = 'Checkout'`, ben.ID, adas.ID); n != 1 {
		t.Fatalf("granted events = %d, want 1", n)
	}
	if n := r.count(t, `SELECT count(*) FROM activity WHERE type = 'access_request.opened' AND data ? 'note'`); n != 0 {
		t.Fatalf("an opened event carries the note")
	}

	err = r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return r.service.SettleFolder(t.Context(), q, ben, folder)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := r.count(t, `SELECT count(*) FROM activity WHERE type = 'access_request.granted'`); n != 1 {
		t.Fatalf("granted events after a second settle = %d, want still 1", n)
	}
	if _, err := r.service.Request(t.Context(), auth.Principal{Kind: auth.KindUser, ID: ada.ID, Name: "Ada", Role: auth.RoleViewer}, obj, ""); !errors.Is(err, ErrCanComment) {
		t.Fatalf("Ada asks again after the grant: err = %v, want ErrCanComment", err)
	}
}

func TestARoleChangeGrantsTheRequestsOfThatPerson(t *testing.T) {
	r := newRig(t)
	d := r.dashboard(t, "Checkout", pgtype.UUID{})
	o := r.observation(t, "Checkout latency")
	r.ask(t, ada, Object{Kind: KindDashboard, ID: d}, "")
	r.ask(t, ada, Object{Kind: KindObservation, ID: o}, "")
	cleos := r.ask(t, cleo, Object{Kind: KindDashboard, ID: d}, "")

	r.db.Exec(t, `UPDATE users SET role = 'commenter' WHERE id = $1::uuid`, ada.ID)
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return ChangeLog{ChangeLog: activity.AuthChanges{}, Requests: r.service}.RoleChanged(t.Context(), q, root, auth.RoleChange{UserID: r.uuid(t, ada.ID), Name: "Ada", From: auth.RoleViewer, To: auth.RoleCommenter, Manual: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.open(t, root); !slices.Equal(got, []string{cleos.ID}) {
		t.Fatalf("open after Ada became a Commenter = %v, want only Cleo's", got)
	}
	if n := r.count(t, `SELECT count(*) FROM activity WHERE type = 'access_request.granted'`); n != 2 {
		t.Fatalf("granted events = %d, want one per request", n)
	}
}

func markedGeneral(t *testing.T, item activity.InboxItem) bool {
	t.Helper()
	var data struct {
		General bool `json:"general"`
	}
	if err := json.Unmarshal(item.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data.General
}

func TestARequestInGeneralSaysSoInItsRowAndItsListing(t *testing.T) {
	r := newRig(t)
	payments := r.folderBy(t, "Payments", ben)
	inGeneral := r.ask(t, ada, Object{Kind: KindDashboard, ID: r.dashboard(t, "Checkout", pgtype.UUID{})}, "")
	inFolder := r.ask(t, cleo, Object{Kind: KindDashboard, ID: r.dashboard(t, "Ledger", payments)}, "")
	if !inGeneral.Folder.General || inGeneral.Folder.Title != "General" || inFolder.Folder.General || inFolder.Folder.Title != "Payments" {
		t.Fatalf("folders = %+v and %+v", inGeneral.Folder, inFolder.Folder)
	}

	items := accessItems(r.inbox(t, root, activity.TriageOpen))
	if len(items) != 1 || !markedGeneral(t, items[0]) {
		t.Fatalf("the Admin's access items = %+v, want one marked general", items)
	}
	items = accessItems(r.inbox(t, ben, activity.TriageOpen))
	if len(items) != 1 || markedGeneral(t, items[0]) {
		t.Fatalf("the folder creator's access items = %+v, want one not marked general", items)
	}

	listed, err := r.service.List(t.Context(), root)
	if err != nil || len(listed) != 2 {
		t.Fatalf("the Admin lists %+v, %v", listed, err)
	}
	for _, item := range listed {
		if want := item.ID == inGeneral.ID; item.Folder.General != want {
			t.Errorf("listed %s with folder %+v", item.ID, item.Folder)
		}
	}
}
