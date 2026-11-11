package public

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const frozenData = `{"frames":[],"meta":{"kind":"range","from":"2026-11-01T07:00:00Z","to":"2026-11-01T09:00:00Z","step_ms":15000,"queries":[{"ref":"A","kind":"range","step_ms":15000}],"cached":false,"executed_at":"2026-11-01T09:00:01Z","truncated":false,"partial":false,"rows":0}}`

const panelBlock = `{"panel":{"type":"timeseries","title":"Rate","datasource":"prom","queries":[{"ref":"A","query":"sum(rate(secret_metric_total[5m]))"}]}}`

type scopedFreezer struct {
	userScoped bool
}

func (f *scopedFreezer) Freeze(context.Context, auth.Principal, *spec.Dashboard, string, time.Time, time.Time) (observations.Frozen, error) {
	return observations.Frozen{Data: json.RawMessage(frozenData), UserScoped: f.userScoped}, nil
}

type rcaRig struct {
	rig
	observations *observations.Service
	freezer      *scopedFreezer
	folder       pgtype.UUID
	router       routers.Router
}

func newRCARig(t *testing.T) rcaRig {
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
	freezer := &scopedFreezer{}
	obs := observations.New(d.DB, access, freezer, base, logger)
	for _, p := range []auth.Principal{admin, editor, viewer, owner} {
		d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $2 || '@example.com', $2)`, p.ID, p.Name)
	}
	r := rcaRig{rig: rig{db: d, service: New(d.DB, access, dash, registry, base, logger, WithRCAs(obs))}, observations: obs, freezer: freezer}
	r.shared = r.dashboard(t, plainSpec)
	var raw string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title) VALUES ('Incidents') RETURNING id::text`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if r.folder, err = db.ParseUUID(raw); err != nil {
		t.Fatal(err)
	}
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.router, err = legacy.NewRouter(doc); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r rcaRig) observation(t *testing.T, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	o, err := r.observations.Create(t.Context(), admin, observations.CreateInput{FolderID: folder, Title: "Checkout 5xx", From: ptr(time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)), To: ptr(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC))})
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(o.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []observations.BlockInput{
		{Kind: observations.KindText, Body: json.RawMessage(`{"markdown":"Started after the deploy"}`)},
		{Kind: observations.KindPanel, Body: json.RawMessage(panelBlock)},
	} {
		if _, err := r.observations.AddBlock(t.Context(), admin, id, in); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (r rcaRig) published(t *testing.T) pgtype.UUID {
	t.Helper()
	id := r.observation(t, r.folder)
	if _, err := r.observations.Publish(t.Context(), admin, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (r rcaRig) createLink(t *testing.T, id pgtype.UUID, revision int, in CreateRCAInput) CreatedRCA {
	t.Helper()
	created, err := r.service.CreateRCALink(t.Context(), admin, id, revision, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return created
}

func ptr[T any](v T) *T {
	return &v
}

func TestRCALinkCreateListAndRevoke(t *testing.T) {
	r := newRCARig(t)
	id := r.published(t)
	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	created := r.createLink(t, id, 1, CreateRCAInput{ExpiresAt: &expires})

	if created.URL != "https://observe.example/rca/"+created.Token {
		t.Errorf("url %q", created.URL)
	}
	if !strings.HasPrefix(created.Token, "pub_") || !strings.HasPrefix(created.Token, created.Link.Prefix) || len(created.Link.Prefix) != 12 {
		t.Errorf("token %q prefix %q", created.Token, created.Link.Prefix)
	}
	var stored int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM public_links WHERE token_hash = $1 AND dashboard_id IS NULL AND variables = 'frozen' AND selections = '{}'::jsonb`, opaquetoken.Hash(created.Token)).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("the row is not stored by hash and revision alone: %d, %v", stored, err)
	}
	list, err := r.service.ListRCALinks(t.Context(), admin, id, 1)
	if err != nil || len(list) != 1 || list[0].Revision != 1 || list[0].Expired || list[0].CreatedBy.Name != "Root" || !list[0].ExpiresAt.Equal(expires) {
		t.Fatalf("list %+v, %v", list, err)
	}
	linkID, _ := db.ParseUUID(created.Link.ID)
	if err := r.service.DeleteRCALink(t.Context(), editor, id, linkID); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("an editor revoked a link: %v", err)
	}
	if err := r.service.DeleteRCALink(t.Context(), admin, id, linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.ViewRCA(t.Context(), created.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a revoked link opened: %v", err)
	}
	if err := r.service.DeleteRCALink(t.Context(), admin, id, linkID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a second revoke: %v", err)
	}
	if err := r.service.DeleteRCALink(t.Context(), admin, r.observation(t, r.folder), linkID); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoking through another observation: %v", err)
	}
}

func TestRCALinkRefusals(t *testing.T) {
	r := newRCARig(t)
	id := r.published(t)
	hidden := r.observation(t, foldertest.Restricted(t, r.db, "Payroll", "payroll"))
	personal := r.published(t)
	r.db.Exec(t, `UPDATE observations SET folder_id = (SELECT id FROM folders WHERE personal AND owner_id = $2::uuid) WHERE id = $1`, personal, owner.ID)
	past := time.Now().Add(-time.Minute)
	cases := []struct {
		name     string
		by       auth.Principal
		id       pgtype.UUID
		revision int
		in       CreateRCAInput
		want     error
	}{
		{"an editor", editor, id, 1, CreateRCAInput{}, folders.ErrForbidden},
		{"a viewer", viewer, id, 1, CreateRCAInput{}, folders.ErrForbidden},
		{"a folder the caller cannot see", editor, hidden, 1, CreateRCAInput{}, observations.ErrNotFound},
		{"a personal folder", owner, personal, 1, CreateRCAInput{}, ErrRCAPersonalFolder},
		{"a revision that does not exist", admin, id, 2, CreateRCAInput{}, observations.ErrRevisionNotFound},
		{"an expiry in the past", admin, id, 1, CreateRCAInput{ExpiresAt: &past}, ErrInvalidExpiry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.service.CreateRCALink(t.Context(), tc.by, tc.id, tc.revision, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("create: %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := r.service.ListRCALinks(t.Context(), viewer, id, 1); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer listed links: %v", err)
	}
}

func TestARevisionWithAUserScopedSnapshotHasNoLink(t *testing.T) {
	r := newRCARig(t)
	r.freezer.userScoped = true
	id := r.published(t)
	_, err := r.service.CreateRCALink(t.Context(), admin, id, 1, CreateRCAInput{})
	if !errors.Is(err, ErrRCAUserScoped) || !strings.Contains(err.Error(), "a snapshot in this RCA shows data that depends on who is viewing") {
		t.Fatalf("create: %v", err)
	}
	r.freezer.userScoped = false
	if _, err := r.observations.Publish(t.Context(), admin, id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.CreateRCALink(t.Context(), admin, id, 2, CreateRCAInput{}); err != nil {
		t.Errorf("a later revision with no scoped snapshot: %v", err)
	}
}

func TestAnObservationHasAtMostTwentyLinksAcrossItsRevisions(t *testing.T) {
	r := newRCARig(t)
	id := r.published(t)
	if _, err := r.observations.Publish(t.Context(), admin, id); err != nil {
		t.Fatal(err)
	}
	for i := range maxLinksPerObservation {
		r.createLink(t, id, 1+i%2, CreateRCAInput{})
	}
	for _, revision := range []int{1, 2} {
		if _, err := r.service.CreateRCALink(t.Context(), admin, id, revision, CreateRCAInput{}); !errors.Is(err, ErrTooManyRCALinks) {
			t.Fatalf("link %d on revision %d: %v", maxLinksPerObservation+1, revision, err)
		}
	}
	if !strings.Contains(ErrTooManyRCALinks.Error(), "an observation has at most 20 public links; revoke one first") {
		t.Errorf("wording %q", ErrTooManyRCALinks)
	}
}

func TestAnRCATokenOpensOnlyTheRCAViewsUntilItExpires(t *testing.T) {
	r := newRCARig(t)
	id := r.published(t)
	expires := time.Now().Add(time.Hour)
	created := r.createLink(t, id, 1, CreateRCAInput{ExpiresAt: &expires})
	dashboardLink := r.create(t, CreateInput{Mode: ModeFrozen})

	view, err := r.service.ViewRCA(t.Context(), created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if view.Revision != 1 || view.Title != "Checkout 5xx" || view.PublishedBy != "Root" || view.State != "open" || view.Summary != "" || view.From == nil {
		t.Errorf("view %+v", view)
	}
	var blocks []struct {
		Kind       string `json:"kind"`
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.Unmarshal(view.Blocks, &blocks); err != nil || len(blocks) != 2 || blocks[0].Kind != "text" || blocks[1].Kind != "snapshot" {
		t.Fatalf("blocks %s, %v", view.Blocks, err)
	}
	var viewed bool
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT last_viewed_at IS NOT NULL FROM public_links WHERE rca_id IS NOT NULL`).Scan(&viewed); err != nil || !viewed {
		t.Errorf("the view was not recorded: %v", err)
	}
	snapshotID, _ := db.ParseUUID(blocks[1].SnapshotID)
	if frames, err := r.service.ViewRCASnapshot(t.Context(), created.Token, snapshotID); err != nil || !sameJSON(t, frames, json.RawMessage(frozenData)) {
		t.Errorf("frames %s, %v", frames, err)
	}

	if _, err := r.service.View(t.Context(), created.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("an RCA token opened the dashboard view: %v", err)
	}
	if _, err := r.service.Resolve(t.Context(), created.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("an RCA token resolved as a dashboard link: %v", err)
	}
	if _, err := r.service.ViewRCA(t.Context(), dashboardLink.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a dashboard token opened the RCA view: %v", err)
	}
	if _, err := r.service.ViewRCASnapshot(t.Context(), dashboardLink.Token, snapshotID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a dashboard token opened an RCA snapshot: %v", err)
	}
	for _, token := range []string{"", "pub_short", created.Token + "x", "pub_" + strings.Repeat("A", 43)} {
		if _, err := r.service.ViewRCA(t.Context(), token); !errors.Is(err, ErrNotFound) {
			t.Errorf("token %q: %v", token, err)
		}
	}

	r.service.now = func() time.Time { return expires.Add(time.Second) }
	if _, err := r.service.ViewRCA(t.Context(), created.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("an expired link opened the RCA: %v", err)
	}
	if _, err := r.service.ViewRCASnapshot(t.Context(), created.Token, snapshotID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an expired link opened a snapshot: %v", err)
	}
	list, err := r.service.ListRCALinks(t.Context(), admin, id, 1)
	if err != nil || len(list) != 1 || !list[0].Expired {
		t.Errorf("list %+v, %v", list, err)
	}
}

func sameJSON(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(x, y)
}

func TestAnRCALinkShowsOnlyItsOwnRevisionsSnapshots(t *testing.T) {
	r := newRCARig(t)
	id := r.published(t)
	if _, err := r.observations.Publish(t.Context(), admin, id); err != nil {
		t.Fatal(err)
	}
	first := r.createLink(t, id, 1, CreateRCAInput{})
	second, err := r.observations.GetRCA(t.Context(), admin, id, 2)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := db.ParseUUID(second.Blocks[1].SnapshotID)
	if _, err := r.service.ViewRCASnapshot(t.Context(), first.Token, other); !errors.Is(err, ErrNotFound) {
		t.Errorf("a snapshot of revision 2 through a revision 1 link: %v", err)
	}
	if _, err := r.service.ViewRCASnapshot(t.Context(), first.Token, pgtype.UUID{Bytes: [16]byte{15: 1}, Valid: true}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown snapshot: %v", err)
	}
}

func TestExistingDashboardLinksKeepWorking(t *testing.T) {
	r := newRCARig(t)
	r.published(t)
	created := r.create(t, CreateInput{Mode: ModeFrozen})
	list, err := r.service.List(t.Context(), admin, r.shared)
	if err != nil || len(list) != 1 || list[0].DashboardID != r.shared.String() {
		t.Fatalf("list %+v, %v", list, err)
	}
	if _, err := r.service.View(t.Context(), created.Token); err != nil {
		t.Errorf("dashboard view: %v", err)
	}
}

type rcaHTTP struct {
	rcaRig
	engine *gin.Engine
	who    auth.Principal
}

func newRCAHTTP(t *testing.T) *rcaHTTP {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &rcaHTTP{rcaRig: newRCARig(t), engine: gin.New(), who: admin}
	handler := NewHandler(h.service, folders.NewAccess(h.db.DB), slog.New(slog.DiscardHandler))
	handler.Register(auth.NewRoutes(h.engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), h.who))
	})))
	handler.RegisterPublic(h.engine.Group("/api/v1/public"))
	return h
}

func (h *rcaHTTP) send(t *testing.T, by auth.Principal, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	h.who = by
	request := func() *http.Request {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		return r
	}
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, request())
	if rec.Code != want {
		t.Fatalf("%s %s answered %d, want %d: %s", method, path, rec.Code, want, rec.Body)
	}
	check := request()
	route, params, err := h.router.FindRoute(check)
	if err != nil {
		t.Fatalf("%s %s is not documented: %v", method, path, err)
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, SkipSettingDefaults: true}
	input := &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options}
	if err := openapi3filter.ValidateRequest(t.Context(), input); err != nil && rec.Code < 400 {
		t.Fatalf("%s %s breaks the document but answered %d: %v", method, path, rec.Code, err)
	}
	check.Body = io.NopCloser(bytes.NewReader(nil))
	response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
	response.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(t.Context(), response); err != nil {
		t.Fatalf("%s %s answered %d against the document: %v\n%s", method, path, rec.Code, err, rec.Body)
	}
	return rec
}

func walkKeys(v any, visit func(key string, value any)) {
	switch n := v.(type) {
	case map[string]any:
		for k, child := range n {
			visit(k, child)
			walkKeys(child, visit)
		}
	case []any:
		for _, child := range n {
			walkKeys(child, visit)
		}
	}
}

func TestRCALinkRoutesAndAnonymousViewsMatchTheDocument(t *testing.T) {
	h := newRCAHTTP(t)
	id := h.published(t)
	base := "/api/v1/observations/" + id.String()

	created := h.send(t, admin, http.MethodPost, base+"/rcas/1/public-links", `{}`, http.StatusCreated)
	var body struct {
		Link  struct{ ID string }
		Token string
		URL   string
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if want := "/api/v1/observations/" + id.String() + "/public-links/" + body.Link.ID; created.Header().Get("Location") != want {
		t.Errorf("Location %q, want %q", created.Header().Get("Location"), want)
	}
	h.send(t, admin, http.MethodGet, base+"/rcas/1/public-links", "", http.StatusOK)
	h.send(t, admin, http.MethodPost, base+"/rcas/1/public-links", `{"expires_at":"2020-01-01T00:00:00Z"}`, http.StatusUnprocessableEntity)
	h.send(t, admin, http.MethodPost, base+"/rcas/1/public-links", `{"expires_at":"soon"}`, http.StatusBadRequest)
	h.send(t, admin, http.MethodPost, base+"/rcas/0/public-links", `{}`, http.StatusBadRequest)
	h.send(t, admin, http.MethodPost, base+"/rcas/7/public-links", `{}`, http.StatusNotFound)
	h.send(t, editor, http.MethodPost, base+"/rcas/1/public-links", `{}`, http.StatusForbidden)
	h.send(t, editor, http.MethodGet, base+"/rcas/1/public-links", "", http.StatusForbidden)

	view := h.send(t, viewer, http.MethodGet, "/api/v1/public/"+body.Token+"/rca", "", http.StatusOK)
	var shown map[string]any
	if err := json.Unmarshal(view.Body.Bytes(), &shown); err != nil {
		t.Fatal(err)
	}
	walkKeys(shown, func(key string, _ any) {
		if key == "query" || key == "queries" || key == "observation_id" || key == "created_by" {
			t.Errorf("the public view carries %q: %s", key, view.Body)
		}
	})
	for _, secret := range []string{id.String(), admin.ID, "secret_metric_total"} {
		if strings.Contains(view.Body.String(), secret) {
			t.Errorf("the public view carries %q: %s", secret, view.Body)
		}
	}
	items := shown["blocks"].([]any)
	snapshot := items[1].(map[string]any)["snapshot_id"].(string)
	h.send(t, viewer, http.MethodGet, "/api/v1/public/"+body.Token+"/rca/snapshots/"+snapshot, "", http.StatusOK)

	unknown := "pub_" + strings.Repeat("B", 43)
	missing := h.send(t, viewer, http.MethodGet, "/api/v1/public/"+unknown+"/rca", "", http.StatusNotFound)
	dashboardMissing := h.send(t, viewer, http.MethodGet, "/api/v1/public/"+unknown, "", http.StatusNotFound)
	if missing.Body.String() != dashboardMissing.Body.String() {
		t.Errorf("the RCA 404 differs from the dashboard 404: %s and %s", missing.Body, dashboardMissing.Body)
	}
	h.send(t, viewer, http.MethodGet, "/api/v1/public/"+body.Token, "", http.StatusNotFound)
	h.send(t, viewer, http.MethodGet, "/api/v1/public/"+body.Token+"/rca/snapshots/not-a-uuid", "", http.StatusNotFound)
	h.send(t, viewer, http.MethodGet, "/api/v1/public/"+body.Token+"/rca/snapshots/00000000-0000-4000-8000-000000000000", "", http.StatusNotFound)

	h.send(t, editor, http.MethodDelete, base+"/public-links/"+body.Link.ID, "", http.StatusForbidden)
	h.send(t, admin, http.MethodDelete, base+"/public-links/"+body.Link.ID, "", http.StatusNoContent)
	h.send(t, admin, http.MethodDelete, base+"/public-links/"+body.Link.ID, "", http.StatusNotFound)
	h.send(t, viewer, http.MethodGet, "/api/v1/public/"+body.Token+"/rca", "", http.StatusNotFound)
}

func TestRCALinkRoutesAnswerUserScopedAndLimitCodes(t *testing.T) {
	h := newRCAHTTP(t)
	h.freezer.userScoped = true
	id := h.published(t)
	base := "/api/v1/observations/" + id.String()
	rec := h.send(t, admin, http.MethodPost, base+"/rcas/1/public-links", `{}`, http.StatusUnprocessableEntity)
	if !strings.Contains(rec.Body.String(), `"user_scoped"`) {
		t.Errorf("body %s", rec.Body)
	}
	h.freezer.userScoped = false
	if _, err := h.observations.Publish(t.Context(), admin, id); err != nil {
		t.Fatal(err)
	}
	for range maxLinksPerObservation {
		h.createLink(t, id, 2, CreateRCAInput{})
	}
	rec = h.send(t, admin, http.MethodPost, base+"/rcas/2/public-links", `{}`, http.StatusConflict)
	if !strings.Contains(rec.Body.String(), `"too_many_links"`) {
		t.Errorf("body %s", rec.Body)
	}
	personal := h.published(t)
	h.db.Exec(t, `UPDATE observations SET folder_id = (SELECT id FROM folders WHERE personal AND owner_id = $2::uuid) WHERE id = $1`, personal, owner.ID)
	h.send(t, owner, http.MethodPost, "/api/v1/observations/"+personal.String()+"/rcas/1/public-links", `{}`, http.StatusForbidden)
}
