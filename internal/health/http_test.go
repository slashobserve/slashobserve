package health_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

type fakeJobs struct {
	mu    sync.Mutex
	names []string
}

func (f *fakeJobs) RunNow(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names = append(f.names, name)
	return nil
}

func serve(t *testing.T, svc *health.Service, repairs *health.Repairs, jobs health.Jobs, by auth.Principal, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})
	h := health.NewHandler(t.Context(), svc, repairs, jobs)
	h.Register(auth.NewRoutes(group))
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	h.Wait()
	return rec
}

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
	r := newRig(t)
	empty := r.create(t, boardSpec("Empty", "empty_series"))
	r.create(t, boardSpec("Twin", "up"))
	r.create(t, boardSpec("twin", "up"))
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	first, err := r.svc.List(t.Context(), r.viewer, health.ListParams{Limit: 1})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page cursor %q: %v", first.NextCursor, err)
	}
	jobs := &fakeJobs{}
	admin := r.editor
	admin.Role = auth.RoleAdmin
	cases := []struct {
		name         string
		repairs      *health.Repairs
		by           auth.Principal
		method, path string
		body         string
		status       int
	}{
		{"list", r.repairs, r.editor, http.MethodGet, "/health", "", http.StatusOK},
		{"list by kind", r.repairs, r.viewer, http.MethodGet, "/health?kind=duplicate_title&limit=1", "", http.StatusOK},
		{"list a later page", r.repairs, r.viewer, http.MethodGet, "/health?limit=1&cursor=" + first.NextCursor, "", http.StatusOK},
		{"list with an old cursor", r.repairs, r.viewer, http.MethodGet, "/health?cursor=" + httpapi.EncodeTimeCursor(time.Now(), empty.String()), "", http.StatusBadRequest},
		{"list with an unknown kind", r.repairs, r.viewer, http.MethodGet, "/health?kind=broken", "", http.StatusBadRequest},
		{"list in an unknown folder", r.repairs, r.viewer, http.MethodGet, "/health?folder_id=0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61", "", http.StatusNotFound},
		{"repair", r.repairs, r.editor, http.MethodPost, "/health/repairs", `{"dashboard_ids":["` + empty.String() + `"]}`, http.StatusAccepted},
		{"repair with no ids", r.repairs, r.editor, http.MethodPost, "/health/repairs", `{"dashboard_ids":[]}`, http.StatusBadRequest},
		{"repair as a viewer", r.repairs, r.viewer, http.MethodPost, "/health/repairs", `{"dashboard_ids":["` + empty.String() + `"]}`, http.StatusForbidden},
		{"repair without an agent", r.noAgent, r.editor, http.MethodPost, "/health/repairs", `{"dashboard_ids":["` + empty.String() + `"]}`, http.StatusServiceUnavailable},
		{"list after a repair", r.repairs, r.viewer, http.MethodGet, "/health", "", http.StatusOK},
		{"run", r.repairs, admin, http.MethodPost, "/health/run", "", http.StatusAccepted},
		{"run as an editor", r.repairs, r.editor, http.MethodPost, "/health/run", "", http.StatusForbidden},
		{"view", r.repairs, r.viewer, http.MethodPost, "/dashboards/" + empty.String() + "/views", "", http.StatusNoContent},
		{"view by a key", r.repairs, r.key, http.MethodPost, "/dashboards/" + empty.String() + "/views", "", http.StatusNoContent},
		{"view of a missing dashboard", r.repairs, r.viewer, http.MethodPost, "/dashboards/0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61/views", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(t, r.svc, tc.repairs, jobs, tc.by, tc.method, tc.path, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			req := httptest.NewRequest(tc.method, "/api/v1"+tc.path, nil)
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
	if len(jobs.names) != 1 || jobs.names[0] != "dashboard_health" {
		t.Errorf("run started jobs %v", jobs.names)
	}
}

func TestHTTPListCarriesTheSummaryOnlyOnTheFirstPageAndNullRepairs(t *testing.T) {
	r := newRig(t)
	r.create(t, boardSpec("Twin", "up"))
	r.create(t, boardSpec("twin", "up"))
	list := func(path string) map[string]json.RawMessage {
		t.Helper()
		rec := serve(t, r.svc, r.repairs, &fakeJobs{}, r.viewer, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", path, rec.Code, rec.Body.String())
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	body := list("/health?limit=1")
	var summary map[string]int
	if err := json.Unmarshal(body["summary"], &summary); err != nil {
		t.Fatalf("summary %s: %v", body["summary"], err)
	}
	fields := []string{"dashboards", "duplicate_titles", "failing", "no_data", "query_failed", "unopened", "variable_failed"}
	got := make([]string, 0, len(summary))
	for k := range summary {
		got = append(got, k)
	}
	slices.Sort(got)
	if !slices.Equal(got, fields) || summary["dashboards"] != 2 || summary["duplicate_titles"] != 2 {
		t.Errorf("summary = %v", summary)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body["dashboards"], &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || string(items[0]["repair"]) != "null" {
		t.Errorf("dashboards = %s, want one with a null repair", body["dashboards"])
	}

	var cursor string
	if err := json.Unmarshal(body["next_cursor"], &cursor); err != nil || cursor == "" {
		t.Fatalf("next_cursor %s: %v", body["next_cursor"], err)
	}
	if later := list("/health?limit=1&cursor=" + cursor); string(later["summary"]) != "null" {
		t.Errorf("later page summary = %s, want null", later["summary"])
	}
}
