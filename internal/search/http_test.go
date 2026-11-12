package search

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
)

type documented struct {
	*rig
	engine *gin.Engine
	router routers.Router
	who    auth.Principal
}

func newDocumented(t *testing.T) *documented {
	t.Helper()
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	d := &documented{rig: newRig(t), engine: gin.New(), router: router, who: admin}
	NewHandler(d.service, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(d.engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), d.who))
	})))
	return d
}

func (d *documented) get(t *testing.T, by auth.Principal, query string, want int) *httptest.ResponseRecorder {
	t.Helper()
	d.who = by
	request := func() *http.Request {
		return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/search?"+query, nil)
	}
	rec := httptest.NewRecorder()
	d.engine.ServeHTTP(rec, request())
	if rec.Code != want {
		t.Fatalf("GET %s answered %d, want %d: %s", query, rec.Code, want, rec.Body)
	}
	check := request()
	route, params, err := d.router.FindRoute(check)
	if err != nil {
		t.Fatalf("GET /search is not documented: %v", err)
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, SkipSettingDefaults: true}
	input := &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options}
	check.Body = io.NopCloser(bytes.NewReader(nil))
	response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
	response.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(t.Context(), response); err != nil {
		t.Fatalf("GET %s answered %d against the document: %v\n%s", query, rec.Code, err, rec.Body)
	}
	return rec
}

type listBody struct {
	Results []struct {
		Kind     string  `json:"kind"`
		Title    string  `json:"title"`
		URL      *string `json:"url"`
		ThreadID *string `json:"thread_id"`
		Snippet  []struct {
			Text  string `json:"text"`
			Match bool   `json:"match"`
		} `json:"snippet"`
		Folder *struct {
			Title string `json:"title"`
		} `json:"folder"`
		Target *struct {
			Kind string `json:"kind"`
		} `json:"target"`
	} `json:"results"`
	NextCursor *string `json:"next_cursor"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) listBody {
	t.Helper()
	var body listBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	return body
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) (code, message string) {
	t.Helper()
	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code == "" {
		t.Fatalf("no error code in %s", rec.Body)
	}
	return body.Code, body.Error
}

func TestSearchRouteMatchesTheDocument(t *testing.T) {
	d := newDocumented(t)
	dashboard := d.dashboard(t, "Heron overview", mustUUID(t, d.text(t, `SELECT general_folder_id()::text`)), "")
	d.finding(t, findingIn{title: "heron finding", markdown: "the heron is back", dashboard: dashboard})
	d.comment(t, KindDashboard, dashboard, "a heron comment")
	d.annotation(t, "", "heron global")

	t.Run("a page carries every field and a cursor until the end", func(t *testing.T) {
		first := decode(t, d.get(t, admin, "q=heron&limit=2", http.StatusOK))
		if len(first.Results) != 2 || first.NextCursor == nil {
			t.Fatalf("first page %+v", first)
		}
		last := decode(t, d.get(t, admin, "q=heron&limit=2&cursor="+url.QueryEscape(*first.NextCursor), http.StatusOK))
		if len(last.Results) != 2 || last.NextCursor != nil {
			t.Fatalf("last page %+v", last)
		}
	})

	t.Run("a comment names what it is on and a global annotation has no url", func(t *testing.T) {
		body := decode(t, d.get(t, admin, "q=heron", http.StatusOK))
		sawComment, sawGlobal := false, false
		for _, r := range body.Results {
			switch r.Kind {
			case "comment":
				sawComment = r.Target != nil && r.Target.Kind == "dashboard" && r.ThreadID != nil && r.URL != nil
			case "annotation":
				sawGlobal = r.URL == nil && r.Folder != nil && r.Folder.Title == "General"
			}
		}
		if !sawComment || !sawGlobal {
			t.Fatalf("comment %v, global annotation %v in %+v", sawComment, sawGlobal, body)
		}
	})

	t.Run("kind repeats to search several kinds", func(t *testing.T) {
		body := decode(t, d.get(t, admin, "q=heron&kind=finding&kind=comment", http.StatusOK))
		kinds := map[string]int{}
		for _, r := range body.Results {
			kinds[r.Kind]++
		}
		if len(body.Results) != 2 || kinds["finding"] != 1 || kinds["comment"] != 1 {
			t.Fatalf("kinds %v", kinds)
		}
	})

	t.Run("a snippet marks the matched word", func(t *testing.T) {
		body := decode(t, d.get(t, admin, "q=heron&kind=finding", http.StatusOK))
		if len(body.Results) != 1 || len(body.Results[0].Snippet) == 0 {
			t.Fatalf("body %+v", body)
		}
		marked := false
		for _, s := range body.Results[0].Snippet {
			marked = marked || (s.Match && s.Text == "heron")
		}
		if !marked {
			t.Fatalf("snippet %+v", body.Results[0].Snippet)
		}
	})

	t.Run("no match is an empty list", func(t *testing.T) {
		body := decode(t, d.get(t, admin, "q=nothingmatchesthis", http.StatusOK))
		if body.Results == nil || len(body.Results) != 0 || body.NextCursor != nil {
			t.Fatalf("body %+v", body)
		}
	})

	t.Run("a key with the Viewer role is accepted", func(t *testing.T) {
		d.get(t, keyT, "q=heron", http.StatusOK)
	})
}

func TestSearchRouteAnswersEveryError(t *testing.T) {
	d := newDocumented(t)
	cases := []struct {
		name    string
		by      auth.Principal
		query   string
		status  int
		code    string
		message string
	}{
		{"a missing query", admin, "", http.StatusBadRequest, "invalid_request", "q must be 1 to 200 characters"},
		{"a query with no word", admin, "q=--", http.StatusBadRequest, "invalid_request", "q needs a word to search for"},
		{"a control character", admin, "q=a%00b", http.StatusBadRequest, "invalid_request", "q must not contain control characters"},
		{"an unknown kind", admin, "q=heron&kind=folder", http.StatusBadRequest, "invalid_request", "kind must be one of dashboard, finding, annotation, observation, comment, saved_search"},
		{"a limit above the ceiling", admin, "q=heron&limit=51", http.StatusBadRequest, "invalid_request", "limit must be an integer from 1 to 50"},
		{"a limit of zero", admin, "q=heron&limit=0", http.StatusBadRequest, "invalid_request", "limit must be an integer from 1 to 50"},
		{"a cursor that is not ours", admin, "q=heron&cursor=garbage", http.StatusBadRequest, "invalid_request", "cursor is not valid"},
		{"a caller without a usable id", auth.Principal{Kind: auth.KindUser, ID: "not-a-uuid", Role: auth.RoleViewer}, "q=heron", http.StatusForbidden, "forbidden_role", ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			code, message := errorCode(t, d.get(t, tt.by, tt.query, tt.status))
			if code != tt.code || tt.message != "" && message != tt.message {
				t.Fatalf("code %q message %q, want %q %q", code, message, tt.code, tt.message)
			}
		})
	}
}
