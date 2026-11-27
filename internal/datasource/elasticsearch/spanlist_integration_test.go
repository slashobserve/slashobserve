package elasticsearch_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

func listIDs(list datasource.TraceList) []string {
	ids := make([]string, len(list.Traces))
	for i, s := range list.Traces {
		ids[i] = s.ID
	}
	return ids
}

func TestIntegrationSpanListFindsFailingTracesAcrossSpans(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := spanDS(t, flavor)
		ctx := context.Background()
		failing, err := ds.ListTraces(ctx, datasource.TraceListRequest{Range: aroundIndexing(), Errors: true})
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for i := checkoutTraces - 1; i >= 0; i-- {
			if i%4 == 0 {
				want = append(want, checkoutTraceID(i))
			}
		}
		want = append(want, legacyTrace, capturedTrace)
		if got := listIDs(failing); !slices.Equal(got, want) || !failing.Complete {
			t.Fatalf("failing = %v complete %v, want %v", got, failing.Complete, want)
		}
		first := failing.Traces[0]
		if first.RootService != "checkout" || first.Route != "POST /checkout" || first.Duration != time.Duration(checkoutRootMillis(36))*time.Millisecond {
			t.Errorf("newest failing = %+v", first)
		}
		if last := failing.Traces[len(failing.Traces)-1]; last.RootService != "frontend" || last.Route != "GET /api/cart" || !last.Start.Equal(indexedAt) {
			t.Errorf("captured = %+v, indexed at %v", last, indexedAt)
		}
		both, err := ds.ListTraces(ctx, datasource.TraceListRequest{Filter: datasource.TraceFilter{Service: "checkout"}, Range: aroundIndexing(), Errors: true})
		if err != nil {
			t.Fatal(err)
		}
		if got := listIDs(both); !slices.Equal(got, want[:10]) || !both.Complete {
			t.Errorf("checkout with an error anywhere = %v complete %v", got, both.Complete)
		}
	})
}

func TestIntegrationSpanListFiltersByRouteAndDuration(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := spanDS(t, flavor)
		ctx := context.Background()
		slow, err := ds.ListTraces(ctx, datasource.TraceListRequest{Filter: datasource.TraceFilter{Route: "POST /checkout"}, Range: aroundIndexing(), MinDuration: 300 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		if len(slow.Traces) != 11 || !slow.Complete || slow.Traces[10].ID != checkoutTraceID(29) {
			t.Fatalf("slow = %v complete %v", listIDs(slow), slow.Complete)
		}
		child, err := ds.ListTraces(ctx, datasource.TraceListRequest{Filter: datasource.TraceFilter{Route: "charge"}, Range: aroundIndexing()})
		if err != nil || len(child.Traces) != 0 {
			t.Errorf("a child span name is no route: %v %v", listIDs(child), err)
		}
		limited, err := ds.ListTraces(ctx, datasource.TraceListRequest{Filter: datasource.TraceFilter{Operation: "charge"}, Range: aroundIndexing(), Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		if got := listIDs(limited); limited.Complete || !slices.Equal(got, []string{checkoutTraceID(39), checkoutTraceID(38), checkoutTraceID(37), checkoutTraceID(36), checkoutTraceID(35)}) {
			t.Errorf("limited = %v complete %v", got, limited.Complete)
		}
	})
}
