package users

import (
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const nilID = "00000000-0000-0000-0000-000000000000"

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(dbtest.Main(m))
}

type rig struct {
	t      *testing.T
	db     *dbtest.Database
	engine *gin.Engine
}

func newRig(t *testing.T) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	publicURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.New(auth.Config{PublicURL: publicURL, IssuerURL: "http://issuer.example/dex"}, database.Queries(), logger)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		if kind := c.GetHeader("X-Test-Kind"); kind != "" {
			p := auth.Principal{Kind: auth.PrincipalKind(kind), ID: c.GetHeader("X-Test-Id")}
			c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
		}
	})
	NewHandler(database.DB, authService, logger).Register(api)
	return &rig{t: t, db: database, engine: engine}
}

func (r *rig) addUser(name string, created time.Time) pgtype.UUID {
	r.t.Helper()
	u, err := r.db.Queries().UpsertUser(r.t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: name, Email: name + "@example.com", Name: name})
	if err != nil {
		r.t.Fatal(err)
	}
	r.db.Exec(r.t, "UPDATE users SET created_at = $1 WHERE id = $2", created, u.ID)
	return u.ID
}

func (r *rig) addSession(user pgtype.UUID, token string) {
	r.t.Helper()
	hash := sha256.Sum256([]byte(token))
	err := r.db.Queries().CreateSession(r.t.Context(), dbsqlc.CreateSessionParams{
		TokenHash: hash[:], UserID: user, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) do(method, path string, caller auth.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if caller.Kind != "" {
		req.Header.Set("X-Test-Kind", string(caller.Kind))
		req.Header.Set("X-Test-Id", caller.ID)
	}
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func userCaller(id pgtype.UUID) auth.Principal {
	return auth.Principal{Kind: auth.KindUser, ID: id.String()}
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) listJSON {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body listJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func names(l listJSON) []string {
	out := make([]string, 0, len(l.Users))
	for _, u := range l.Users {
		out = append(out, u.Name)
	}
	return out
}

func TestListPagesByFirstSignInEvenWhenActivityMoves(t *testing.T) {
	r := newRig(t)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	var ids []pgtype.UUID
	for i, name := range []string{"ada", "bob", "cyd", "dee", "eve"} {
		ids = append(ids, r.addUser(name, base.Add(time.Duration(i)*time.Minute)))
	}
	caller := userCaller(ids[0])

	first := decodeList(t, r.do(http.MethodGet, "/api/v1/users?limit=2", caller))
	if got := strings.Join(names(first), ","); got != "eve,dee" || first.NextCursor == nil {
		t.Fatalf("first page = %s, cursor %v", got, first.NextCursor)
	}

	r.db.Exec(t, "UPDATE users SET last_seen_at = now() WHERE id = $1", ids[0])

	second := decodeList(t, r.do(http.MethodGet, "/api/v1/users?limit=2&cursor="+url.QueryEscape(*first.NextCursor), caller))
	if got := strings.Join(names(second), ","); got != "cyd,bob" || second.NextCursor == nil {
		t.Fatalf("second page = %s, cursor %v", got, second.NextCursor)
	}

	last := decodeList(t, r.do(http.MethodGet, "/api/v1/users?limit=2&cursor="+url.QueryEscape(*second.NextCursor), caller))
	if got := strings.Join(names(last), ","); got != "ada" || last.NextCursor != nil {
		t.Fatalf("last page = %s, cursor %v", got, last.NextCursor)
	}
}

func TestListRejectsBadParameters(t *testing.T) {
	r := newRig(t)
	caller := userCaller(r.addUser("ada", time.Now()))
	for _, query := range []string{"limit=0", "limit=501", "limit=x", "cursor=%40%40"} {
		if rec := r.do(http.MethodGet, "/api/v1/users?"+query, caller); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, rec.Code)
		}
	}
}

func TestServiceAccountsCannotUseThePeopleRoutes(t *testing.T) {
	r := newRig(t)
	user := r.addUser("ada", time.Now())
	r.addSession(user, "ada-1")
	sa := auth.Principal{Kind: auth.KindServiceAccount, ID: nilID}
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/users"},
		{http.MethodDelete, "/api/v1/users/" + user.String() + "/sessions"},
	} {
		rec := r.do(tc.method, tc.path, sa)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"forbidden"`) {
			t.Errorf("%s %s = %d %s, want 403 forbidden", tc.method, tc.path, rec.Code, rec.Body)
		}
	}
	if n := countSessions(t, r, user); n != 1 {
		t.Fatalf("sessions = %d, want the one the test made", n)
	}
}

func countSessions(t *testing.T, r *rig, user pgtype.UUID) int {
	t.Helper()
	rows, err := r.db.Queries().ListUsers(t.Context(), dbsqlc.ListUsersParams{RowLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == user {
			return int(row.SessionCount)
		}
	}
	t.Fatalf("user %s not listed", user)
	return 0
}

func TestSignOutValidatesTheTarget(t *testing.T) {
	r := newRig(t)
	caller := userCaller(r.addUser("ada", time.Now()))
	if rec := r.do(http.MethodDelete, "/api/v1/users/"+nilID+"/sessions", caller); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"not_found"`) {
		t.Errorf("unknown id = %d %s, want 404 not_found", rec.Code, rec.Body)
	}
	if rec := r.do(http.MethodDelete, "/api/v1/users/not-a-uuid/sessions", caller); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id = %d %s, want 400", rec.Code, rec.Body)
	}
}

func TestSignOutEveryoneElseLeavesTheCallersCookie(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	bob := r.addUser("bob", time.Now())
	r.addSession(ada, "ada-1")
	r.addSession(bob, "bob-1")
	r.addSession(bob, "bob-2")

	rec := r.do(http.MethodDelete, "/api/v1/users/"+bob.String()+"/sessions", userCaller(ada))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("signing out someone else set cookies %v", got)
	}
	if countSessions(t, r, bob) != 0 || countSessions(t, r, ada) != 1 {
		t.Fatal("only bob's sessions should be gone")
	}
}

func TestSignOutYourselfExpiresTheSessionCookie(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	r.addSession(ada, "ada-1")
	r.addSession(ada, "ada-2")

	rec := r.do(http.MethodDelete, "/api/v1/users/"+ada.String()+"/sessions", userCaller(ada))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "so_session" || cookies[0].Value != "" || cookies[0].MaxAge >= 0 || !cookies[0].HttpOnly {
		t.Fatalf("cookies = %+v, want one expired HttpOnly so_session", cookies)
	}
	if n := countSessions(t, r, ada); n != 0 {
		t.Fatalf("sessions = %d, want 0", n)
	}
}
