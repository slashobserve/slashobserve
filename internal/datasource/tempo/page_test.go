package tempo_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const searchQuery = `{ resource.service.name = "checkout" }`

func traceID32(c byte) string { return strings.Repeat(string(c), 32) }

func traceJSON(id string, startNs int64) string {
	return fmt.Sprintf(`{"traceID":%q,"rootServiceName":"checkout","rootTraceName":"POST /checkout","startTimeUnixNano":"%d","durationMs":5}`, id, startNs)
}

func searchReply(metrics string, traces ...string) reply {
	return reply{0, `{"traces":[` + strings.Join(traces, ",") + `],"metrics":{` + metrics + `}}`}
}

func tracePageRequest(size int, cursor *datasource.PageCursor) datasource.PageRequest {
	return datasource.PageRequest{Query: datasource.Query{Text: searchQuery}, Range: lastHour, Page: 1, PageSize: size, Cursor: cursor}
}

func replaceSearch(fake *fakeTempo, rep reply) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.replies["/api/search"] = rep
}

func TestTraceSearchPageSendsLimitPlusOne(t *testing.T) {
	ds, fake := serve(t, map[string]reply{"/api/search": {0, searchBody}})
	frames, info, err := ds.QueryPage(context.Background(), tracePageRequest(50, nil))
	if err != nil {
		t.Fatal(err)
	}
	params := fake.last(t).params
	if params.Get("q") != searchQuery || params.Get("limit") != "51" || params.Get("start") != "1793962800" || params.Get("end") != "1793966401" {
		t.Fatalf("params = %v, want limit 51 over the whole range", params)
	}
	if info.Page != 1 || info.PageSize != 50 || info.HasNext || info.Next != nil || info.Total != 2 || info.TotalRelation != datasource.TotalExact {
		t.Fatalf("info = %+v, want page 1 of exactly 2 traces", info)
	}
	if len(frames) != 1 || frames[0].Rows() != 2 || frames[0].Meta != nil {
		t.Fatalf("frames = %+v, want one frame of 2 traces with no meta", frames)
	}
}

func TestNextTraceSearchPageCutsInsideTheSecond(t *testing.T) {
	const second = int64(1793966399)
	a, b, c, d, e := traceID32('a'), traceID32('b'), traceID32('c'), traceID32('d'), traceID32('e')
	ds, fake := serve(t, map[string]reply{"/api/search": searchReply("",
		traceJSON(a, (second+1)*1e9+500_000_000), traceJSON(b, second*1e9+300_000_000), traceJSON(c, second*1e9+200_000_000), traceJSON(d, second*1e9+100_000_000))})
	frames, info, err := ds.QueryPage(context.Background(), tracePageRequest(3, nil))
	if err != nil {
		t.Fatal(err)
	}
	if fake.last(t).params.Get("limit") != "4" {
		t.Fatalf("params = %v, want limit 4", fake.last(t).params)
	}
	if got := fieldValues(t, frames[0], "TraceId"); !slices.Equal(got, []any{a, b, c}) {
		t.Fatalf("page 1 = %v, want a, b, c", got)
	}
	if !info.HasNext || info.Total != 3 || info.TotalRelation != datasource.TotalAtLeast || info.Next == nil {
		t.Fatalf("info = %+v, want at least 3 traces and a next cursor", info)
	}
	if info.Next.Page != 2 || info.Next.EndNs != second*1e9+200_000_001 || !slices.Equal(info.Next.Seen, []string{b, c}) {
		t.Fatalf("next = %+v, want page 2 ending after c with b and c seen", info.Next)
	}

	replaceSearch(fake, searchReply("", traceJSON(b, second*1e9+300_000_000), traceJSON(c, second*1e9+200_000_000), traceJSON(d, second*1e9+100_000_000), traceJSON(e, (second-1)*1e9+500_000_000)))
	frames, info, err = ds.QueryPage(context.Background(), tracePageRequest(3, info.Next))
	if err != nil {
		t.Fatal(err)
	}
	params := fake.last(t).params
	if params.Get("end") != "1793966400" || params.Get("limit") != "6" || params.Get("start") != "1793962800" {
		t.Fatalf("params = %v, want the boundary second as end, limit 6 and the pinned start", params)
	}
	if got := fieldValues(t, frames[0], "TraceId"); !slices.Equal(got, []any{d, e}) {
		t.Fatalf("page 2 = %v, want d and e with the seen traces dropped", got)
	}
	if info.Page != 2 || info.HasNext || info.Next != nil || info.Total != 5 || info.TotalRelation != datasource.TotalExact {
		t.Fatalf("info = %+v, want the last page with exactly 5 traces", info)
	}
}

func TestAPageOfOneSecondCannotAdvance(t *testing.T) {
	const second = int64(1793966399)
	ds, _ := serve(t, map[string]reply{"/api/search": searchReply("",
		traceJSON(traceID32('a'), second*1e9+300), traceJSON(traceID32('b'), second*1e9+200), traceJSON(traceID32('c'), second*1e9+100))})
	_, _, err := ds.QueryPage(context.Background(), tracePageRequest(2, nil))
	if !errors.Is(err, datasource.ErrLimitExceeded) || !strings.Contains(err.Error(), "more than page_size rows share one timestamp") {
		t.Fatalf("err = %v, want the shared timestamp limit error", err)
	}
}

func TestPartialSearchHasUnknownTotal(t *testing.T) {
	ds, _ := serve(t, map[string]reply{"/api/search": searchReply(`"inspectedBytes":"1805425","completedJobs":1,"totalJobs":7`, traceJSON(traceID32('a'), 1793966399000000000))})
	frames, info, err := ds.QueryPage(context.Background(), tracePageRequest(50, nil))
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalRelation != datasource.TotalUnknown || info.Total != 0 || info.HasNext {
		t.Fatalf("info = %+v, want an unknown total", info)
	}
	meta := frames[0].Meta
	if meta == nil || !meta.Partial || meta.Truncated || meta.JobsDone != 1 || meta.JobsTotal != 7 || meta.InspectedBytes != 1805425 {
		t.Fatalf("meta = %+v, want partial with 1 of 7 jobs and no truncation", meta)
	}
}

func TestPageRefusesATraceID(t *testing.T) {
	ds, fake := serve(t, map[string]reply{"/api/search": {0, searchBody}})
	req := tracePageRequest(50, nil)
	req.Query.Text = traceID
	_, _, err := ds.QueryPage(context.Background(), req)
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "paging needs a TraceQL search") {
		t.Fatalf("err = %v", err)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("%d requests reached Tempo for a refused page", len(fake.requests))
	}
}
