package prompts

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/audittest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	adminID = "9b2a6e1c-3c1d-4a47-8f43-1a7a1f0f7e22"
)

type promptRig struct {
	t      *testing.T
	db     *dbtest.Database
	engine *gin.Engine
	who    auth.Principal
	flush  func()
}

func newPromptRig(t *testing.T) *promptRig {
	t.Helper()
	gin.SetMode(gin.TestMode)
	d := dbtest.New(t)
	for _, p := range []struct{ id, name string }{{callerID, "Ada"}, {adminID, "Root"}} {
		d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $2 || '@example.com', $2)`, p.id, p.name)
	}
	mw, flush := audittest.Middleware(t, d)
	r := &promptRig{t: t, db: d, engine: gin.New(), flush: flush}
	api := r.engine.Group("/api/v1", mw, func(c *gin.Context) {
		auth.Authenticated(c.Request.Context(), r.who, audit.AuthSession, "")
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), r.who))
	})
	NewHandler(d.DB, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	return r
}

func (r *promptRig) call(who auth.Principal, method, path, body string) *httptest.ResponseRecorder {
	r.t.Helper()
	r.who = who
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/prompts"+path, strings.NewReader(body)))
	return rec
}

func (r *promptRig) create(who auth.Principal, body string) string {
	r.t.Helper()
	rec := r.call(who, http.MethodPost, "", body)
	if rec.Code != http.StatusCreated {
		r.t.Fatalf("create answered %d: %s", rec.Code, rec.Body)
	}
	return strings.Split(rec.Header().Get("Location"), "/prompts/")[1]
}

func (r *promptRig) events() []string {
	r.t.Helper()
	r.flush()
	rows, err := r.db.SQLDB(r.t).QueryContext(r.t.Context(), `SELECT action || ' ' || outcome || ' ' || coalesce(target_name, '') || ' ' || coalesce(data->>'changed', '-') || ' ' || coalesce(data->>'from_name', '-') || ' ' || coalesce(data->>'owner_id', '-') || ' ' || actor_name
		FROM audit_events WHERE action LIKE 'saved_prompt.%' ORDER BY occurred_at, seq`)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			r.t.Fatal(err)
		}
		out = append(out, line)
	}
	return out
}

func TestOnlyChangesToASharedPromptAreRecorded(t *testing.T) {
	r := newPromptRig(t)
	ada := auth.Principal{Kind: auth.KindUser, ID: callerID, Name: "Ada", Role: auth.RoleEditor}
	root := auth.Principal{Kind: auth.KindUser, ID: adminID, Name: "Root", Role: auth.RoleAdmin}

	private := r.create(ada, `{"name":"mine","body":"why is checkout slow"}`)
	if rec := r.call(ada, http.MethodPatch, "/"+private, `{"name":"mine two"}`); rec.Code != http.StatusOK {
		t.Fatalf("rename answered %d: %s", rec.Code, rec.Body)
	}
	if rec := r.call(ada, http.MethodDelete, "/"+private, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete answered %d", rec.Code)
	}

	shared := r.create(ada, `{"name":"team","body":"why is checkout slow","shared":true}`)
	if rec := r.call(root, http.MethodPatch, "/"+shared, `{"body":"why is search slow"}`); rec.Code != http.StatusOK {
		t.Fatalf("admin edit answered %d: %s", rec.Code, rec.Body)
	}
	if rec := r.call(ada, http.MethodPatch, "/"+shared, `{"shared":false,"name":"back"}`); rec.Code != http.StatusOK {
		t.Fatalf("unshare answered %d: %s", rec.Code, rec.Body)
	}
	if rec := r.call(ada, http.MethodPatch, "/"+shared, `{"shared":true}`); rec.Code != http.StatusOK {
		t.Fatalf("share answered %d: %s", rec.Code, rec.Body)
	}
	if rec := r.call(root, http.MethodDelete, "/"+shared, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("admin delete answered %d", rec.Code)
	}

	want := []string{
		"saved_prompt.create success team - - " + callerID + " Ada",
		`saved_prompt.update success team ["body"] team ` + callerID + " Root",
		`saved_prompt.unshare success team ["name", "shared"] - ` + callerID + " Ada",
		`saved_prompt.share success back ["shared"] - ` + callerID + " Ada",
		"saved_prompt.delete success back - - " + callerID + " Root",
	}
	got := r.events()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAFailedRequestOnAPrivatePromptIsRecordedWithoutItsNameOrText(t *testing.T) {
	r := newPromptRig(t)
	ada := auth.Principal{Kind: auth.KindUser, ID: callerID, Name: "Ada", Role: auth.RoleEditor}

	if rec := r.call(ada, http.MethodPost, "", `{"name":"","body":"private text","shared":false}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create answered %d: %s", rec.Code, rec.Body)
	}
	r.flush()
	events := r.db.AuditEvents(t, "saved_prompt.create")
	if len(events) != 1 || events[0].Outcome != "failed" || len(events[0].Data) != 0 {
		t.Fatalf("events = %+v, want one failed create with no data", events)
	}
}
