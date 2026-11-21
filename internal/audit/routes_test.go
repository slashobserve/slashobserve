package audit

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
)

func TestReadDeclaration(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		want    Declaration
		wantErr string
	}{
		{name: "an action with a path target", raw: map[string]any{"action": "dashboard.update", "target": map[string]any{"kind": "dashboard", "id": "path.id"}}, want: Declaration{Action: "dashboard.update", Target: &TargetRule{Kind: "dashboard", ID: "path.id"}}},
		{name: "an action with a kind only", raw: map[string]any{"action": "dashboard.create", "target": map[string]any{"kind": "dashboard"}}, want: Declaration{Action: "dashboard.create", Target: &TargetRule{Kind: "dashboard"}}},
		{name: "none with a reason", raw: map[string]any{"none": "list read"}, want: Declaration{None: "list read"}},
		{name: "neither", raw: map[string]any{}, wantErr: "neither"},
		{name: "both", raw: map[string]any{"action": "dashboard.update", "none": "x"}, wantErr: "both"},
		{name: "none with a path target", raw: map[string]any{"none": "x", "target": map[string]any{"kind": "dashboard", "id": "path.id"}}, want: Declaration{None: "x", Target: &TargetRule{Kind: "dashboard", ID: "path.id"}}},
		{name: "none with a target that names no id", raw: map[string]any{"none": "x", "target": map[string]any{"kind": "dashboard"}}, wantErr: "path parameter"},
		{name: "a target without a kind", raw: map[string]any{"action": "dashboard.update", "target": map[string]any{"id": "path.id"}}, wantErr: "no kind"},
		{name: "a target id from the body", raw: map[string]any{"action": "dashboard.update", "target": map[string]any{"kind": "dashboard", "id": "body.id"}}, wantErr: "path.<name>"},
		{name: "not an object", raw: "dashboard.update", wantErr: "{action, target}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &openapi3.Operation{Extensions: map[string]any{Extension: tt.raw}}
			got, ok, err := ReadDeclaration(op)
			if !ok {
				t.Fatal("the declaration was not found")
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Action != tt.want.Action || got.None != tt.want.None || (got.Target == nil) != (tt.want.Target == nil) || (got.Target != nil && *got.Target != *tt.want.Target) {
				t.Fatalf("declaration = %+v, want %+v", got, tt.want)
			}
		})
	}
	if _, ok, _ := ReadDeclaration(&openapi3.Operation{}); ok {
		t.Fatal("an operation without the extension reports a declaration")
	}
}

func TestTheEmbeddedDocumentDeclaresRoutesByTemplate(t *testing.T) {
	routes, err := declaredRoutes()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := routes.lookup("post", "/api/v1/dashboards/{id}/versions")
	if !ok || d.Action != "dashboard.update" {
		t.Fatalf("POST /dashboards/{id}/versions = %+v %v, want dashboard.update", d, ok)
	}
	if name, ok := d.Target.PathParam(); !ok || name != "id" {
		t.Fatalf("target parameter = %q %v, want id", name, ok)
	}
	if _, ok := routes.lookup("GET", "/api/v1/dashboards"); ok {
		t.Fatal("a route declared none without a target is in the table")
	}
	if d, ok := routes.lookup("GET", "/api/v1/dashboards/{id}/versions/{version}"); !ok || d.Action != "" || d.Target == nil || d.Target.Kind != "dashboard" {
		t.Fatalf("GET /dashboards/{id}/versions/{version} = %+v %v, want none with a dashboard target", d, ok)
	}
}

func TestOutcomeFor(t *testing.T) {
	onTarget := Declaration{Action: "dashboard.view", Target: &TargetRule{Kind: "dashboard", ID: "path.id"}}
	onKind := Declaration{Action: "dashboard.create", Target: &TargetRule{Kind: "dashboard"}}
	for _, d := range []Declaration{onTarget, onKind, {}} {
		for status, want := range map[int]Outcome{
			200: OutcomeSuccess, 201: OutcomeSuccess, 204: OutcomeSuccess, 304: OutcomeSuccess,
			301: OutcomeUnknown, 302: OutcomeUnknown, 303: OutcomeUnknown, 307: OutcomeUnknown, 308: OutcomeUnknown,
			401: OutcomeDenied, 403: OutcomeDenied, 429: OutcomeDenied,
			400: OutcomeFailed, 409: OutcomeFailed, 422: OutcomeFailed, 500: OutcomeFailed, 503: OutcomeFailed,
		} {
			if got := outcomeFor(status, "", d); got != want {
				t.Errorf("status %d on %+v = %s, want %s", status, d, got, want)
			}
		}
	}
	if got := outcomeFor(404, "not_found", onTarget); got != OutcomeDenied {
		t.Errorf("404 on a route with a target id = %s, want denied", got)
	}
	for _, code := range []string{"not_imported", "version_not_found", ""} {
		if got := outcomeFor(404, code, onTarget); got != OutcomeFailed {
			t.Errorf("404 %q on a route with a target id = %s, want failed", code, got)
		}
	}
	for _, d := range []Declaration{onKind, {}} {
		if got := outcomeFor(404, "not_found", d); got != OutcomeFailed {
			t.Errorf("404 on %+v = %s, want failed", d, got)
		}
	}
}

var routesThatConcludeTheirRedirects = []string{"GET /api/v1/auth/callback"}

func TestEveryDeclaredRouteThatRedirectsConcludesItsOwnResult(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			d, _, err := ReadDeclaration(op)
			if err != nil || d.Action == "" {
				continue
			}
			route := method + " " + APIPrefix + path
			for code := range op.Responses.Map() {
				status, err := strconv.Atoi(code)
				if err == nil && redirected(status) && !slices.Contains(routesThatConcludeTheirRedirects, route) {
					t.Errorf("%s answers %d and declares %s: its handler must call Conclude, then list it here", route, status, d.Action)
				}
			}
		}
	}
}

func TestTraceIDComesFromAValidTraceparentOnly(t *testing.T) {
	for header, want := range map[string]string{
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01": "4bf92f3577b34da6a3ce929d0e0e4736",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01": "",
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01": "",
		"00-4bf92f3577b34da6a3ce929d0e0e473-00f067aa0ba902b7-011": "",
		"garbage": "",
		"":        "",
	} {
		if got := traceID(header); got != want {
			t.Errorf("traceID(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestClientAddrNormalisesWhatTheServerResolved(t *testing.T) {
	for resolved, want := range map[string]string{
		"192.0.2.7":        "192.0.2.7",
		"2001:db8::1":      "2001:db8::1",
		"::ffff:192.0.2.9": "192.0.2.9",
		"":                 "invalid IP",
		"not an address":   "invalid IP",
	} {
		if got := clientAddr(resolved).String(); got != want {
			t.Errorf("clientAddr(%q) = %s, want %s", resolved, got, want)
		}
	}
}

func TestGinPathsBecomeOpenAPITemplates(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v1/dashboards/:id/versions/:version": "/api/v1/dashboards/{id}/versions/{version}",
		"/api/v1/dashboards":                       "/api/v1/dashboards",
		"":                                         "",
	} {
		if got := ginToTemplate(path); got != want {
			t.Errorf("ginToTemplate(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestTheErrorCodeIsReadFromAnErrorResponseOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		status int
		body   string
		want   string
	}{
		{status: http.StatusConflict, body: `{"error":"stale","code":"version_conflict"}`, want: "version_conflict"},
		{status: http.StatusOK, body: `{"code":"not_an_error"}`, want: ""},
		{status: http.StatusInternalServerError, body: `not json`, want: ""},
	} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		capture := &errorCapture{ResponseWriter: c.Writer}
		capture.WriteHeader(tt.status)
		if _, err := capture.WriteString(tt.body); err != nil {
			t.Fatal(err)
		}
		if got := capture.refusal().Code; got != tt.want {
			t.Errorf("status %d body %s: code = %q, want %q", tt.status, tt.body, got, tt.want)
		}
		if rec.Body.String() != tt.body {
			t.Errorf("the response body changed to %q", rec.Body.String())
		}
	}
}
