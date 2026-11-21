package tailhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

const auditQuery = "query=%7Bapp%3D%22audit%22%7D"

func withAdminsOnly(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	audit := &fakeLines{typ: "loki", responses: []response{lines(1)}}
	if err := f.h.registry.AddAdminsOnly(datasource.Instance{UID: "audit", Datasource: audit}); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestATailOnAnAdminsOnlyDatasourceIsNotFoundForANonAdmin(t *testing.T) {
	f := withAdminsOnly(t)
	for _, role := range []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor} {
		t.Run(string(role), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/datasources/audit/tail?"+auditQuery, nil)
			req.Header.Set("X-Role", string(role))
			rec := httptest.NewRecorder()
			f.engine.ServeHTTP(rec, req)
			expectError(t, rec, http.StatusNotFound, "not_found")
			missing := f.get("nope", auditQuery, "u1")
			if rec.Body.String() != missing.Body.String() {
				t.Fatalf("restricted body %s, missing body %s", rec.Body, missing.Body)
			}
		})
	}
}

func TestATailOnAnAdminsOnlyDatasourceClosesWhenTheAdminIsDemoted(t *testing.T) {
	f := withAdminsOnly(t)
	f.h.pingInterval = testPoll
	s := f.openOn("audit", "u1", auth.RoleAdmin, auditQuery)
	decode[readyData](t, s.next(t), "ready")
	f.sessions.role.Store(auth.RoleAdmin)
	if s.closedWithin(4 * testPoll) {
		t.Fatal("the stream closed while its reader was still an Admin")
	}
	f.sessions.role.Store(auth.RoleEditor)
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream stayed open after its reader stopped being an Admin")
	}
}

func TestATailClosesForANonAdminWhenItsDatasourceBecomesAdminsOnly(t *testing.T) {
	f := newFixture(t)
	f.h.pingInterval = testPoll
	app := datasource.Instance{UID: "app", Datasource: &fakeLines{typ: "loki", responses: []response{lines(1)}}}
	if err := f.h.registry.Put(app); err != nil {
		t.Fatal(err)
	}
	s := f.openOn("app", "u1", auth.RoleViewer, auditQuery)
	decode[readyData](t, s.next(t), "ready")
	if s.closedWithin(4 * testPoll) {
		t.Fatal("the stream closed while its datasource was open to everyone")
	}
	app.Audience = datasource.AudienceAdmins
	if err := f.h.registry.Put(app); err != nil {
		t.Fatal(err)
	}
	if !s.closedWithin(waitTimeout) {
		t.Fatal("a Viewer's stream stayed open after its datasource became Admins only")
	}
}

func TestATailOnAnEveryoneDatasourceIgnoresARoleChange(t *testing.T) {
	f := newFixture(t)
	f.h.pingInterval = testPoll
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D")
	decode[readyData](t, s.next(t), "ready")
	f.sessions.role.Store(auth.RoleViewer)
	if s.closedWithin(4 * testPoll) {
		t.Fatal("a tail on an everyone datasource closed on a ping")
	}
}
