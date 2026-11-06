package provision_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/provision"
)

func TestHTTPResponsesMatchTheAPIDocument(t *testing.T) {
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		svc    *fakeService
		role   auth.Role
		method string
		path   string
	}{
		{"status", &fakeService{report: sampleReport()}, auth.RoleAdmin, http.MethodGet, ""},
		{"status when off", &fakeService{}, auth.RoleAdmin, http.MethodGet, ""},
		{"status for an editor", &fakeService{}, auth.RoleEditor, http.MethodGet, ""},
		{"run", &fakeService{report: sampleReport()}, auth.RoleAdmin, http.MethodPost, "/run"},
		{"run while off", &fakeService{runErr: provision.ErrOff}, auth.RoleAdmin, http.MethodPost, "/run"},
		{"run on an unreadable directory", &fakeService{runErr: provision.ErrDirUnreadable}, auth.RoleAdmin, http.MethodPost, "/run"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveHTTP(t, tc.svc, tc.role, tc.method, tc.path)
			req := httptest.NewRequest(tc.method, "/api/v1/provision"+tc.path, nil)
			route, params, err := router.FindRoute(req)
			if err != nil {
				t.Fatalf("no documented operation: %v", err)
			}
			options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true}
			input := &openapi3filter.ResponseValidationInput{
				RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route, Options: options},
				Status:                 rec.Code,
				Header:                 rec.Header(),
				Options:                options,
			}
			input.SetBodyBytes(rec.Body.Bytes())
			if err := openapi3filter.ValidateResponse(t.Context(), input); err != nil {
				t.Fatalf("status %d body %s breaks the document: %v", rec.Code, rec.Body.String(), err)
			}
		})
	}
}

type fakeService struct {
	runErr    error
	reportErr error
	report    provision.Report
	runs      int
}

func (f *fakeService) Run(context.Context) error {
	f.runs++
	return f.runErr
}

func (f *fakeService) Report(context.Context) (provision.Report, error) {
	return f.report, f.reportErr
}

func serveHTTP(t *testing.T, svc *fakeService, role auth.Role, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		p := auth.Principal{Kind: auth.KindUser, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "Ada", Role: role}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
	})
	provision.NewHandler(svc, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/provision"+path, nil))
	return rec
}

func sampleReport() provision.Report {
	at := time.Date(2026, 11, 6, 9, 30, 0, 0, time.UTC)
	return provision.Report{
		Dir: "/provision",
		Dashboards: []provision.Provisioned{
			{Path: "shop/checkout.json", DashboardID: "0a1b2c3d-0000-4000-8000-000000000001", Folder: "shop", UpdatedAt: at},
			{Path: "root.json", DashboardID: "0a1b2c3d-0000-4000-8000-000000000002", Folder: "General", UpdatedAt: at, LastError: "the spec is invalid: /title: required"},
		},
		Errors: []provision.FileError{{Path: "broken.json", Message: "the file cannot be read", SeenAt: at}},
	}
}

const sampleBody = `{"dir":"/provision","dashboards":[` +
	`{"path":"shop/checkout.json","dashboard_id":"0a1b2c3d-0000-4000-8000-000000000001","folder":"shop","updated_at":"2026-11-06T09:30:00Z","last_error":null},` +
	`{"path":"root.json","dashboard_id":"0a1b2c3d-0000-4000-8000-000000000002","folder":"General","updated_at":"2026-11-06T09:30:00Z","last_error":"the spec is invalid: /title: required"}],` +
	`"errors":[{"path":"broken.json","message":"the file cannot be read","seen_at":"2026-11-06T09:30:00Z"}]}`

func TestHTTPStatusAnswersTheReport(t *testing.T) {
	cases := []struct {
		name   string
		report provision.Report
		want   string
	}{
		{"with dashboards and errors", sampleReport(), sampleBody},
		{"off and empty", provision.Report{}, `{"dir":null,"dashboards":[],"errors":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{report: tc.report}
			rec := serveHTTP(t, svc, auth.RoleAdmin, http.MethodGet, "")
			if rec.Code != http.StatusOK || rec.Body.String() != tc.want {
				t.Fatalf("status %d body %s, want %s", rec.Code, rec.Body.String(), tc.want)
			}
			if svc.runs != 0 {
				t.Fatalf("GET ran provisioning %d times", svc.runs)
			}
		})
	}
}

func TestHTTPRunRunsOnceAndAnswersTheReport(t *testing.T) {
	svc := &fakeService{report: sampleReport()}
	rec := serveHTTP(t, svc, auth.RoleAdmin, http.MethodPost, "/run")
	if rec.Code != http.StatusOK || rec.Body.String() != sampleBody || svc.runs != 1 {
		t.Fatalf("status %d, runs %d, body %s", rec.Code, svc.runs, rec.Body.String())
	}
}

func TestHTTPRefusalsAndFailures(t *testing.T) {
	cases := []struct {
		name   string
		svc    *fakeService
		role   auth.Role
		method string
		path   string
		status int
		code   string
	}{
		{"an editor cannot read the status", &fakeService{}, auth.RoleEditor, http.MethodGet, "", http.StatusForbidden, "forbidden_role"},
		{"an editor cannot run", &fakeService{}, auth.RoleEditor, http.MethodPost, "/run", http.StatusForbidden, "forbidden_role"},
		{"run while off", &fakeService{runErr: provision.ErrOff}, auth.RoleAdmin, http.MethodPost, "/run", http.StatusConflict, "provisioning_off"},
		{"run on an unreadable directory", &fakeService{runErr: fmt.Errorf("%w: missing", provision.ErrDirUnreadable)}, auth.RoleAdmin, http.MethodPost, "/run", http.StatusInternalServerError, "provision_dir_unreadable"},
		{"run fails", &fakeService{runErr: errors.New("database down")}, auth.RoleAdmin, http.MethodPost, "/run", http.StatusInternalServerError, "internal"},
		{"status fails", &fakeService{reportErr: errors.New("database down")}, auth.RoleAdmin, http.MethodGet, "", http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveHTTP(t, tc.svc, tc.role, tc.method, tc.path)
			var body struct {
				Error string `json:"error"`
				Code  string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q: %v", rec.Body.String(), err)
			}
			if rec.Code != tc.status || body.Code != tc.code || body.Error == "" {
				t.Fatalf("status %d body %s, want %d %s", rec.Code, rec.Body.String(), tc.status, tc.code)
			}
			if body.Error == "database down" {
				t.Fatal("the response echoes the error text")
			}
		})
	}
}
