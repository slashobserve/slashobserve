package prometheus_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

func warningDatasource(t *testing.T, body string) *prometheus.Datasource {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	ds, err := prometheus.New(prometheus.Options{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func TestAnAnswerWithWarningsIsPartialAndCarriesTheirText(t *testing.T) {
	ds := warningDatasource(t, `{"status":"success","warnings":["store a is down"],"data":{"resultType":"vector","result":[{"metric":{"__name__":"up"},"value":[1700000000,"1"]}]}}`)

	frames, err := ds.QueryInstant(t.Context(), datasource.Query{Text: "up"}, datasource.TimeRange{From: time.Now().Add(-time.Minute), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	meta := datasource.MergeMeta(frames)
	if !meta.Partial || meta.Warning != "store a is down" {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestAnEmptyAnswerWithWarningsStillSaysItIsPartial(t *testing.T) {
	ds := warningDatasource(t, `{"status":"success","warnings":["store a is down"],"data":{"resultType":"vector","result":[]}}`)

	frames, err := ds.QueryInstant(t.Context(), datasource.Query{Text: "up"}, datasource.TimeRange{From: time.Now().Add(-time.Minute), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	if meta := datasource.MergeMeta(frames); !meta.Partial || meta.Warning != "store a is down" {
		t.Fatalf("meta = %+v", datasource.MergeMeta(frames))
	}
}

func TestAnAnswerWithoutWarningsIsNotPartial(t *testing.T) {
	ds := warningDatasource(t, `{"status":"success","data":{"resultType":"vector","result":[]}}`)

	frames, err := ds.QueryInstant(t.Context(), datasource.Query{Text: "up"}, datasource.TimeRange{From: time.Now().Add(-time.Minute), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	if len(frames) != 0 || datasource.MergeMeta(frames).Partial {
		t.Fatalf("frames = %+v", frames)
	}
}
