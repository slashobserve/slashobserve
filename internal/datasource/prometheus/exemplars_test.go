package prometheus_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

type exemplarServer struct {
	mu     sync.Mutex
	method string
	path   string
	form   url.Values
}

func (s *exemplarServer) request() (string, string, url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.method, s.path, s.form
}

func exemplarDS(t *testing.T, status int, body string) (*prometheus.Datasource, *exemplarServer) {
	t.Helper()
	seen := &exemplarServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		seen.mu.Lock()
		seen.method, seen.path, seen.form = r.Method, r.URL.Path, form
		seen.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	ds, err := prometheus.New(prometheus.Options{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, seen
}

func exemplarRange() datasource.TimeRange {
	return datasource.TimeRange{From: time.Unix(1_700_000_000, 0), To: time.Unix(1_700_003_600, 0)}
}

const exemplarBody = `{"status":"success","data":[
  {"seriesLabels":{"__name__":"http_request_duration_seconds_bucket","job":"api","le":"0.5"},
   "exemplars":[
     {"labels":{"trace_id":"b","span":"s2"},"value":"0.7","timestamp":1700000120.5},
     {"labels":{"trace_id":"a"},"value":"NaN","timestamp":1700000060.25}
   ]},
  {"seriesLabels":{"__name__":"http_request_duration_seconds_bucket","job":"api","le":"0.1"},
   "exemplars":[{"labels":{"trace_id":"c"},"value":"0.05","timestamp":1700000010}]},
  {"seriesLabels":{"__name__":"empty"},"exemplars":[]}
]}`

func TestQueryExemplarsRequest(t *testing.T) {
	ds, seen := exemplarDS(t, http.StatusOK, `{"status":"success","data":[]}`)
	r := exemplarRange()
	q := datasource.Query{
		Text:    `histogram_quantile(0.9, rate(http_request_duration_seconds_bucket{job="api"}[$__rate_interval]))`,
		Filters: []datasource.AdhocFilter{{Key: "pod", Op: "=", Value: "web-0"}},
	}
	if _, err := ds.QueryExemplars(context.Background(), q, r, 30*time.Second); err != nil {
		t.Fatalf("QueryExemplars: %v", err)
	}
	method, path, form := seen.request()
	if method != http.MethodPost || path != "/api/v1/query_exemplars" {
		t.Fatalf("request = %s %s, want POST /api/v1/query_exemplars", method, path)
	}
	wantQuery := `histogram_quantile(0.9, rate(http_request_duration_seconds_bucket{job="api",pod="web-0"}[1m]))`
	if got := form.Get("query"); got != wantQuery {
		t.Errorf("query = %q, want %q", got, wantQuery)
	}
	if form.Get("start") != "1700000000.000" || form.Get("end") != "1700003600.000" {
		t.Errorf("start/end = %q/%q, want 1700000000.000/1700003600.000", form.Get("start"), form.Get("end"))
	}
}

func TestQueryExemplarsFrames(t *testing.T) {
	ds, _ := exemplarDS(t, http.StatusOK, exemplarBody)
	frames, err := ds.QueryExemplars(context.Background(), datasource.Query{Text: "http_request_duration_seconds_bucket"}, exemplarRange(), time.Minute)
	if err != nil {
		t.Fatalf("QueryExemplars: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2 (a series without exemplars has none)", len(frames))
	}
	first, second := frames[0], frames[1]
	if first.Name != "http_request_duration_seconds_bucket" || !reflect.DeepEqual(first.Labels, map[string]string{"job": "api", "le": "0.1"}) {
		t.Errorf("frames are not ordered by series labels: first = %q %v", first.Name, first.Labels)
	}
	wantNames := []string{"Time", "Value", "span", "trace_id"}
	var gotNames []string
	for _, f := range second.Fields {
		gotNames = append(gotNames, f.Name)
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("field names = %v, want %v", gotNames, wantNames)
	}
	wantTypes := []datasource.FieldType{datasource.FieldTime, datasource.FieldNumber, datasource.FieldString, datasource.FieldString}
	for i, f := range second.Fields {
		if f.Type != wantTypes[i] {
			t.Errorf("field %q type = %q, want %q", f.Name, f.Type, wantTypes[i])
		}
	}
	wantTimes := []any{time.UnixMilli(1_700_000_060_250).UTC(), time.UnixMilli(1_700_000_120_500).UTC()}
	if !reflect.DeepEqual(second.Fields[0].Values, wantTimes) {
		t.Errorf("times = %v, want %v (oldest first)", second.Fields[0].Values, wantTimes)
	}
	if !reflect.DeepEqual(second.Fields[1].Values, []any{nil, 0.7}) {
		t.Errorf("values = %v, want [nil 0.7] (NaN becomes nil)", second.Fields[1].Values)
	}
	if !reflect.DeepEqual(second.Fields[2].Values, []any{"", "s2"}) {
		t.Errorf("span = %v, want an empty string where the exemplar lacks the key", second.Fields[2].Values)
	}
	if !reflect.DeepEqual(second.Fields[3].Values, []any{"a", "b"}) {
		t.Errorf("trace_id = %v, want [a b]", second.Fields[3].Values)
	}
}

func TestQueryExemplarsEmptyData(t *testing.T) {
	for name, body := range map[string]string{
		"empty array": `{"status":"success","data":[]}`,
		"null data":   `{"status":"success","data":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			ds, _ := exemplarDS(t, http.StatusOK, body)
			frames, err := ds.QueryExemplars(context.Background(), datasource.Query{Text: "up"}, exemplarRange(), time.Minute)
			if err != nil || len(frames) != 0 {
				t.Fatalf("QueryExemplars = %v, %v; want no frames and no error", frames, err)
			}
		})
	}
}

func TestQueryExemplarsMapsUpstreamErrorsLikeQueryRange(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"bad query", http.StatusBadRequest, `{"status":"error","errorType":"bad_data","error":"parse error"}`, datasource.ErrInvalidQuery},
		{"unauthorized", http.StatusUnauthorized, `{}`, datasource.ErrAuthFailed},
		{"forbidden", http.StatusForbidden, `{}`, datasource.ErrAuthFailed},
		{"rate limited", http.StatusTooManyRequests, `{}`, datasource.ErrUnavailable},
		{"server error", http.StatusInternalServerError, `not json`, datasource.ErrUnavailable},
		{"upstream timeout", http.StatusServiceUnavailable, `{"status":"error","errorType":"timeout","error":"timed out"}`, datasource.ErrTimeout},
		{"unreadable data", http.StatusOK, `{"status":"success","data":{"not":"a list"}}`, datasource.ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, _ := exemplarDS(t, tc.status, tc.body)
			r := exemplarRange()
			q := datasource.Query{Text: "up"}
			_, rangeErr := ds.QueryRange(context.Background(), q, r, time.Minute)
			_, err := ds.QueryExemplars(context.Background(), q, r, time.Minute)
			if !errors.Is(err, tc.want) {
				t.Fatalf("QueryExemplars error = %v, want %v", err, tc.want)
			}
			if tc.name != "unreadable data" && !errors.Is(rangeErr, tc.want) {
				t.Errorf("QueryRange error = %v, want the same kind %v", rangeErr, tc.want)
			}
		})
	}
}

func TestQueryExemplarsRejectsBadInput(t *testing.T) {
	ds, seen := exemplarDS(t, http.StatusOK, `{"status":"success","data":[]}`)
	ctx := context.Background()
	r := exemplarRange()
	cases := map[string]func() error{
		"step under 1ms": func() error {
			_, err := ds.QueryExemplars(ctx, datasource.Query{Text: "up"}, r, time.Microsecond)
			return err
		},
		"reversed range": func() error {
			_, err := ds.QueryExemplars(ctx, datasource.Query{Text: "up"}, datasource.TimeRange{From: r.To, To: r.From}, time.Minute)
			return err
		},
		"unparsable query": func() error {
			_, err := ds.QueryExemplars(ctx, datasource.Query{Text: "up{"}, r, time.Minute)
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			if err := run(); !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Fatalf("want ErrInvalidQuery, got %v", err)
			}
		})
	}
	if _, path, _ := seen.request(); path != "" {
		t.Errorf("an invalid request reached the upstream: %s", path)
	}
}
