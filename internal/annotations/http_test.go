package annotations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

const (
	dashboardText  = "0a1b2c3d-0000-4000-8000-000000000001"
	annotationText = "0a1b2c3d-0000-4000-8000-0000000000aa"
)

type fakeAnnotations struct {
	created CreateInput
	listed  ListQuery
	deleted pgtype.UUID
	page    Page
	err     error
}

func (f *fakeAnnotations) Create(_ context.Context, _ auth.Principal, in CreateInput) (Annotation, error) {
	f.created = in
	if f.err != nil {
		return Annotation{}, f.err
	}
	return Annotation{ID: annotationText, DashboardID: in.DashboardID.String(), PanelID: in.PanelID, From: in.From, To: in.To, Text: in.Text, Tags: append([]string{}, in.Tags...), CreatedBy: editor.ID, CreatedByName: editor.Name, CreatedAt: base}, nil
}

func (f *fakeAnnotations) Delete(_ context.Context, _ auth.Principal, id pgtype.UUID) error {
	f.deleted = id
	return f.err
}

func (f *fakeAnnotations) List(_ context.Context, _ auth.Principal, q ListQuery) (Page, error) {
	f.listed = q
	return f.page, f.err
}

func serve(t *testing.T, fake *fakeAnnotations, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger, _ := logtest.New()
	router := gin.New()
	NewHandler(fake, logger).Register(auth.NewRoutes(router.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), editor))
	})))
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func TestCreateParsesTheBodyAndAnswersTheAnnotation(t *testing.T) {
	fake := &fakeAnnotations{}
	rec := serve(t, fake, http.MethodPost, "/api/v1/annotations",
		`{"dashboard_id":"`+dashboardText+`","panel_id":"latency","time_from":"2026-11-01T12:00:00+02:00","time_to":"2026-11-01T10:30:00.5Z","text":"deploy v2","tags":["deploy"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/api/v1/annotations/"+annotationText {
		t.Errorf("Location %q", got)
	}
	in := fake.created
	if in.DashboardID.String() != dashboardText || in.PanelID != "latency" || !in.From.Equal(base) || in.To == nil || !in.To.Equal(base.Add(30*time.Minute+500*time.Millisecond)) {
		t.Errorf("service got %+v", in)
	}
	body := decode[map[string]any](t, rec)
	want := map[string]any{
		"id": annotationText, "dashboard_id": dashboardText, "panel_id": "latency",
		"time_from": "2026-11-01T10:00:00Z", "time_to": "2026-11-01T10:30:00.5Z", "text": "deploy v2",
		"tags": []any{"deploy"}, "created_by": editor.ID, "created_by_name": editor.Name, "created_at": "2026-11-01T10:00:00Z",
	}
	for k, v := range want {
		if got, err := json.Marshal(body[k]); err != nil || !slices.Equal(got, must(json.Marshal(v))) {
			t.Errorf("%s = %v, want %v", k, body[k], v)
		}
	}
	if len(body) != len(want) {
		t.Errorf("fields %v, want exactly %v", body, want)
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func TestCreateGlobalAnswersNullDashboardAndPanel(t *testing.T) {
	fake := &fakeAnnotations{}
	rec := serve(t, fake, http.MethodPost, "/api/v1/annotations", `{"text":"deploy"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if fake.created.DashboardID.Valid || !fake.created.From.IsZero() {
		t.Errorf("service got %+v, want no dashboard and no time", fake.created)
	}
	body := decode[map[string]any](t, rec)
	for _, k := range []string{"dashboard_id", "panel_id", "time_to"} {
		if v, ok := body[k]; !ok || v != nil {
			t.Errorf("%s = %v, want null", k, v)
		}
	}
}

func TestCreateRefusesBadRequests(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		err    error
		status int
		code   string
	}{
		{"not JSON", `{`, nil, http.StatusBadRequest, "invalid_request"},
		{"unknown field", `{"text":"x","colour":"red"}`, nil, http.StatusBadRequest, "invalid_request"},
		{"dashboard id not a UUID", `{"text":"x","dashboard_id":"checkout"}`, nil, http.StatusBadRequest, "invalid_request"},
		{"time not RFC 3339", `{"text":"x","time_from":"yesterday"}`, nil, http.StatusUnprocessableEntity, "invalid_time"},
		{"end not RFC 3339", `{"text":"x","time_to":"1700000000"}`, nil, http.StatusUnprocessableEntity, "invalid_time"},
		{"too large", `{"text":"` + strings.Repeat("x", maxBodyBytes) + `"}`, nil, http.StatusRequestEntityTooLarge, "too_large"},
		{"invalid input", `{"text":""}`, &InvalidError{Code: "invalid_text", Message: messageInvalidText}, http.StatusUnprocessableEntity, "invalid_text"},
		{"dashboard not found", `{"text":"x","dashboard_id":"` + dashboardText + `"}`, ErrNotFound, http.StatusNotFound, "not_found"},
		{"level too low", `{"text":"x"}`, ErrForbidden, http.StatusForbidden, "forbidden_role"},
		{"database down", `{"text":"x"}`, errors.New("connection refused"), http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(t, &fakeAnnotations{err: tc.err}, http.MethodPost, "/api/v1/annotations", tc.body)
			if got := decode[errorBody](t, rec); rec.Code != tc.status || got.Code != tc.code {
				t.Errorf("status %d code %q, want %d %q", rec.Code, got.Code, tc.status, tc.code)
			}
		})
	}
}

func TestListParsesTheQuery(t *testing.T) {
	after := httpapi.EncodeTimeCursor(base, annotationText)
	fake := &fakeAnnotations{}
	rec := serve(t, fake, http.MethodGet, "/api/v1/annotations?dashboard_id="+dashboardText+"&from=2026-11-01T09:00:00Z&to=2026-11-01T11:00:00Z&tags=deploy,checkout&limit=20&cursor="+after, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	q := fake.listed
	if q.DashboardID.String() != dashboardText || q.From == nil || !q.From.Equal(base.Add(-time.Hour)) || q.To == nil || !q.To.Equal(base.Add(time.Hour)) {
		t.Errorf("query %+v", q)
	}
	if !slices.Equal(q.Tags, []string{"deploy", "checkout"}) || q.Limit != 20 || q.After == nil || !q.After.Time.Equal(base) || q.After.ID.String() != annotationText {
		t.Errorf("query %+v", q)
	}
}

func TestListDefaultsToOneHundredAndNoFilters(t *testing.T) {
	fake := &fakeAnnotations{}
	rec := serve(t, fake, http.MethodGet, "/api/v1/annotations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if q := fake.listed; q.Limit != defaultLimit || q.DashboardID.Valid || q.From != nil || q.To != nil || q.Tags != nil || q.After != nil {
		t.Errorf("query %+v", q)
	}
	if got := rec.Body.String(); got != `{"annotations":[],"next_cursor":null}` {
		t.Errorf("body %s", got)
	}
}

func TestListAnswersTheNextCursor(t *testing.T) {
	next, err := httpapiCursor(base, annotationText)
	if err != nil {
		t.Fatal(err)
	}
	to := base.Add(time.Minute)
	fake := &fakeAnnotations{page: Page{
		Annotations: []Annotation{{ID: annotationText, From: base, To: &to, Text: "deploy", Tags: []string{}, CreatedAt: base}},
		Next:        next,
	}}
	rec := serve(t, fake, http.MethodGet, "/api/v1/annotations?limit=1", "")
	body := decode[listJSON](t, rec)
	if len(body.Annotations) != 1 || body.Annotations[0].TimeTo == nil || *body.Annotations[0].TimeTo != "2026-11-01T10:01:00Z" {
		t.Errorf("annotations %+v", body.Annotations)
	}
	if body.NextCursor == nil || *body.NextCursor != httpapi.EncodeTimeCursor(base, annotationText) {
		t.Errorf("next_cursor %v", body.NextCursor)
	}
}

func TestListAnswersTheAuthorNameOrNull(t *testing.T) {
	fake := &fakeAnnotations{page: Page{Annotations: []Annotation{
		{ID: annotationText, From: base, Text: "deploy", Tags: []string{}, CreatedBy: editor.ID, CreatedByName: "Eve", CreatedAt: base},
		{ID: annotationText, From: base, Text: "gone", Tags: []string{}, CreatedAt: base},
	}}}
	rec := serve(t, fake, http.MethodGet, "/api/v1/annotations", "")
	var body struct {
		Annotations []map[string]any `json:"annotations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Annotations) != 2 {
		t.Fatalf("annotations %+v", body.Annotations)
	}
	if got := body.Annotations[0]["created_by_name"]; got != "Eve" {
		t.Errorf("created_by_name %v, want Eve", got)
	}
	if got, ok := body.Annotations[1]["created_by_name"]; !ok || got != nil {
		t.Errorf("created_by_name %v (present %v), want null", got, ok)
	}
}

func httpapiCursor(t time.Time, id string) (*httpapi.TimeCursor, error) {
	c, ok := httpapi.DecodeTimeCursor(httpapi.EncodeTimeCursor(t, id))
	if !ok {
		return nil, errors.New("cursor does not round-trip")
	}
	return c, nil
}

func TestListRefusesBadQueries(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		err    error
		status int
		code   string
	}{
		{"dashboard id not a UUID", "dashboard_id=checkout", nil, http.StatusBadRequest, "invalid_request"},
		{"from not RFC 3339", "from=now-6h", nil, http.StatusBadRequest, "invalid_request"},
		{"to before from", "from=2026-11-01T10:00:00Z&to=2026-11-01T09:00:00Z", nil, http.StatusBadRequest, "invalid_request"},
		{"uppercase tag", "tags=Deploy", nil, http.StatusBadRequest, "invalid_request"},
		{"empty tag between commas", "tags=deploy,,checkout", nil, http.StatusBadRequest, "invalid_request"},
		{"limit zero", "limit=0", nil, http.StatusBadRequest, "invalid_request"},
		{"limit over the ceiling", "limit=501", nil, http.StatusBadRequest, "invalid_request"},
		{"cursor garbage", "cursor=nope", nil, http.StatusBadRequest, "invalid_request"},
		{"dashboard not visible", "dashboard_id=" + dashboardText, ErrNotFound, http.StatusNotFound, "not_found"},
		{"database down", "", errors.New("connection refused"), http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(t, &fakeAnnotations{err: tc.err}, http.MethodGet, "/api/v1/annotations?"+tc.query, "")
			if got := decode[errorBody](t, rec); rec.Code != tc.status || got.Code != tc.code {
				t.Errorf("status %d code %q, want %d %q", rec.Code, got.Code, tc.status, tc.code)
			}
		})
	}
}

func TestDeleteAnswersNoContentOrTheRefusal(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		err    error
		status int
		code   string
	}{
		{"deleted", annotationText, nil, http.StatusNoContent, ""},
		{"id not a UUID", "deploy", nil, http.StatusBadRequest, "invalid_request"},
		{"not found", annotationText, ErrNotFound, http.StatusNotFound, "not_found"},
		{"not the creator", annotationText, ErrNotCreator, http.StatusForbidden, "not_creator"},
		{"database down", annotationText, errors.New("connection refused"), http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAnnotations{err: tc.err}
			rec := serve(t, fake, http.MethodDelete, "/api/v1/annotations/"+tc.id, "")
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.code == "" {
				if fake.deleted.String() != annotationText || rec.Body.Len() != 0 {
					t.Errorf("deleted %v, body %q", fake.deleted, rec.Body.String())
				}
				return
			}
			if got := decode[errorBody](t, rec); got.Code != tc.code {
				t.Errorf("code %q, want %q", got.Code, tc.code)
			}
		})
	}
}
