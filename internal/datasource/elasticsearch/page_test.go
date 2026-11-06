package elasticsearch_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

const settingsPath = "/logs-*/_settings"

func pagedReply(total int64, relation string, hits int) string {
	rows := make([]string, hits)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"_index":"logs-shop-2026.11","_id":"id%d","_source":{"@timestamp":"2026-11-06T11:59:59.%09dZ","message":"line %d"}}`, i, i, i)
	}
	return fmt.Sprintf(`{"timed_out":false,"hits":{"total":{"value":%d,"relation":%q},"hits":[%s]}}`, total, relation, strings.Join(rows, ","))
}

func pageServer(t *testing.T, settings, search string) (*fakeServer, *elasticsearch.Datasource) {
	t.Helper()
	f := healthyServer(t)
	f.fixed(http.MethodGet, settingsPath, http.StatusOK, settings)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, search)
	return f, serve(t, f)
}

func pageRequest(page, size int, text string) datasource.PageRequest {
	return datasource.PageRequest{Query: datasource.Query{Text: text}, Range: queryRange, Page: page, PageSize: size}
}

func TestQueryPageSendsTheOffsetAndCountsExactly(t *testing.T) {
	f, ds := pageServer(t, `{}`, pagedReply(1234, "eq", 100))
	frames, info, err := ds.QueryPage(context.Background(), pageRequest(3, 100, "service.name:checkout"))
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f.last(t).body, `{
		"from": 200, "size": 100, "track_total_hits": true, "timeout": "30000ms", "sort": `+defaultSort+`,
		"query": {"bool": {"filter": [`+rangeFilter+`, {"match": {"service.name": "checkout"}}]}}
	}`)
	if frames[0].Rows() != 100 {
		t.Fatalf("rows = %d", frames[0].Rows())
	}
	if meta := frames[0].Meta; meta == nil || meta.Truncated || meta.RowsTotal != 0 {
		t.Fatalf("meta = %+v, want no truncated and no rows_total", meta)
	}
	want := datasource.PageInfo{Page: 3, PageSize: 100, Total: 1234, TotalRelation: datasource.TotalExact, HasNext: true, Window: 10000}
	if info.Next != nil || info.Page != want.Page || info.PageSize != want.PageSize || info.Total != want.Total || info.TotalRelation != want.TotalRelation || info.HasNext != want.HasNext || info.Window != want.Window {
		t.Fatalf("info = %+v, want %+v", info, want)
	}
}

func TestQueryPageFirstPageSendsNoOffset(t *testing.T) {
	f, ds := pageServer(t, `{}`, pagedReply(10, "eq", 10))
	if _, _, err := ds.QueryPage(context.Background(), pageRequest(0, 50, "")); err != nil {
		t.Fatal(err)
	}
	body := searchBody(t, f.last(t))
	if _, ok := body["from"]; ok || body["size"] != 50.0 {
		t.Fatalf("body = %v", body)
	}
}

func TestQueryPageHasNextStopsAtTheTotalAndAtTheWindow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		page     int
		total    int64
		relation string
		hasNext  bool
		relWant  datasource.TotalRelation
	}{
		{"last page of a small result", 3, 250, "eq", false, datasource.TotalExact},
		{"a full page before the end", 2, 250, "eq", true, datasource.TotalExact},
		{"last reachable page of a big result", 100, 20000, "eq", false, datasource.TotalExact},
		{"one page before the last reachable", 99, 20000, "eq", true, datasource.TotalExact},
		{"capped total", 1, 10000, "gte", true, datasource.TotalAtLeast},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ds := pageServer(t, `{}`, pagedReply(tc.total, tc.relation, 100))
			_, info, err := ds.QueryPage(context.Background(), pageRequest(tc.page, 100, ""))
			if err != nil {
				t.Fatal(err)
			}
			if info.HasNext != tc.hasNext || info.TotalRelation != tc.relWant || info.Total != tc.total || info.Next != nil {
				t.Fatalf("info = %+v", info)
			}
		})
	}
}

func TestQueryPageRefusesPagesPastTheWindowBeforeSearching(t *testing.T) {
	f, ds := pageServer(t, `{}`, pagedReply(20000, "eq", 100))
	_, _, err := ds.QueryPage(context.Background(), pageRequest(101, 100, ""))
	want := "pages past 10,000 results are not available on this index; narrow the time range"
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if f.count(searchPath) != 0 {
		t.Fatal("a refused page reached the search endpoint")
	}
	if _, _, err := ds.QueryPage(context.Background(), pageRequest(1<<60, 500, "")); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("huge page: %v", err)
	}
}

func TestQueryPageMapsTheServersResultWindowRefusal(t *testing.T) {
	f, ds := pageServer(t, `{}`, pagedReply(1, "eq", 1))
	f.fixed(http.MethodPost, searchPath, http.StatusBadRequest, esError("illegal_argument_exception", "Result window is too large, from + size must be less than or equal to: [10000] but was [10100]. See the scroll api"))
	_, _, err := ds.QueryPage(context.Background(), pageRequest(2, 100, ""))
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "pages past 10,000 results are not available on this index; narrow the time range") {
		t.Fatalf("error = %v", err)
	}
}

func TestQueryPageIgnoresACursorAndTheBodysSize(t *testing.T) {
	f, ds := pageServer(t, `{}`, pagedReply(500, "eq", 50))
	plain := pageRequest(2, 50, `{"size": 3, "query": {"match_all": {}}}`)
	if _, _, err := ds.QueryPage(context.Background(), plain); err != nil {
		t.Fatal(err)
	}
	without := f.last(t).body
	plain.Cursor = &datasource.PageCursor{Page: 7, EndNs: 1, Seen: []string{"x"}}
	_, info, err := ds.QueryPage(context.Background(), plain)
	if err != nil {
		t.Fatal(err)
	}
	if f.last(t).body != without || info.Page != 2 || info.Next != nil {
		t.Fatalf("a cursor changed the request or the answer: %s, %+v", f.last(t).body, info)
	}
	if body := searchBody(t, f.last(t)); body["size"] != 50.0 || body["from"] != 50.0 {
		t.Fatalf("body = %v", body)
	}
}

func TestQueryPageRefusesAggregations(t *testing.T) {
	f, ds := pageServer(t, `{}`, pagedReply(1, "eq", 1))
	_, _, err := ds.QueryPage(context.Background(), pageRequest(1, 50, `{"aggs": {"a": {"terms": {"field": "service.name"}}}}`))
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "paging needs a KQL, Lucene or filter-only query body") {
		t.Fatalf("error = %v", err)
	}
	if f.count(searchPath) != 0 {
		t.Fatal("an aggregation query reached the search endpoint")
	}
}

func TestQueryPageExpandsMacrosAndAppliesFilters(t *testing.T) {
	f, ds := pageServer(t, `{}`, pagedReply(1, "eq", 1))
	req := pageRequest(1, 50, "lucene:service.name:${svc:pipe}")
	req.Query.Variables = []datasource.Variable{{Name: "svc", Values: []string{"a", "b"}}}
	req.Query.Filters = []datasource.AdhocFilter{{Key: "log.level", Op: "=", Value: "error"}}
	if _, _, err := ds.QueryPage(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	body := f.last(t).body
	if !strings.Contains(body, `service.name:a|b`) || !strings.Contains(body, `{"term":{"log.level":"error"}}`) {
		t.Fatalf("body = %s", body)
	}
}

func TestPagesByOffset(t *testing.T) {
	_, ds := pageServer(t, `{}`, `{}`)
	var source datasource.PageSource = ds
	if !source.PagesByOffset() {
		t.Fatal("Elasticsearch pages by offset")
	}
}

func TestMaxResultWindow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		status   int
		want     int
	}{
		{"not set on any index", `{}`, 200, 10000},
		{"smallest across indices", `{"logs-shop-2026.11":{"settings":{"index":{"max_result_window":"10000"}}},"logs-legacy-2026.11":{"settings":{"index":{"max_result_window":"5000"}}}}`, 200, 5000},
		{"raised on the only index that sets it", `{"logs-shop-2026.11":{"settings":{"index":{"max_result_window":"50000"}}}}`, 200, 50000},
		{"number instead of a string", `{"a":{"settings":{"index":{"max_result_window":2000}}}}`, 200, 2000},
		{"unreadable value", `{"a":{"settings":{"index":{"max_result_window":"lots"}}}}`, 200, 10000},
		{"no permission to read settings", `{"error":"forbidden"}`, 403, 10000},
		{"garbage", `[1]`, 200, 10000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := healthyServer(t)
			f.fixed(http.MethodGet, settingsPath, tc.status, tc.settings)
			f.fixed(http.MethodPost, searchPath, http.StatusOK, pagedReply(100000, "eq", 50))
			ds := serve(t, f)
			m, err := ds.Mapping(context.Background())
			if err != nil || m.Window != tc.want {
				t.Fatalf("window = %d, err = %v, want %d", m.Window, err, tc.want)
			}
			_, info, err := ds.QueryPage(context.Background(), pageRequest(1, 50, ""))
			if err != nil || info.Window != tc.want {
				t.Fatalf("info.Window = %d, err = %v, want %d", info.Window, err, tc.want)
			}
		})
	}
}

func TestMaxResultWindowIsCachedWithTheMapping(t *testing.T) {
	f, ds := pageServer(t, `{"a":{"settings":{"index":{"max_result_window":"5000"}}}}`, pagedReply(100000, "eq", 50))
	for range 3 {
		if _, _, err := ds.QueryPage(context.Background(), pageRequest(1, 50, "")); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count(settingsPath); n != 1 {
		t.Fatalf("settings read %d times, want once", n)
	}
	if _, _, err := ds.QueryPage(context.Background(), pageRequest(101, 50, "")); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("page past a 5000 window: %v", err)
	}
}
