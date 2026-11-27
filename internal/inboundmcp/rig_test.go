package inboundmcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/audittest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/egress/egresstest"
	"github.com/slashobserve/slashobserve/internal/sealed"
)

var testKey = sealed.Key("0123456789abcdef0123456789abcdef")

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type rig struct {
	database *dbtest.Database
	service  *Service
	store    *allowlist.Store
	fake     *fakeMCP
	admin    auth.Principal
	logs     *syncBuffer
	engine   *gin.Engine
	router   routers.Router
	caller   auth.Principal
	flush    func()
	bodies   []string
}

type rigOptions struct {
	lock allowlist.Lock
	key  sealed.Key
}

func newRig(t *testing.T, tools ...mcp.Tool) *rig {
	t.Helper()
	return newRigWith(t, rigOptions{key: testKey}, tools...)
}

func newRigWith(t *testing.T, opts rigOptions, tools ...mcp.Tool) *rig {
	t.Helper()
	database := dbtest.New(t)
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store := allowlist.New(database.DB, opts.lock, logger)
	fake := &fakeMCP{secret: "s3cret-value", tools: tools}
	mux := http.NewServeMux()
	mux.Handle("/mcp", fake.handler())
	client := egresstest.NewClient(t, store, mux, fakeHost)
	service := New(Deps{DB: database.DB, Key: opts.key, Domains: store, Connector: MCPConnector{Trip: client.CallTripper(), PrivateTrip: client.PrivateCallTripper(), Version: "test"}, Logger: logger})

	user, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "issuer", OidcSubject: "ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	admin := auth.Principal{Kind: auth.KindUser, ID: user.ID.String(), Name: "ada", Email: "ada@example.com", Role: auth.RoleAdmin}

	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{database: database, service: service, store: store, fake: fake, admin: admin, logs: logs, router: router, caller: admin}
	gin.SetMode(gin.TestMode)
	r.engine = gin.New()
	var mw gin.HandlerFunc
	mw, r.flush = audittest.Middleware(t, database)
	r.engine.Use(mw)
	group := r.engine.Group("/api/v1", func(c *gin.Context) {
		auth.Authenticated(c.Request.Context(), r.caller, audit.AuthSession, "")
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), r.caller))
	})
	NewHandler(service, logger).Register(auth.NewRoutes(group))
	return r
}

func (r *rig) call(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	r.bodies = append(r.bodies, rec.Body.String())

	check := httptest.NewRequest(method, path, strings.NewReader(body))
	check.Header = req.Header.Clone()
	route, params, err := r.router.FindRoute(check)
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

func expectCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != status || body.Code != code {
		t.Fatalf("answered %d %q, want %d %q: %s", rec.Code, body.Code, status, code, rec.Body)
	}
}

func (r *rig) allow(t *testing.T, hosts ...string) {
	t.Helper()
	for _, h := range hosts {
		p, err := domains.ParsePattern(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.store.Add(t.Context(), r.admin, p); err != nil {
			t.Fatal(err)
		}
	}
}

func registration(uid, secret string) string {
	return `{"uid":"` + uid + `","name":"Docs","url":"https://` + fakeHost + `/mcp","audience":"admins","secret_header":"X-Api-Key","secret":"` + secret + `"}`
}
