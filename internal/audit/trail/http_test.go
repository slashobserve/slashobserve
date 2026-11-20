package trail

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/auth"
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
			c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), *principal))
		}
		c.Next()
	})
	NewHandler(r.reader, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	return engine
}

func (r *httpRig) get(t *testing.T, engine *gin.Engine, path string, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1"+path+"?"+query.Encode(), nil)
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

func TestBadParametersAnswer400(t *testing.T) {
	r := newHTTPRig(t)
	engine := r.engine(t, &admin)
	for _, tc := range []struct {
		path  string
		query url.Values
	}{
		{"/audit/events", url.Values{"limit": {"0"}}},
		{"/audit/events", url.Values{"limit": {"1001"}}},
		{"/audit/events", url.Values{"since": {"now-40d"}}},
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

func TestEveryReadOfTheTrailIsRecordedWithoutItsQueryText(t *testing.T) {
	r := newHTTPRig(t)
	ids := trailtest.Insert(t, r.db, trailtest.Event{At: r.at(-5), Actor: "secretname"}, trailtest.Event{At: r.at(-6)})
	engine := r.engine(t, &admin)
	const text = "actor_name:secretname"
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
	} {
		r.get(t, engine, call.path, call.query)
	}
	reads := r.reads(t)
	if len(reads) != 6 {
		t.Fatalf("recorded %d reads: %+v", len(reads), reads)
	}
	since, until := r.base.Add(-time.Hour).Format(time.RFC3339Nano), r.base.Format(time.RFC3339Nano)
	want := []recordedRead{
		{"audit.search", "success", "", map[string]any{"form": "kql", "since": since, "until": until, "view": "events", "row_count": 1.0}},
		{"audit.search", "success", "", map[string]any{"form": "kql", "since": since, "until": until, "view": "facets", "field": "origin", "row_count": 1.0}},
		{"audit.search", "success", "", map[string]any{"form": "lucene", "since": since, "until": until, "view": "histogram", "row_count": 6.0}},
		{"audit.search", "success", "audit_event", map[string]any{"view": "event", "row_count": 1.0}},
		{"audit.export", "success", "", map[string]any{"form": "kql", "since": since, "until": until, "format": "ndjson", "row_count": 1.0, "truncated": false}},
		{"audit.search", "failed", "", map[string]any{}},
	}
	for i, got := range reads {
		w := want[i]
		gotData, _ := json.Marshal(got.data)
		wantData, _ := json.Marshal(w.data)
		if got.action != w.action || got.targetKind != w.targetKind || string(gotData) != string(wantData) {
			t.Errorf("read %d = %+v, want %+v", i, got, w)
		}
		if strings.Contains(string(gotData), "secretname") {
			t.Errorf("read %d carries the query text: %s", i, gotData)
		}
	}
	if reads[5].outcome == "success" {
		t.Errorf("a refused query was recorded as a success: %+v", reads[5])
	}
}
