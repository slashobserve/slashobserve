package audit

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
	api.GET("/dashboards/:id", handler)
	api.GET("/dashboards", handler)
	return engine
}

func serve(t *testing.T, engine *gin.Engine, method, path string) (*httptest.ResponseRecorder, uuid.UUID) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
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

func TestARouteDeclaredNoneRecordsNothing(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"items": []string{}}) })
	_, request := serve(t, engine, http.MethodGet, "/api/v1/dashboards")
	s.run(t)()
	if got := s.eventsOf(t, request); len(got) != 0 {
		t.Fatalf("events = %+v, want none", got)
	}
}
