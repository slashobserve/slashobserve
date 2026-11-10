package annotations

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const payrollGroup = "payroll-team"

var (
	editor      = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve", Role: auth.RoleEditor}
	otherEditor = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Ed", Role: auth.RoleEditor}
	viewer      = auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Name: "Vic", Role: auth.RoleViewer}
	payroll     = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Pat", Role: auth.RoleViewer, Groups: []string{payrollGroup}}
	admin       = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Root", Role: auth.RoleAdmin}
	deployKey   = auth.Principal{Kind: auth.KindServiceAccount, ID: "66666666-6666-4666-8666-666666666666", Name: "ci", Role: auth.RoleEditor}
)

var base = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type rig struct {
	db      *dbtest.Database
	service *Service
	open    pgtype.UUID
	hidden  pgtype.UUID
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	r := rig{db: d, service: New(d.DB, folders.NewAccess(d.DB))}
	r.open = r.dashboard(t, "Checkout")
	r.hidden = r.dashboard(t, "Payroll")
	foldertest.Move(t, d, r.hidden, foldertest.Restricted(t, d, "Payroll", payrollGroup))
	return r
}

func (r rig) dashboard(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ($1, 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, title).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r rig) create(t *testing.T, by auth.Principal, in CreateInput) Annotation {
	t.Helper()
	a, err := r.service.Create(t.Context(), by, in)
	if err != nil {
		t.Fatalf("create %q: %v", in.Text, err)
	}
	return a
}

func (r rig) list(t *testing.T, by auth.Principal, q ListQuery) Page {
	t.Helper()
	if q.Limit == 0 {
		q.Limit = 100
	}
	page, err := r.service.List(t.Context(), by, q)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return page
}

func texts(page Page) []string {
	out := make([]string, 0, len(page.Annotations))
	for _, a := range page.Annotations {
		out = append(out, a.Text)
	}
	return out
}

func at(minutes int) time.Time {
	return base.Add(time.Duration(minutes) * time.Minute)
}

func TestCreateKeepsWhatWasSent(t *testing.T) {
	r := newRig(t)
	to := at(5)
	a := r.create(t, editor, CreateInput{DashboardID: r.open, PanelID: "latency", From: at(0), To: &to, Text: "  canary at 10%  ", Tags: []string{"deploy", "checkout", "deploy"}})
	if a.DashboardID != r.open.String() || a.PanelID != "latency" || !a.From.Equal(at(0)) || a.To == nil || !a.To.Equal(to) {
		t.Errorf("created %+v", a)
	}
	if a.Text != "canary at 10%" || !slices.Equal(a.Tags, []string{"deploy", "checkout"}) || a.CreatedBy != editor.ID {
		t.Errorf("text %q, tags %v, created_by %q", a.Text, a.Tags, a.CreatedBy)
	}
}

func TestCreateWithoutTimeUsesNow(t *testing.T) {
	r := newRig(t)
	before := time.Now().Add(-time.Second)
	a := r.create(t, deployKey, CreateInput{Text: "deploy v1.2.3"})
	if a.From.Before(before) || a.From.After(time.Now().Add(time.Second)) {
		t.Errorf("time_from %v, want about now", a.From)
	}
	if a.DashboardID != "" || a.CreatedBy != deployKey.ID {
		t.Errorf("created %+v, want a global annotation by the key", a)
	}
}

func TestListNamesTheUserOrServiceAccountThatAddedIt(t *testing.T) {
	r := newRig(t)
	sql := r.db.SQLDB(t)
	if _, err := sql.ExecContext(t.Context(), `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1, 'https://idp', 'eve', 'eve@example.com', 'Eve Adams'), ($2, 'https://idp', 'ed', 'ed@example.com', '')`, editor.ID, otherEditor.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sql.ExecContext(t.Context(), `INSERT INTO service_accounts (id, name) VALUES ($1, 'ci-deploy')`, deployKey.ID); err != nil {
		t.Fatal(err)
	}
	r.create(t, editor, CreateInput{From: at(0), Text: "by a user"})
	r.create(t, otherEditor, CreateInput{From: at(1), Text: "by a user without a name"})
	r.create(t, deployKey, CreateInput{From: at(2), Text: "by a key"})
	r.create(t, admin, CreateInput{From: at(3), Text: "by someone gone"})

	names := map[string]string{}
	for _, a := range r.list(t, viewer, ListQuery{}).Annotations {
		names[a.Text] = a.CreatedByName
	}
	want := map[string]string{"by a user": "Eve Adams", "by a user without a name": "ed@example.com", "by a key": "ci-deploy", "by someone gone": ""}
	for text, name := range want {
		if names[text] != name {
			t.Errorf("%q created_by_name %q, want %q", text, names[text], name)
		}
	}
}

func TestAnnotationRecordsWhoAddedItAndThroughWhat(t *testing.T) {
	r := newRig(t)
	sql := r.db.SQLDB(t)
	if _, err := sql.ExecContext(t.Context(), `INSERT INTO service_accounts (id, name) VALUES ($1, $2)`, deployKey.ID, deployKey.Name); err != nil {
		t.Fatal(err)
	}
	over := origin.With(t.Context(), origin.Origin{Kind: origin.MCP, Client: "claude-code"})
	created, err := r.service.Create(over, deployKey, CreateInput{From: at(0), Text: "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	var kind, kindOrigin, client string
	if err := sql.QueryRowContext(t.Context(), `SELECT created_by_kind, origin, origin_client FROM annotations WHERE id = $1::uuid`, created.ID).Scan(&kind, &kindOrigin, &client); err != nil {
		t.Fatal(err)
	}
	if kind != "service_account" || kindOrigin != "mcp" || client != "claude-code" {
		t.Fatalf("stored kind %q, origin %q, client %q", kind, kindOrigin, client)
	}
	for name, a := range map[string]Annotation{"created": created, "listed": r.list(t, viewer, ListQuery{}).Annotations[0]} {
		who, ok := a.Actor()
		if !ok || who.Label() != "ci (via Claude Code)" {
			t.Errorf("%s: actor %+v (present %v), want the label ci (via Claude Code)", name, who, ok)
		}
	}
}

func TestAnnotationOfAGoneAccountHasNoActor(t *testing.T) {
	r := newRig(t)
	r.create(t, deployKey, CreateInput{From: at(0), Text: "deploy"})
	got := r.list(t, viewer, ListQuery{}).Annotations[0]
	if who, ok := got.Actor(); ok {
		t.Fatalf("actor %+v for an account that is not in service_accounts, want none", who)
	}
}

func TestCreateAnswersTheCallersName(t *testing.T) {
	r := newRig(t)
	if a := r.create(t, deployKey, CreateInput{Text: "deploy"}); a.CreatedByName != deployKey.Name {
		t.Errorf("created_by_name %q, want %q", a.CreatedByName, deployKey.Name)
	}
}

func TestViewerSeesGlobalAndDashboardAnnotationsOnTheDashboard(t *testing.T) {
	r := newRig(t)
	r.create(t, editor, CreateInput{From: at(0), Text: "global deploy"})
	r.create(t, editor, CreateInput{DashboardID: r.open, From: at(10), Text: "checkout note"})
	other := r.dashboard(t, "Search")
	r.create(t, editor, CreateInput{DashboardID: other, From: at(20), Text: "search note"})

	if got := texts(r.list(t, viewer, ListQuery{DashboardID: r.open})); !slices.Equal(got, []string{"checkout note", "global deploy"}) {
		t.Errorf("dashboard list %v, want the dashboard's note and the global one, newest first", got)
	}
	if got := texts(r.list(t, viewer, ListQuery{})); !slices.Equal(got, []string{"global deploy"}) {
		t.Errorf("list without a dashboard %v, want only global annotations", got)
	}
}

func TestDashboardTheCallerCannotSeeIsNotFound(t *testing.T) {
	r := newRig(t)
	r.create(t, admin, CreateInput{DashboardID: r.hidden, From: at(0), Text: "payroll run"})

	for _, by := range []auth.Principal{viewer, editor} {
		if _, err := r.service.List(t.Context(), by, ListQuery{DashboardID: r.hidden, Limit: 10}); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s lists the hidden dashboard: error %v, want ErrNotFound", by.Name, err)
		}
	}
	if got := texts(r.list(t, payroll, ListQuery{DashboardID: r.hidden})); !slices.Equal(got, []string{"payroll run"}) {
		t.Errorf("payroll group sees %v", got)
	}
}

func TestCreateRefusals(t *testing.T) {
	r := newRig(t)
	missing := pgtype.UUID{Bytes: [16]byte{15: 9}, Valid: true}
	cases := []struct {
		name string
		by   auth.Principal
		in   CreateInput
		want error
	}{
		{"viewer creates a global annotation", viewer, CreateInput{Text: "x"}, ErrForbidden},
		{"viewer annotates a dashboard they only view", viewer, CreateInput{DashboardID: r.open, Text: "x"}, ErrForbidden},
		{"group viewer annotates a dashboard they only view", payroll, CreateInput{DashboardID: r.hidden, Text: "x"}, ErrForbidden},
		{"editor annotates a hidden dashboard", editor, CreateInput{DashboardID: r.hidden, Text: "x"}, ErrNotFound},
		{"editor annotates a missing dashboard", editor, CreateInput{DashboardID: missing, Text: "x"}, ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.service.Create(t.Context(), tc.by, tc.in); !errors.Is(err, tc.want) {
				t.Errorf("error %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	r := newRig(t)
	early := at(-1)
	beforeEpoch := time.Date(1969, 12, 31, 0, 0, 0, 0, time.UTC)
	elevenTags := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	cases := []struct {
		name string
		in   CreateInput
		code string
	}{
		{"empty text", CreateInput{Text: "   "}, "invalid_text"},
		{"text over 2000 characters", CreateInput{Text: strings.Repeat("é", MaxTextRunes+1)}, "invalid_text"},
		{"control character in text", CreateInput{Text: "a\x07b"}, "invalid_text"},
		{"uppercase tag", CreateInput{Text: "x", Tags: []string{"Deploy"}}, "invalid_tags"},
		{"tag with a comma", CreateInput{Text: "x", Tags: []string{"a,b"}}, "invalid_tags"},
		{"eleven tags", CreateInput{Text: "x", Tags: elevenTags}, "invalid_tags"},
		{"panel without a dashboard", CreateInput{Text: "x", PanelID: "latency"}, "invalid_panel_id"},
		{"panel id over 64 characters", CreateInput{DashboardID: r.open, Text: "x", PanelID: strings.Repeat("p", 65)}, "invalid_panel_id"},
		{"end before start", CreateInput{Text: "x", From: at(0), To: &early}, "invalid_time"},
		{"start before 1970", CreateInput{Text: "x", From: beforeEpoch}, "invalid_time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.service.Create(t.Context(), editor, tc.in)
			var invalid *InvalidError
			if !errors.As(err, &invalid) || invalid.Code != tc.code {
				t.Errorf("error %v, want %s", err, tc.code)
			}
		})
	}
	tenTags := elevenTags[:MaxTags]
	maxText := strings.Repeat("é", MaxTextRunes)
	r.create(t, editor, CreateInput{Text: maxText, Tags: tenTags})
}

func TestDeleteIsForTheCreatorOrAnAdmin(t *testing.T) {
	r := newRig(t)
	mine := r.create(t, editor, CreateInput{DashboardID: r.open, From: at(0), Text: "mine"})
	global := r.create(t, editor, CreateInput{From: at(1), Text: "global"})
	hidden := r.create(t, admin, CreateInput{DashboardID: r.hidden, From: at(2), Text: "hidden"})

	cases := []struct {
		name string
		by   auth.Principal
		id   string
		want error
	}{
		{"another editor", otherEditor, mine.ID, ErrNotCreator},
		{"a viewer", viewer, global.ID, ErrNotCreator},
		{"someone who cannot see the dashboard", editor, hidden.ID, ErrNotFound},
		{"the creator", editor, mine.ID, nil},
		{"the creator again", editor, mine.ID, ErrNotFound},
		{"an admin", admin, global.ID, nil},
		{"an admin on a hidden dashboard", admin, hidden.ID, nil},
	}
	for _, tc := range cases {
		id, err := db.ParseUUID(tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.service.Delete(t.Context(), tc.by, id); !errors.Is(err, tc.want) {
			t.Errorf("%s: error %v, want %v", tc.name, err, tc.want)
		}
	}
	if got := r.list(t, admin, ListQuery{DashboardID: r.open}); len(got.Annotations) != 0 {
		t.Errorf("annotations left: %v", texts(got))
	}
}

func TestTagFilterMatchesAnyTag(t *testing.T) {
	r := newRig(t)
	r.create(t, editor, CreateInput{From: at(0), Text: "deploy", Tags: []string{"deploy", "checkout"}})
	r.create(t, editor, CreateInput{From: at(1), Text: "incident", Tags: []string{"incident"}})
	r.create(t, editor, CreateInput{From: at(2), Text: "untagged"})

	cases := []struct {
		tags []string
		want []string
	}{
		{nil, []string{"untagged", "incident", "deploy"}},
		{[]string{"checkout"}, []string{"deploy"}},
		{[]string{"incident", "deploy"}, []string{"incident", "deploy"}},
		{[]string{"nothing"}, []string{}},
	}
	for _, tc := range cases {
		if got := texts(r.list(t, viewer, ListQuery{Tags: tc.tags})); !slices.Equal(got, tc.want) {
			t.Errorf("tags %v: %v, want %v", tc.tags, got, tc.want)
		}
	}
}

func TestTimeWindowIncludesOverlappingRanges(t *testing.T) {
	r := newRig(t)
	rangeEnd := at(30)
	r.create(t, editor, CreateInput{From: at(0), Text: "before"})
	r.create(t, editor, CreateInput{From: at(5), To: &rangeEnd, Text: "maintenance"})
	r.create(t, editor, CreateInput{From: at(20), Text: "inside"})
	r.create(t, editor, CreateInput{From: at(40), Text: "after"})

	from, to := at(10), at(25)
	if got := texts(r.list(t, viewer, ListQuery{From: &from, To: &to})); !slices.Equal(got, []string{"inside", "maintenance"}) {
		t.Errorf("window: %v", got)
	}
	edge := at(20)
	if got := texts(r.list(t, viewer, ListQuery{From: &edge, To: &edge})); !slices.Equal(got, []string{"inside", "maintenance"}) {
		t.Errorf("a window of one instant: %v", got)
	}
}

func TestCursorPagesThroughAnnotationsAtTheSameTime(t *testing.T) {
	r := newRig(t)
	for _, text := range []string{"a", "b", "c"} {
		r.create(t, editor, CreateInput{From: at(0), Text: text})
	}
	r.create(t, editor, CreateInput{From: at(-10), Text: "older"})

	var seen []string
	ended := false
	q := ListQuery{Limit: 2}
	for range 4 {
		page := r.list(t, viewer, q)
		seen = append(seen, texts(page)...)
		if page.Next == nil {
			ended = true
			break
		}
		q.After = page.Next
	}
	if !ended {
		t.Fatal("the pages never ended")
	}
	if len(seen) != 4 || seen[3] != "older" {
		t.Fatalf("pages %v, want 4 annotations ending with the older one", seen)
	}
	sorted := slices.Clone(seen[:3])
	slices.Sort(sorted)
	if !slices.Equal(sorted, []string{"a", "b", "c"}) {
		t.Errorf("same-time annotations %v, want each once", seen[:3])
	}
}

func TestDeletingTheDashboardDeletesItsAnnotations(t *testing.T) {
	r := newRig(t)
	r.create(t, editor, CreateInput{DashboardID: r.open, From: at(0), Text: "gone"})
	r.create(t, editor, CreateInput{From: at(1), Text: "kept"})
	r.db.Exec(t, `DELETE FROM dashboards WHERE id = $1`, r.open)
	if got := texts(r.list(t, viewer, ListQuery{})); !slices.Equal(got, []string{"kept"}) {
		t.Errorf("after the delete: %v", got)
	}
	if _, err := r.service.Create(t.Context(), editor, CreateInput{DashboardID: r.open, Text: "late"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("create on a deleted dashboard: %v, want ErrNotFound", err)
	}
}
