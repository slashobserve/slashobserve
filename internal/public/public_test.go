package public

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

var (
	admin  = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Root", Role: auth.RoleAdmin}
	editor = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve", Role: auth.RoleEditor}
	viewer = auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Name: "Vic", Role: auth.RoleViewer}
	owner  = auth.Principal{Kind: auth.KindUser, ID: "77777777-7777-4777-8777-777777777777", Name: "Olive", Role: auth.RoleAdmin}
)

const plainSpec = `{"specVersion":"0","title":"Checkout","time":{"from":"now-1h","to":"now"},
"variables":[{"name":"env","type":"custom","options":[{"text":"prod","value":"prod"},{"text":"staging","value":"staging"}]}],
"panels":[{"id":"p1","type":"stat","title":"Requests","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"ref":"A","query":"sum(rate(http_requests_total{env=\"$env\"}[5m]))"}]}]}`

const userSpec = `{"specVersion":"0","title":"Mine","time":{"from":"now-1h","to":"now"},
"panels":[{"id":"p1","type":"stat","title":"Mine","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"ref":"A","query":"up{owner=\"${__user.login}\"}"}]}]}`

type rig struct {
	db      *dbtest.Database
	service *Service
	shared  pgtype.UUID
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger)
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(d.DB)
	dash := dashboards.New(d.DB, queryproxy.NewRunner(registry, logger), access, logger)
	base := &url.URL{Scheme: "https", Host: "observe.example"}
	r := rig{db: d, service: New(d.DB, access, dash, registry, base, logger)}
	r.shared = r.dashboard(t, plainSpec)
	return r
}

func (r rig) dashboard(t *testing.T, specText string) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ('T', 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	r.version(t, id, 1, specText)
	return id
}

func (r rig) version(t *testing.T, id pgtype.UUID, version int, specText string) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, parent_version, spec, summary, author_kind, author_id, author_name)
VALUES ($1, $2, CASE WHEN $2 = 1 THEN NULL ELSE $2 - 1 END, $3::jsonb, 'seed', 'user', gen_random_uuid(), 'seed')`, id, version, specText)
	r.db.Exec(t, `UPDATE dashboards SET latest_version = $2 WHERE id = $1`, id, version)
}

func (r rig) create(t *testing.T, in CreateInput) Created {
	t.Helper()
	created, err := r.service.Create(t.Context(), admin, r.shared, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return created
}

func TestCreateNeedsManage(t *testing.T) {
	r := newRig(t)
	hidden := r.dashboard(t, plainSpec)
	foldertest.Move(t, r.db, hidden, foldertest.Restricted(t, r.db, "Payroll", "payroll"))
	cases := []struct {
		name string
		by   auth.Principal
		id   pgtype.UUID
		want error
	}{
		{"an editor", editor, r.shared, folders.ErrForbidden},
		{"a viewer", viewer, r.shared, folders.ErrForbidden},
		{"a folder the caller cannot see", editor, hidden, folders.ErrNoAccess},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.service.Create(t.Context(), tc.by, tc.id, CreateInput{Mode: ModeFrozen})
			if !errors.Is(err, tc.want) {
				t.Fatalf("create: %v, want %v", err, tc.want)
			}
			if _, err := r.service.List(t.Context(), tc.by, tc.id); !errors.Is(err, tc.want) {
				t.Fatalf("list: %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := r.service.Create(t.Context(), admin, r.shared, CreateInput{Mode: ModeFrozen}); err != nil {
		t.Fatalf("an admin could not create a link: %v", err)
	}
}

func TestCreateRefusalNamesTheAccessItNeeds(t *testing.T) {
	r := newRig(t)
	_, err := r.service.Create(t.Context(), editor, r.shared, CreateInput{Mode: ModeFrozen})
	if want := forbidden.NeedsAccess("manage", "General"); !errors.Is(err, folders.ErrForbidden) || forbidden.BodyOf(err) != want {
		t.Fatalf("error %v answers %+v, want %+v", err, forbidden.BodyOf(err), want)
	}
}

func TestCreateRefusals(t *testing.T) {
	r := newRig(t)
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1, 'https://idp', 'olive', 'olive@example.com', 'Olive')`, owner.ID)
	personal := r.dashboard(t, plainSpec)
	r.db.Exec(t, `UPDATE dashboards SET folder_id = (SELECT id FROM folders WHERE personal AND owner_id = $2) WHERE id = $1`, personal, owner.ID)
	scoped := r.dashboard(t, userSpec)
	past := time.Now().Add(-time.Minute)
	cases := []struct {
		name string
		by   auth.Principal
		id   pgtype.UUID
		in   CreateInput
		want error
	}{
		{"a personal folder", owner, personal, CreateInput{Mode: ModeFrozen}, ErrPersonalFolder},
		{"a dashboard reading the viewer", admin, scoped, CreateInput{Mode: ModeFree}, ErrUserScoped},
		{"an expiry in the past", admin, r.shared, CreateInput{Mode: ModeFrozen, ExpiresAt: &past}, ErrInvalidExpiry},
		{"an unknown variable", admin, r.shared, CreateInput{Mode: ModeFrozen, Selections: map[string][]string{"nope": {"x"}}}, ErrUnknownVariable},
		{"an unknown mode", admin, r.shared, CreateInput{Mode: "pinned"}, ErrInvalidMode},
		{"a label that is too long", admin, r.shared, CreateInput{Mode: ModeFrozen, Label: strings.Repeat("x", maxLabelRunes+1)}, ErrInvalidLabel},
		{"a label with a control character", admin, r.shared, CreateInput{Mode: ModeFrozen, Label: "a\nb"}, ErrInvalidLabel},
		{"a label with a bidirectional override", admin, r.shared, CreateInput{Mode: ModeFrozen, Label: "board\u202eevoke"}, ErrInvalidLabel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.service.Create(t.Context(), tc.by, tc.id, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	for range maxLinksPerDashboard {
		r.create(t, CreateInput{Mode: ModeFrozen})
	}
	if _, err := r.service.Create(t.Context(), admin, r.shared, CreateInput{Mode: ModeFrozen}); !errors.Is(err, ErrTooManyLinks) {
		t.Fatalf("link %d: %v, want ErrTooManyLinks", maxLinksPerDashboard+1, err)
	}
}

func TestRacingCreatesCannotPassTheLinkLimit(t *testing.T) {
	r := newRig(t)
	for range maxLinksPerDashboard - 1 {
		r.create(t, CreateInput{Mode: ModeFrozen})
	}
	const racers = 8
	var wg sync.WaitGroup
	var created, refused atomic.Int32
	for range racers {
		wg.Go(func() {
			_, err := r.service.Create(t.Context(), admin, r.shared, CreateInput{Mode: ModeFrozen})
			switch {
			case err == nil:
				created.Add(1)
			case errors.Is(err, ErrTooManyLinks):
				refused.Add(1)
			default:
				t.Error(err)
			}
		})
	}
	wg.Wait()

	if created.Load() != 1 || refused.Load() != racers-1 {
		t.Fatalf("%d racing creates succeeded and %d were refused, want 1 and %d", created.Load(), refused.Load(), racers-1)
	}
}

type fakeOptions struct {
	values map[string][]string
	err    error
	calls  *int
}

func (f fakeOptions) VariableOptions(_ context.Context, _ pgtype.UUID, _ int, _ *spec.Dashboard, name string, _ map[string][]string) ([]string, error) {
	if f.calls != nil {
		*f.calls++
	}
	return f.values[name], f.err
}

func TestCreateAtTheCapRunsNoVariableQuery(t *testing.T) {
	r := newRig(t)
	for range maxLinksPerDashboard {
		r.create(t, CreateInput{Mode: ModeFrozen})
	}
	var calls int
	r.service.options = fakeOptions{values: map[string][]string{"env": {"prod"}}, calls: &calls}
	in := CreateInput{Mode: ModeFree, Selections: map[string][]string{"env": {"prod"}}}
	if _, err := r.service.Create(t.Context(), admin, r.shared, in); !errors.Is(err, ErrTooManyLinks) {
		t.Fatalf("err = %v, want ErrTooManyLinks", err)
	}
	if calls != 0 {
		t.Fatalf("the cap answer ran %d variable queries", calls)
	}
}

func TestCreateChecksFreeSelectionsAgainstTheVariableOptions(t *testing.T) {
	r := newRig(t)
	r.service.options = fakeOptions{values: map[string][]string{"env": {"prod", "staging"}}}
	cases := []struct {
		name string
		in   CreateInput
		want error
	}{
		{"an option", CreateInput{Mode: ModeFree, Selections: map[string][]string{"env": {"prod"}}}, nil},
		{"a value that is not an option", CreateInput{Mode: ModeFree, Selections: map[string][]string{"env": {"checkout"}}}, ErrSelectionNotAllowed},
		{"no selections", CreateInput{Mode: ModeFree}, nil},
		{"a frozen link keeps its stored value", CreateInput{Mode: ModeFrozen, Selections: map[string][]string{"env": {"checkout"}}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.service.Create(t.Context(), admin, r.shared, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	r.service.options = fakeOptions{err: errors.New("datasource down")}
	in := CreateInput{Mode: ModeFree, Selections: map[string][]string{"env": {"prod"}}}
	if _, err := r.service.Create(t.Context(), admin, r.shared, in); !errors.Is(err, ErrOptionsUnavailable) {
		t.Fatalf("err = %v, want ErrOptionsUnavailable", err)
	}
}

func TestCreatedLinkResolvesUntilRevoked(t *testing.T) {
	r := newRig(t)
	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	created := r.create(t, CreateInput{Mode: ModeFree, Selections: map[string][]string{"env": {"staging"}}, ExpiresAt: &expires})
	if created.URL != "https://observe.example/p/"+created.Token {
		t.Fatalf("url %q", created.URL)
	}
	if !strings.HasPrefix(created.Token, created.Link.Prefix) || len(created.Link.Prefix) != 12 {
		t.Fatalf("prefix %q of %q", created.Link.Prefix, created.Token)
	}
	var stored int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM public_links WHERE token_hash = $1 AND position($2 in display_prefix || selections::text || created_by_name) = 0`, opaquetoken.Hash(created.Token), created.Token).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("the row is not stored by hash alone: %d, %v", stored, err)
	}
	resolved, err := r.service.Resolve(t.Context(), created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.DashboardID != r.shared || resolved.Version != 1 || resolved.Mode != ModeFree || !reflect.DeepEqual(resolved.Selections, map[string][]string{"env": {"staging"}}) || !resolved.ExpiresAt.Equal(expires) {
		t.Fatalf("resolved %+v", resolved)
	}
	r.version(t, r.shared, 2, plainSpec)
	if resolved, err = r.service.Resolve(t.Context(), created.Token); err != nil || resolved.Version != 2 {
		t.Fatalf("the link does not follow the latest version: %+v, %v", resolved, err)
	}
	list, err := r.service.List(t.Context(), admin, r.shared)
	if err != nil || len(list) != 1 || !list[0].Usable || list[0].Expired || list[0].CreatedBy.Name != "Root" {
		t.Fatalf("list %+v, %v", list, err)
	}
	linkID, _ := db.ParseUUID(created.Link.ID)
	if err := r.service.Delete(t.Context(), editor, r.shared, linkID); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("an editor revoked a link: %v", err)
	}
	if err := r.service.Delete(t.Context(), admin, r.shared, linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.Resolve(t.Context(), created.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a revoked link resolved: %v", err)
	}
	if err := r.service.Delete(t.Context(), admin, r.shared, linkID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a second revoke: %v", err)
	}
}

func TestResolveRefusals(t *testing.T) {
	r := newRig(t)
	expires := time.Now().Add(time.Hour)
	created := r.create(t, CreateInput{Mode: ModeFrozen, ExpiresAt: &expires})
	for _, token := range []string{"", "pub_short", created.Token + "x", "pub_" + strings.Repeat("A", 43)} {
		if _, err := r.service.Resolve(t.Context(), token); !errors.Is(err, ErrNotFound) {
			t.Fatalf("token %q: %v", token, err)
		}
	}
	r.service.now = func() time.Time { return expires.Add(time.Second) }
	if _, err := r.service.Resolve(t.Context(), created.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an expired link resolved: %v", err)
	}
	list, err := r.service.List(t.Context(), admin, r.shared)
	if err != nil || len(list) != 1 || list[0].Usable || !list[0].Expired {
		t.Fatalf("list %+v, %v", list, err)
	}
}

func TestViewServesTheLatestSpecWithoutQueryText(t *testing.T) {
	r := newRig(t)
	created := r.create(t, CreateInput{Mode: ModeFrozen, Selections: map[string][]string{"env": {"prod"}}})
	v, err := r.service.View(t.Context(), created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != "Checkout" || v.Spec.Panels[0].Queries[0].Query != "" || v.Spec.Panels[0].Queries[0].Ref != "A" || v.Spec.Panels[0].Datasource != "prom" {
		t.Fatalf("view %+v", v.Spec.Panels[0])
	}
	var viewed bool
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT last_viewed_at IS NOT NULL FROM public_links`).Scan(&viewed); err != nil || !viewed {
		t.Fatalf("the view was not recorded: %v", err)
	}
	r.version(t, r.shared, 2, userSpec)
	if _, err := r.service.View(t.Context(), created.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a link to a user-scoped version opened: %v", err)
	}
	list, err := r.service.List(t.Context(), admin, r.shared)
	if err != nil || len(list) != 1 || list[0].Usable || list[0].Expired {
		t.Fatalf("list %+v, %v", list, err)
	}
}

func TestALabelIsKeptTrimmedAndListed(t *testing.T) {
	r := newRig(t)
	created := r.create(t, CreateInput{Mode: ModeFrozen, Label: "  For the board  "})
	if created.Link.Label != "For the board" {
		t.Fatalf("created label %q", created.Link.Label)
	}
	list, err := r.service.List(t.Context(), admin, r.shared)
	if err != nil || len(list) != 1 || list[0].Label != "For the board" {
		t.Fatalf("list %+v, %v", list, err)
	}
}

func TestExpiredLinksDoNotCountTowardTheCap(t *testing.T) {
	r := newRig(t)
	expires := time.Now().Add(time.Hour)
	r.create(t, CreateInput{Mode: ModeFrozen, ExpiresAt: &expires})
	for range maxLinksPerDashboard - 1 {
		r.create(t, CreateInput{Mode: ModeFrozen})
	}
	if _, err := r.service.Create(t.Context(), admin, r.shared, CreateInput{Mode: ModeFrozen}); !errors.Is(err, ErrTooManyLinks) {
		t.Fatalf("with every link live: %v, want ErrTooManyLinks", err)
	}
	r.service.now = func() time.Time { return expires.Add(time.Second) }
	if _, err := r.service.Create(t.Context(), admin, r.shared, CreateInput{Mode: ModeFrozen}); err != nil {
		t.Fatalf("after one link expired: %v", err)
	}
}

func TestAnUnexpiredLinkStaysListedBehindManyExpiredOnes(t *testing.T) {
	r := newRig(t)
	permanent := r.create(t, CreateInput{Mode: ModeFrozen})
	clock := time.Now()
	r.service.now = func() time.Time { return clock }
	for range 105 {
		expires := clock.Add(time.Hour)
		r.create(t, CreateInput{Mode: ModeFrozen, ExpiresAt: &expires})
		clock = clock.Add(2 * time.Hour)
	}
	list, err := r.service.List(t.Context(), admin, r.shared)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 || list[0].ID != permanent.Link.ID || list[0].Expired {
		t.Fatalf("the live link is not first in the list of %d", len(list))
	}
}

func TestRefuseAnswersAForbiddenManagerCallWithItsNeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	(&Handler{}).refuse(c, forbidden.Need(folders.ErrForbidden, forbidden.NeedsAccess("manage", "QA")))
	want := `{"error":"your role does not allow this","code":"forbidden_role","needed_access":"manage","folder_title":"QA"}`
	if rec.Code != http.StatusForbidden || rec.Body.String() != want {
		t.Fatalf("status %d body %s, want %s", rec.Code, rec.Body.String(), want)
	}
}
