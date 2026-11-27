package tempo_test

import (
	"context"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/tempo"
)

func TestListQueryJoinsTheFilterAsSpansets(t *testing.T) {
	for _, tc := range []struct {
		req  datasource.TraceListRequest
		want string
	}{
		{datasource.TraceListRequest{}, "{}"},
		{datasource.TraceListRequest{Errors: true}, "{ status = error }"},
		{
			datasource.TraceListRequest{Filter: datasource.TraceFilter{Route: `GET "/a"`, Service: "cart", Operation: "Get"}, Errors: true, MinDuration: 1500 * time.Microsecond},
			`{ trace:rootName = "GET \"/a\"" } && { resource.service.name = "cart" } && { name = "Get" } && { status = error } && { trace:duration >= 1500us }`,
		},
	} {
		if got := tempo.ListQuery(tc.req); got != tc.want {
			t.Errorf("ListQuery(%+v) = %s, want %s", tc.req, got, tc.want)
		}
	}
}

func TestListTracesSaysWhetherItListedEveryTrace(t *testing.T) {
	body := `{"traces":[{"traceID":"abc","rootServiceName":"cart","rootTraceName":"GET /cart","startTimeUnixNano":"1793872800000000000","durationMs":12}],"metrics":{"completedJobs":1,"totalJobs":1}}`
	ds, _ := serve(t, map[string]reply{"/api/search": {0, body}})
	got, err := ds.ListTraces(context.Background(), datasource.TraceListRequest{Range: statsRange, Errors: true, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Traces) != 1 {
		t.Fatalf("list = %+v", got)
	}
	if tr := got.Traces[0]; tr.ID != "00000000000000000000000000000abc" || tr.Route != "GET /cart" || tr.RootService != "cart" || tr.Duration != 12*time.Millisecond {
		t.Fatalf("trace = %+v", tr)
	}
	got, err = ds.ListTraces(context.Background(), datasource.TraceListRequest{Range: statsRange, Limit: 1})
	if err != nil || got.Complete {
		t.Fatalf("a full page is complete: %+v, %v", got, err)
	}
}
