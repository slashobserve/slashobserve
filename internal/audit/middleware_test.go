package audit

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const dashboardID = "55555555-5555-4555-8555-555555555555"

type requestEvent struct {
	action, outcome, reason, targetKind, targetID, route, method string
	status                                                       int
}

func (s storeRig) engine(t *testing.T, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	mw, err := s.recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	api := engine.Group("/api/v1", mw)
	api.POST("/dashboards/:id/versions", handler)
	api.POST("/dashboards/:id/viewers", handler)
	api.GET("/dashboards/:id/import", handler)
	api.GET("/dashboards/:id", handler)
	api.GET("/dashboards/:id/versions/:version", handler)
	api.GET("/dashboards", handler)
	api.POST("/dashboards", handler)
	api.POST("/datasources/:uid/query", handler)
	return engine
}

func serve(t *testing.T, engine *gin.Engine, method, path string) (*httptest.ResponseRecorder, uuid.UUID) {
	t.Helper()
	return serveWith(t, engine, method, path, nil)
}

func serveWith(t *testing.T, engine *gin.Engine, method, path string, header map[string]string) (*httptest.ResponseRecorder, uuid.UUID) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	req.RemoteAddr = "192.0.2.7:4100"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	id, err := uuid.Parse(rec.Header().Get(RequestIDHeader))
	if err != nil || id.Version() != 7 {
		t.Fatalf("%s = %q, want a UUID v7", RequestIDHeader, rec.Header().Get(RequestIDHeader))
	}
	return rec, id
}

func (s storeRig) eventsOf(t *testing.T, request uuid.UUID) []requestEvent {
	t.Helper()
	rows, err := s.db.SQLDB(t).QueryContext(t.Context(), `SELECT action, outcome, coalesce(reason, ''), coalesce(target_kind, ''), coalesce(target_id, ''), coalesce(route, ''), coalesce(http_method, ''), coalesce(status, 0)
FROM audit_events WHERE request_id = $1 AND client_ip = '192.0.2.7'`, request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []requestEvent
	for rows.Next() {
		var e requestEvent
		if err := rows.Scan(&e.action, &e.outcome, &e.reason, &e.targetKind, &e.targetID, &e.route, &e.method, &e.status); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestARefusedRequestRecordsDeniedAndAnswersAsBefore(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	const body = `{"error":"you may only view this dashboard","code":"forbidden_role"}`
	engine := s.engine(t, func(c *gin.Context) { c.Data(http.StatusForbidden, "application/json", []byte(body)) })
	rec, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	if rec.Code != http.StatusForbidden || rec.Body.String() != body {
		t.Fatalf("answer = %d %s, want the handler's 403", rec.Code, rec.Body.String())
	}
	s.run(t)()
	want := requestEvent{action: "dashboard.update", outcome: "denied", reason: "forbidden_role", targetKind: "dashboard", targetID: dashboardID, route: "/api/v1/dashboards/{id}/versions", method: "POST", status: 403}
	if got := s.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestAHandlerThatRecordedIsNotRecordedTwice(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		err := s.db.InTx(c.Request.Context(), func(q *dbsqlc.Queries) error {
			return s.recorder.Record(c.Request.Context(), q, Event{Action: "dashboard.update", Target: Target{Kind: "dashboard", ID: c.Param("id")}, Data: map[string]any{"version": 2}})
		})
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusCreated)
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	s.run(t)()
	got := s.eventsOf(t, request)
	if len(got) != 1 || got[0].outcome != "success" || got[0].status != 0 {
		t.Fatalf("events = %+v, want only the handler's own success", got)
	}
}

func TestARolledBackRecordBecomesAFailedEvent(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		err := s.db.InTx(c.Request.Context(), func(q *dbsqlc.Queries) error {
			if err := s.recorder.Record(c.Request.Context(), q, Event{Action: "dashboard.update", Target: Target{Kind: "dashboard", ID: c.Param("id")}}); err != nil {
				return err
			}
			return errors.New("lost the race")
		})
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "the dashboard changed", "code": "version_conflict"})
		}
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	s.run(t)()
	want := requestEvent{action: "dashboard.update", outcome: "failed", reason: "version_conflict", targetKind: "dashboard", targetID: dashboardID, route: "/api/v1/dashboards/{id}/versions", method: "POST", status: 409}
	if got := s.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestADeclaredReadRecordsAnAccessEvent(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"id": c.Param("id")}) })
	_, request := serve(t, engine, http.MethodGet, "/api/v1/dashboards/"+dashboardID)
	s.run(t)()
	want := requestEvent{action: "dashboard.view", outcome: "success", targetKind: "dashboard", targetID: dashboardID, route: "/api/v1/dashboards/{id}", method: "GET", status: 200}
	if got := s.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

type queryEvent struct {
	outcome, datasource, targetID, query, rows, from, to string
}

func (s storeRig) queryEventsOf(t *testing.T, request uuid.UUID) []queryEvent {
	t.Helper()
	rows, err := s.db.SQLDB(t).QueryContext(t.Context(), `SELECT outcome, coalesce(datasource_uid, ''), coalesce(target_id, ''), coalesce(data->>'query', ''), coalesce(data->>'row_count', ''), coalesce(data->>'from', ''), coalesce(data->>'to', '')
FROM audit_events WHERE request_id = $1 AND action = 'query.run' ORDER BY datasource_uid`, request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []queryEvent
	for rows.Next() {
		var e queryEvent
		if err := rows.Scan(&e.outcome, &e.datasource, &e.targetID, &e.query, &e.rows, &e.from, &e.to); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestAQueryRunRecordsEachDatasourceTheRequestQueriedWithItsTextWindowAndRows(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	at := func(minute int) time.Time { return time.Date(2026, 11, 21, 10, minute, 0, 0, time.UTC) }
	notes := []Query{
		{Datasource: "prom", Text: "up", From: at(10), To: at(20), Rows: 3},
		{Datasource: "loki", Text: `{app="a"}`, From: at(0), To: at(30), Rows: 7},
		{Datasource: "prom", Text: "up", From: at(0), To: at(25), Rows: 4},
		{Datasource: "prom", Text: "rate(x[1m])", From: at(15), To: at(30), Rows: 1},
	}
	status := http.StatusOK
	engine := s.engine(t, func(c *gin.Context) {
		for _, q := range notes {
			NoteQuery(c.Request.Context(), q)
		}
		c.JSON(status, gin.H{})
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/datasources/prom/query")
	status = http.StatusBadGateway
	_, failed := serve(t, engine, http.MethodPost, "/api/v1/datasources/prom/query")
	s.run(t)()
	want := []queryEvent{
		{"success", "loki", "loki", `{app="a"}`, "7", "2026-11-21T10:00:00Z", "2026-11-21T10:30:00Z"},
		{"success", "prom", "prom", "up\nrate(x[1m])", "8", "2026-11-21T10:00:00Z", "2026-11-21T10:30:00Z"},
	}
	if got := s.queryEventsOf(t, request); !slices.Equal(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	if got := s.queryEventsOf(t, failed); len(got) != 2 || got[0].outcome != "failed" || got[1].outcome != "failed" {
		t.Fatalf("a failed request recorded %+v, want each queried datasource as failed", got)
	}
}

func TestARepeatOfAQueryOverTheSamePeriodIsRecordedOnceInTenMinutes(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	base := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	note := Query{Datasource: "prom", Text: "up", From: base, To: base.Add(15 * time.Minute)}
	status := http.StatusOK
	engine := s.engine(t, func(c *gin.Context) {
		NoteQuery(c.Request.Context(), note)
		c.JSON(status, gin.H{})
	})
	_, first := serve(t, engine, http.MethodPost, "/api/v1/datasources/prom/query")
	note.From, note.To = base.Add(time.Minute), base.Add(16*time.Minute)
	_, refresh := serve(t, engine, http.MethodPost, "/api/v1/datasources/prom/query")
	note.From, note.To = base, base.Add(time.Hour)
	_, wider := serve(t, engine, http.MethodPost, "/api/v1/datasources/prom/query")
	note.From, note.To, note.Text = base, base.Add(15*time.Minute), "up == 0"
	_, other := serve(t, engine, http.MethodPost, "/api/v1/datasources/prom/query")
	note.Text, status = "up", http.StatusBadGateway
	_, failed := serve(t, engine, http.MethodPost, "/api/v1/datasources/prom/query")
	s.run(t)()
	for name, tc := range map[string]struct {
		request uuid.UUID
		want    int
	}{"the first run": {first, 1}, "a refresh of it": {refresh, 0}, "a wider period": {wider, 1}, "another query": {other, 1}, "a failure of the repeat": {failed, 1}} {
		if got := s.queryEventsOf(t, tc.request); len(got) != tc.want {
			t.Errorf("%s recorded %+v, want %d events", name, got, tc.want)
		}
	}
}

func TestARecordedQueryIsCappedAndAStreamIsNeverFolded(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	long := strings.Repeat("a", 2*MaxQueryBytes)
	stream := false
	engine := s.engine(t, func(c *gin.Context) {
		NoteQuery(c.Request.Context(), Query{Datasource: "loki", Text: long})
		if stream {
			Conclude(c.Request.Context(), Result{Outcome: OutcomeSuccess, Target: Target{Kind: "datasource", ID: "loki"}, Data: map[string]any{"stream": true}})
		}
		c.JSON(http.StatusOK, gin.H{})
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/datasources/loki/query")
	stream = true
	_, first := serve(t, engine, http.MethodPost, "/api/v1/datasources/loki/query")
	_, second := serve(t, engine, http.MethodPost, "/api/v1/datasources/loki/query")
	s.run(t)()
	got := s.queryEventsOf(t, request)
	if len(got) != 1 || got[0].query != long[:MaxQueryBytes]+" [truncated]" {
		t.Fatalf("recorded %d events, first query of %d bytes", len(got), len(got[0].query))
	}
	for _, id := range []uuid.UUID{first, second} {
		if got := s.queryEventsOf(t, id); len(got) != 1 {
			t.Errorf("a repeated stream recorded %+v, want one event each", got)
		}
	}
}

func TestQueriesNotedOnARouteThatIsNotAQueryRunAreNotRecordedAsOne(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		NoteQuery(c.Request.Context(), Query{Datasource: "prom", Text: "up"})
		c.JSON(http.StatusOK, gin.H{})
	})
	_, view := serve(t, engine, http.MethodGet, "/api/v1/dashboards/"+dashboardID)
	_, bare := serve(t, engine, http.MethodPost, "/api/v1/datasources/prom/query")
	s.run(t)()
	if got := s.queryEventsOf(t, view); len(got) != 0 {
		t.Fatalf("a dashboard view recorded queries %+v", got)
	}
	if got := s.eventsOf(t, view); len(got) != 1 || got[0].action != "dashboard.view" {
		t.Fatalf("a dashboard view recorded %+v", got)
	}
	if got := s.queryEventsOf(t, bare); len(got) != 1 || got[0].targetID != "prom" || got[0].query != "up" {
		t.Fatalf("a noted query recorded %+v", got)
	}
}

func TestARouteDeclaredNoneRecordsNothing(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"items": []string{}}) })
	_, request := serve(t, engine, http.MethodGet, "/api/v1/dashboards")
	s.run(t)()
	if got := s.eventsOf(t, request); len(got) != 0 {
		t.Fatalf("events = %+v, want none", got)
	}
}

func TestARefusalOnARouteWithoutAnActionRecordsRequestRefuse(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "slow down", "code": "rate_limited"})
	})
	_, request := serve(t, engine, http.MethodGet, "/api/v1/dashboards")
	s.run(t)()
	want := requestEvent{action: string(ActionRefuse), outcome: "denied", reason: "rate_limited", route: "/api/v1/dashboards", method: "GET", status: 429}
	if got := s.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestANotFoundIsDeniedOnlyOnARouteWithATarget(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "code": "not_found"})
	})
	_, onTarget := serve(t, engine, http.MethodGet, "/api/v1/dashboards/"+dashboardID)
	_, onKind := serve(t, engine, http.MethodPost, "/api/v1/dashboards")
	_, onNone := serve(t, engine, http.MethodGet, "/api/v1/dashboards")
	s.run(t)()
	if got := s.eventsOf(t, onTarget); len(got) != 1 || got[0].action != "dashboard.view" || got[0].outcome != "denied" {
		t.Errorf("404 on a dashboard id: %+v, want dashboard.view denied", got)
	}
	if got := s.eventsOf(t, onKind); len(got) != 1 || got[0].action != "dashboard.create" || got[0].outcome != "failed" {
		t.Errorf("404 on a create: %+v, want dashboard.create failed", got)
	}
	if got := s.eventsOf(t, onNone); len(got) != 0 {
		t.Errorf("404 on a route declared none: %+v, want nothing", got)
	}
}

func TestARefusedReadOnARouteDeclaredNoneWithATargetIsRecordedEveryTime(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	status, code := http.StatusNotFound, "not_found"
	engine := s.engine(t, func(c *gin.Context) { c.JSON(status, gin.H{"error": "refused", "code": code}) })
	path := "/api/v1/dashboards/" + dashboardID + "/versions/3"
	_, first := serve(t, engine, http.MethodGet, path)
	_, second := serve(t, engine, http.MethodGet, path)
	status, code = http.StatusForbidden, "forbidden_role"
	_, forbidden := serve(t, engine, http.MethodGet, path)
	status = http.StatusOK
	_, found := serve(t, engine, http.MethodGet, path)
	s.run(t)()
	missing := requestEvent{action: string(ActionRefuse), outcome: "denied", reason: "not_found", targetKind: "dashboard", targetID: dashboardID, route: "/api/v1/dashboards/{id}/versions/{version}", method: "GET", status: 404}
	for name, request := range map[string]uuid.UUID{"first": first, "second": second} {
		if got := s.eventsOf(t, request); len(got) != 1 || got[0] != missing {
			t.Errorf("%s refused read = %+v, want %+v", name, got, missing)
		}
	}
	refusal := missing
	refusal.reason, refusal.status = "forbidden_role", 403
	if got := s.eventsOf(t, forbidden); len(got) != 1 || got[0] != refusal {
		t.Errorf("403 = %+v, want %+v", got, refusal)
	}
	if got := s.eventsOf(t, found); len(got) != 0 {
		t.Errorf("a successful read recorded %+v, want nothing", got)
	}
}

func TestARefusedQueryRunNamesTheDatasourceItWasRefusedOn(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	status, code := http.StatusForbidden, "forbidden_datasource"
	engine := s.engine(t, func(c *gin.Context) { c.JSON(status, gin.H{"error": "refused", "code": code}) })
	_, forbidden := serve(t, engine, http.MethodPost, "/api/v1/datasources/prod-logs/query")
	status, code = http.StatusTooManyRequests, "rate_limited"
	_, limited := serve(t, engine, http.MethodPost, "/api/v1/datasources/prod-logs/query")
	status, code = http.StatusUnprocessableEntity, "invalid_request"
	_, invalid := serve(t, engine, http.MethodPost, "/api/v1/datasources/prod-logs/query")
	s.run(t)()
	for name, tc := range map[string]struct {
		request uuid.UUID
		outcome string
	}{"a 403": {forbidden, "denied"}, "a 429": {limited, "denied"}, "a 422": {invalid, "failed"}} {
		want := []queryEvent{{outcome: tc.outcome, datasource: "prod-logs", targetID: "prod-logs"}}
		if got := s.queryEventsOf(t, tc.request); !slices.Equal(got, want) {
			t.Errorf("%s recorded %+v, want %+v", name, got, want)
		}
	}
}

func TestARepeatOfAQueryIsRecordedOncePerRunAndCredentialNotOncePerPerson(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	base := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	engine := s.engine(t, func(c *gin.Context) {
		Update(c.Request.Context(), func(sc *Scope) {
			if run := c.GetHeader("X-Run"); run != "" {
				sc.Initiator, sc.RunID = InitiatorAgent, uuid.NullUUID{UUID: uuid.MustParse(run), Valid: true}
			}
			sc.Auth.CredentialID = c.GetHeader("X-Key")
		})
		NoteQuery(c.Request.Context(), Query{Datasource: "loki", Text: `{app="shop"}`, From: base, To: base.Add(15 * time.Minute)})
		c.JSON(http.StatusOK, gin.H{})
	})
	const runA, runB = "00000000-0000-4000-8000-00000000000a", "00000000-0000-4000-8000-00000000000b"
	path := "/api/v1/datasources/loki/query"
	_, logsPage := serveWith(t, engine, http.MethodPost, path, nil)
	_, logsPageAgain := serveWith(t, engine, http.MethodPost, path, nil)
	_, firstRunA := serveWith(t, engine, http.MethodPost, path, map[string]string{"X-Run": runA})
	_, secondRunA := serveWith(t, engine, http.MethodPost, path, map[string]string{"X-Run": runA})
	_, firstRunB := serveWith(t, engine, http.MethodPost, path, map[string]string{"X-Run": runB})
	_, keyOne := serveWith(t, engine, http.MethodPost, path, map[string]string{"X-Key": "key-1"})
	_, keyTwo := serveWith(t, engine, http.MethodPost, path, map[string]string{"X-Key": "key-2"})
	s.run(t)()
	for name, tc := range map[string]struct {
		request uuid.UUID
		want    int
	}{
		"the Logs page":          {logsPage, 1},
		"the Logs page again":    {logsPageAgain, 0},
		"run A":                  {firstRunA, 1},
		"run A again":            {secondRunA, 0},
		"run B":                  {firstRunB, 1},
		"the first service key":  {keyOne, 1},
		"the second service key": {keyTwo, 1},
	} {
		if got := s.queryEventsOf(t, tc.request); len(got) != tc.want {
			t.Errorf("%s recorded %+v, want %d events", name, got, tc.want)
		}
	}
}

func TestARefusedCallThatNamesAHiddenIdIsRecordedOnEveryMethodButOnlyForANotFound(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	status, code := http.StatusNotFound, "not_found"
	engine := s.engine(t, func(c *gin.Context) { c.JSON(status, gin.H{"error": "refused", "code": code}) })
	_, probe := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/viewers")
	_, probeAgain := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/viewers")
	code = "not_imported"
	_, notImported := serve(t, engine, http.MethodGet, "/api/v1/dashboards/"+dashboardID+"/import")
	code = "version_not_found"
	_, noVersion := serve(t, engine, http.MethodGet, "/api/v1/dashboards/"+dashboardID+"/versions/9")
	s.run(t)()
	want := requestEvent{action: string(ActionRefuse), outcome: "denied", reason: "not_found", targetKind: "dashboard", targetID: dashboardID, route: "/api/v1/dashboards/{id}/viewers", method: "POST", status: 404}
	for name, request := range map[string]uuid.UUID{"first": probe, "second": probeAgain} {
		if got := s.eventsOf(t, request); len(got) != 1 || got[0] != want {
			t.Errorf("%s POST probe = %+v, want %+v", name, got, want)
		}
	}
	for name, request := range map[string]uuid.UUID{"not_imported": notImported, "version_not_found": noVersion} {
		if got := s.eventsOf(t, request); len(got) != 0 {
			t.Errorf("a 404 %s under a visible dashboard recorded %+v, want nothing", name, got)
		}
	}
}

func TestAConcludedFailureOnANotFoundIsFailedNotDenied(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		Conclude(c.Request.Context(), Result{Outcome: OutcomeFailed, Reason: "not_found", Target: Target{Kind: "datasource", ID: "tempo"}})
		c.JSON(http.StatusNotFound, gin.H{"error": "trace not found", "code": "not_found"})
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/datasources/tempo/query")
	s.run(t)()
	want := []queryEvent{{outcome: "failed", datasource: "tempo", targetID: "tempo"}}
	if got := s.queryEventsOf(t, request); !slices.Equal(got, want) {
		t.Fatalf("recorded %+v, want %+v", got, want)
	}
}

func TestTheEventCarriesTheAddressThePlatformResolved(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	mw, err := s.recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	if err := engine.SetTrustedProxies([]string{"192.0.2.0/24"}); err != nil {
		t.Fatal(err)
	}
	engine.Group("/api/v1", mw).POST("/dashboards/:id/versions", func(c *gin.Context) { c.Status(http.StatusForbidden) })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions", nil)
	req.RemoteAddr = "192.0.2.7:4100"
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.9")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	request, err := uuid.Parse(rec.Header().Get(RequestIDHeader))
	if err != nil {
		t.Fatal(err)
	}
	s.run(t)()
	var got string
	if err := s.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT host(client_ip) FROM audit_events WHERE request_id = $1`, request).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "203.0.113.9" {
		t.Fatalf("client_ip = %s, want 203.0.113.9", got)
	}
}
