package trail

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
)

type httpRig struct {
	readerRig
	recorder *audit.Recorder
	stop     func()
}

func newHTTPRig(t *testing.T) *httpRig {
	t.Helper()
	r := &httpRig{readerRig: newReaderRig(t)}
	r.recorder = audit.New(audit.Config{DB: r.db.DB, Server: "test", Version: "test", Logger: slog.New(slog.DiscardHandler)})
	audit.Install(r.recorder)
	t.Cleanup(func() { audit.Install(nil) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.recorder.Run(ctx)
	}()
	r.stop = func() {
		cancel()
		<-done
	}
	t.Cleanup(r.stop)
	return r
}

func (r *httpRig) engine(t *testing.T, principal *auth.Principal) *gin.Engine {
	t.Helper()
	mw, err := r.recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	api := engine.Group("/api/v1", mw, func(c *gin.Context) {
		if principal != nil {
			ctx := auth.WithPrincipal(c.Request.Context(), *principal)
			audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(*principal) })
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	})
	NewHandler(r.reader, fromContext, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	return engine
}

func (r *httpRig) get(t *testing.T, engine *gin.Engine, path string, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	target, err := url.Parse("/api/v1" + path)
	if err != nil {
		t.Fatal(err)
	}
	merged := target.Query()
	for k, v := range query {
		merged[k] = v
	}
	target.RawQuery = merged.Encode()
	req := httptest.NewRequest(http.MethodGet, target.String(), nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func (r *httpRig) hourWindow(extra url.Values) url.Values {
	q := url.Values{"since": {r.base.Add(-time.Hour).Format(time.RFC3339)}, "until": {r.base.Format(time.RFC3339)}}
	for k, v := range extra {
		q[k] = v
	}
	return q
}

var admin = auth.Principal{Kind: auth.KindUser, ID: "7d1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "Ada", Role: auth.RoleAdmin}

func auditRoutes(eventID string) []string {
	return []string{"/audit/events", "/audit/events/" + eventID, "/audit/facets?field=actor_name", "/audit/histogram", "/audit/aggregate?q=" + url.QueryEscape(`{"aggs":{"o":{"terms":{"field":"origin"}}}}`), "/audit/fields", "/audit/export"}
}

func TestOnlyAdminsReadTheTrail(t *testing.T) {
	r := newHTTPRig(t)
	ids := trailtest.Insert(t, r.db, trailtest.Event{At: time.Now().UTC().Add(-time.Minute)})
	refused := map[string]*auth.Principal{
		"viewer":        {Kind: auth.KindUser, ID: admin.ID, Role: auth.RoleViewer},
		"commenter":     {Kind: auth.KindUser, ID: admin.ID, Role: auth.RoleCommenter},
		"editor":        {Kind: auth.KindUser, ID: admin.ID, Role: auth.RoleEditor},
		"editor key":    {Kind: auth.KindServiceAccount, ID: admin.ID, Role: auth.RoleEditor},
		"anonymous":     nil,
		"no role given": {Kind: auth.KindUser, ID: admin.ID},
	}
	for name, p := range refused {
		engine := r.engine(t, p)
		for _, route := range auditRoutes(ids[0]) {
			rec := r.get(t, engine, route, nil)
			var body struct {
				Code string `json:"code"`
			}
			if rec.Code != http.StatusForbidden || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Code != "forbidden_role" {
				t.Errorf("%s GET %s = %d %s, want 403 forbidden_role", name, route, rec.Code, rec.Body)
			}
		}
	}
	for name, p := range map[string]*auth.Principal{"admin": &admin, "admin key": {Kind: auth.KindServiceAccount, ID: admin.ID, Role: auth.RoleAdmin}} {
		engine := r.engine(t, p)
		for _, route := range auditRoutes(ids[0]) {
			if rec := r.get(t, engine, route, nil); rec.Code != http.StatusOK {
				t.Errorf("%s GET %s = %d %s, want 200", name, route, rec.Code, rec.Body)
			}
		}
	}
}

func TestAParseErrorAnswersWithItsOffsets(t *testing.T) {
	r := newHTTPRig(t)
	engine := r.engine(t, &admin)
	for _, route := range []string{"/audit/events", "/audit/facets", "/audit/histogram", "/audit/aggregate", "/audit/export"} {
		rec := r.get(t, engine, route, url.Values{"q": {"outcome:denied and status:x"}, "field": {"actor_name"}})
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusUnprocessableEntity || body["code"] != CodeQueryInvalid || body["start"] != 19.0 || body["end"] != 27.0 || !strings.Contains(body["error"].(string), `"x"`) {
			t.Errorf("%s = %d %v", route, rec.Code, body)
		}
	}
}

func TestEveryAnswerListsTheWarningsOfItsQuery(t *testing.T) {
	r := newHTTPRig(t)
	engine := r.engine(t, &admin)
	warning := `"andd" is searched as text; did you mean "and"?`
	for _, route := range []string{"/audit/events", "/audit/facets", "/audit/histogram"} {
		for q, want := range map[string][]any{"outcome:denied andd origin:mcp": {warning}, "outcome:denied and origin:mcp": {}} {
			rec := r.get(t, engine, route, r.hourWindow(url.Values{"q": {q}, "field": {"actor_name"}}))
			var body struct {
				Warnings []any `json:"warnings"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK || body.Warnings == nil || !slices.Equal(body.Warnings, want) {
				t.Errorf("%s q=%q = %d %s, want warnings %v", route, q, rec.Code, rec.Body, want)
			}
		}
	}
	rec := r.get(t, engine, "/audit/aggregate", r.hourWindow(url.Values{"q": {`{"aggs":{"o":{"terms":{"field":"origin"}}}}`}}))
	if !strings.Contains(rec.Body.String(), `"warnings":[]`) {
		t.Errorf("aggregate = %d %s, want an empty warnings list", rec.Code, rec.Body)
	}
}

func TestBadParametersAnswer400(t *testing.T) {
	r := newHTTPRig(t)
	engine := r.engine(t, &admin)
	for _, tc := range []struct {
		path  string
		query url.Values
	}{
		{"/audit/events", url.Values{"limit": {"0"}}},
		{"/audit/events", url.Values{"limit": {"1001"}}},
		{"/audit/events", url.Values{"since": {"now-400d"}}},
		{"/audit/events", url.Values{"cursor": {"x"}}},
		{"/audit/facets", nil},
		{"/audit/facets", url.Values{"field": {"actor_name"}, "limit": {"101"}}},
		{"/audit/histogram", url.Values{"buckets": {"201"}}},
		{"/audit/histogram", url.Values{"buckets": {"07"}}},
		{"/audit/export", url.Values{"format": {"xml"}}},
	} {
		if rec := r.get(t, engine, tc.path, tc.query); rec.Code != http.StatusBadRequest {
			t.Errorf("%s?%s = %d %s, want 400", tc.path, tc.query.Encode(), rec.Code, rec.Body)
		}
	}
	if rec := r.get(t, engine, "/audit/events/"+trailtest.IDAt(r.base).String(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown event = %d %s", rec.Code, rec.Body)
	}
}

func TestTheExportIsAnAttachmentWithItsRowLimit(t *testing.T) {
	r := newHTTPRig(t)
	trailtest.Insert(t, r.db, trailtest.Event{At: r.at(-5)}, trailtest.Event{At: r.at(-6)})
	engine := r.engine(t, &admin)
	rec := r.get(t, engine, "/audit/export", r.hourWindow(url.Values{"format": {"csv"}}))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" || rec.Header().Get(RowLimitHeader) != "1000000" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="audit-`) || strings.Count(rec.Body.String(), "\n") != 3 {
		t.Fatalf("export = %d %v\n%s", rec.Code, rec.Header(), rec.Body)
	}
}

type recordedRead struct {
	action, outcome, targetKind string
	data                        map[string]any
}

func (r *httpRig) reads(t *testing.T) []recordedRead {
	t.Helper()
	r.stop()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT action, outcome, coalesce(target_kind, ''), data FROM audit_events WHERE action LIKE 'audit.%' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []recordedRead
	for rows.Next() {
		var e recordedRead
		var raw []byte
		if err := rows.Scan(&e.action, &e.outcome, &e.targetKind, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &e.data); err != nil {
			t.Fatal(err)
		}
		if e.data == nil {
			e.data = map[string]any{}
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEveryReadOfTheTrailIsRecordedWithItsQueryAndView(t *testing.T) {
	r := newHTTPRig(t)
	ids := trailtest.Insert(t, r.db, trailtest.Event{At: r.at(-5), Actor: "secretname"}, trailtest.Event{At: r.at(-6)})
	engine := r.engine(t, &admin)
	const text = "actor_name:secretname"
	unknown := trailtest.IDAt(r.base).String()
	aggs := `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"10m"},"aggs":{"o":{"terms":{"field":"origin","size":1000}}}}}}`
	for _, call := range []struct {
		path  string
		query url.Values
	}{
		{"/audit/events", r.hourWindow(url.Values{"q": {text}})},
		{"/audit/facets", r.hourWindow(url.Values{"q": {text}, "field": {"origin"}})},
		{"/audit/histogram", r.hourWindow(url.Values{"q": {"lucene: " + text}, "buckets": {"6"}})},
		{"/audit/events/" + ids[0], nil},
		{"/audit/export", r.hourWindow(url.Values{"q": {text}, "format": {"ndjson"}})},
		{"/audit/events", url.Values{"q": {"outcome:ok"}}},
		{"/audit/export", url.Values{"q": {text}, "since": {"now-400d"}}},
		{"/audit/aggregate", r.hourWindow(url.Values{"q": {aggs}})},
		{"/audit/facets", r.hourWindow(url.Values{"q": {"outcome:ok"}, "field": {"origin"}})},
		{"/audit/facets", r.hourWindow(url.Values{"field": {"nofield"}})},
		{"/audit/events/" + unknown, nil},
		{"/audit/fields", nil},
	} {
		r.get(t, engine, call.path, call.query)
	}
	reads := r.reads(t)
	since, until := r.base.Add(-time.Hour).Format(time.RFC3339Nano), r.base.Format(time.RFC3339Nano)
	want := []recordedRead{
		{"audit.search", "success", "", map[string]any{"form": "kql", "since": since, "until": until, "query": text, "view": "events", "row_count": 1.0}},
		{"audit.search", "success", "", map[string]any{"form": "kql", "since": since, "until": until, "query": text, "view": "facets", "field": "origin", "row_count": 1.0}},
		{"audit.search", "success", "", map[string]any{"form": "lucene", "since": since, "until": until, "query": "lucene: " + text, "view": "histogram", "row_count": 6.0}},
		{"audit.search", "success", "", map[string]any{"form": "kql", "query": "id:" + ids[0], "view": "event", "row_count": 1.0}},
		{"audit.export", "success", "", map[string]any{"form": "kql", "since": since, "until": until, "query": text, "format": "ndjson", "row_count": 1.0, "truncated": false}},
		{"audit.search", "failed", "", map[string]any{"query": "outcome:ok"}},
		{"audit.export", "failed", "", map[string]any{"query": text}},
		{"audit.search", "success", "", map[string]any{"form": "dsl", "since": since, "until": until, "query": aggs, "view": "aggregate"}},
		{"audit.search", "failed", "", map[string]any{"query": "outcome:ok"}},
		{"audit.search", "failed", "", map[string]any{"form": "kql", "since": since, "until": until, "view": "facets", "field": "nofield"}},
		{"audit.search", "failed", "", map[string]any{"query": "id:" + unknown, "view": "event"}},
	}
	if len(reads) != len(want) {
		t.Fatalf("recorded %d reads: %+v", len(reads), reads)
	}
	for i, got := range reads {
		w := want[i]
		if got.action != w.action || got.outcome != w.outcome || got.targetKind != w.targetKind {
			t.Errorf("read %d = %+v, want %+v", i, got, w)
		}
		for key, value := range w.data {
			if got.data[key] != value {
				t.Errorf("read %d %s = %v, want %v in %+v", i, key, got.data[key], value, got)
			}
		}
	}
}

func TestARepeatedSearchIsRecordedOncePerActorAndQueryInTenMinutes(t *testing.T) {
	r := newHTTPRig(t)
	clock := r.base
	r.reader.now = func() time.Time { return clock }
	ada := r.engine(t, &admin)
	other := admin
	other.ID = "8e2d3b63-9d6f-4b64-8b4f-2e3e5d1b0c22"
	grace := r.engine(t, &other)
	events := func(engine *gin.Engine, text string) {
		t.Helper()
		if rec := r.get(t, engine, "/audit/events", r.hourWindow(url.Values{"q": {text}})); rec.Code != http.StatusOK {
			t.Fatalf("search %q = %d %s", text, rec.Code, rec.Body)
		}
	}
	events(ada, "origin:web")
	events(ada, "origin:web")
	events(ada, "origin:mcp")
	events(grace, "origin:web")
	clock = clock.Add(9 * time.Minute)
	events(ada, "origin:web")
	clock = clock.Add(2 * time.Minute)
	events(ada, "origin:web")
	var queries []string
	for _, read := range r.reads(t) {
		if read.action != "audit.search" {
			t.Fatalf("recorded %+v, want only searches", read)
		}
		queries = append(queries, read.data["query"].(string))
	}
	if want := []string{"origin:web", "origin:mcp", "origin:web", "origin:web"}; !slices.Equal(queries, want) {
		t.Fatalf("recorded searches = %q, want %q", queries, want)
	}
}

func TestARepeatOfAFacetHistogramAggregateOrEventReadIsRecordedOncePerViewAndQuery(t *testing.T) {
	r := newHTTPRig(t)
	ids := trailtest.Insert(t, r.db, trailtest.Event{At: r.at(-5)})
	clock := r.base
	r.reader.now = func() time.Time { return clock }
	engine := r.engine(t, &admin)
	aggs := url.QueryEscape(`{"aggs":{"o":{"terms":{"field":"origin"}}}}`)
	paths := []string{"/audit/facets?field=origin", "/audit/facets?field=actor_name", "/audit/histogram", "/audit/events/" + ids[0], "/audit/aggregate?q=" + aggs, "/audit/events"}
	for range 2 {
		for _, path := range paths {
			if rec := r.get(t, engine, path, r.hourWindow(nil)); rec.Code != http.StatusOK {
				t.Fatalf("%s = %d %s", path, rec.Code, rec.Body)
			}
		}
	}
	clock = clock.Add(11 * time.Minute)
	r.get(t, engine, "/audit/histogram", r.hourWindow(nil))
	var views []string
	for _, read := range r.reads(t) {
		view, _ := read.data["view"].(string)
		field, _ := read.data["field"].(string)
		views = append(views, view+" "+field)
	}
	want := []string{"facets origin", "facets actor_name", "histogram ", "event ", "aggregate ", "events ", "histogram "}
	if !slices.Equal(views, want) {
		t.Fatalf("recorded %q, want %q", views, want)
	}
}

func TestReadingTheFieldListRecordsNothing(t *testing.T) {
	r := newHTTPRig(t)
	engine := r.engine(t, &admin)
	if rec := r.get(t, engine, "/audit/fields", nil); rec.Code != http.StatusOK {
		t.Fatalf("fields = %d %s", rec.Code, rec.Body)
	}
	if reads := r.reads(t); len(reads) != 0 {
		t.Fatalf("recorded %+v, want nothing", reads)
	}
}

func TestAReadByAnotherAdminIsRecordedAgain(t *testing.T) {
	r := newHTTPRig(t)
	ada := r.engine(t, &admin)
	other := admin
	other.ID = "8e2d3b63-9d6f-4b64-8b4f-2e3e5d1b0c22"
	grace := r.engine(t, &other)
	for _, engine := range []*gin.Engine{ada, grace, ada} {
		r.get(t, engine, "/audit/histogram", r.hourWindow(nil))
	}
	if reads := r.reads(t); len(reads) != 2 {
		t.Fatalf("recorded %d reads, want one for each admin: %+v", len(reads), reads)
	}
}

func TestARecordedQueryIsCappedAndMarked(t *testing.T) {
	r := newHTTPRig(t)
	engine := r.engine(t, &admin)
	text := "actor_name:" + strings.Repeat("a", 2*audit.MaxQueryBytes)
	if rec := r.get(t, engine, "/audit/events", r.hourWindow(url.Values{"q": {text}})); rec.Code != http.StatusOK {
		t.Fatalf("search = %d %s", rec.Code, rec.Body)
	}
	reads := r.reads(t)
	if len(reads) != 1 {
		t.Fatalf("recorded %+v", reads)
	}
	got, _ := reads[0].data["query"].(string)
	if got != text[:audit.MaxQueryBytes]+" [truncated]" {
		t.Fatalf("recorded a %d byte query ending %q", len(got), got[max(0, len(got)-20):])
	}
}

type flushHook struct {
	gin.ResponseWriter
	after func()
}

func (w *flushHook) Flush() {
	w.ResponseWriter.Flush()
	w.after()
}

type principalsFunc func(*http.Request) (auth.Principal, bool, error)

func (f principalsFunc) RequestPrincipal(r *http.Request) (auth.Principal, bool, error) { return f(r) }

var fromContext = principalsFunc(func(r *http.Request) (auth.Principal, bool, error) {
	p, ok := auth.FromContext(r.Context())
	return p, ok, nil
})

func (r *httpRig) serve(t *testing.T, reader *Reader, principals Principals, hook func()) *httptest.Server {
	t.Helper()
	mw, err := r.recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	api := engine.Group("/api/v1", mw, func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), admin))
		c.Writer = &flushHook{ResponseWriter: c.Writer, after: hook}
		c.Next()
	})
	NewHandler(reader, principals, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

func (r *httpRig) insertPages(t *testing.T) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO audit_events (id, occurred_at, category, action, outcome, actor_kind, initiator, origin, server_id, server_version)
SELECT gen_random_uuid(), $1::timestamptz - make_interval(secs => n), 'content', 'dashboard.update', 'success', 'user', 'person', 'web', 'test', 'test'
FROM generate_series(1, $2::int) AS n`, r.base, exportPageRows+5)
}

func (r *httpRig) exportCutOff(t *testing.T, server *httptest.Server) {
	t.Helper()
	resp, err := http.Get(server.URL + "/api/v1/audit/export?" + r.hourWindow(nil).Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !errors.Is(err, io.ErrUnexpectedEOF) || strings.Count(string(body), "\n") != exportPageRows {
		t.Fatalf("export = %d, %d lines, read error %v; want the first page cut off", resp.StatusCode, strings.Count(string(body), "\n"), err)
	}
}

func TestAnExportStopsWhenTheCallerIsNoLongerAnAdmin(t *testing.T) {
	r := newHTTPRig(t)
	r.insertPages(t)
	var demoted atomic.Bool
	principals := principalsFunc(func(req *http.Request) (auth.Principal, bool, error) {
		p, ok := auth.FromContext(req.Context())
		if demoted.Load() {
			p.Role = auth.RoleEditor
		}
		return p, ok, nil
	})
	r.exportCutOff(t, r.serve(t, r.reader, principals, func() { demoted.Store(true) }))
	reads := r.reads(t)
	if len(reads) != 1 || reads[0].action != "audit.export" || reads[0].outcome != "denied" || reads[0].data["row_count"] != float64(exportPageRows) {
		t.Fatalf("recorded %+v", reads)
	}
	var reason string
	var status int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT reason, status FROM audit_events WHERE action = 'audit.export'`).Scan(&reason, &status); err != nil || reason != "forbidden_role" || status != http.StatusForbidden {
		t.Fatalf("recorded reason %q status %d (%v)", reason, status, err)
	}
}

func TestAnExportThatFailsAfterItsFirstPageIsCutOff(t *testing.T) {
	r := newHTTPRig(t)
	r.insertPages(t)
	second, err := db.Open(t.Context(), db.Config{URL: r.db.URL()}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	r.exportCutOff(t, r.serve(t, NewReader(second, slog.New(slog.DiscardHandler)), fromContext, func() { once.Do(second.Close) }))
	reads := r.reads(t)
	if len(reads) != 1 || reads[0].action != "audit.export" || reads[0].outcome != "failed" || reads[0].data["row_count"] != float64(exportPageRows) {
		t.Fatalf("recorded %+v", reads)
	}
}

func TestACompleteExportSaysWhetherRowsWereLeftOut(t *testing.T) {
	r := newHTTPRig(t)
	trailtest.Insert(t, r.db, trailtest.Event{At: r.at(-5)})
	server := r.serve(t, r.reader, fromContext, func() {})
	resp, err := http.Get(server.URL + "/api/v1/audit/export?" + r.hourWindow(nil).Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	if got := resp.Trailer.Get(TruncatedHeader); got != "false" {
		t.Fatalf("%s trailer = %q, want false", TruncatedHeader, got)
	}
}
