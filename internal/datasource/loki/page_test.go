package loki_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const checkoutStream = `{service_name="checkout"}`

var checkoutLabels = map[string]string{"service_name": "checkout"}

func checkoutLines(lines ...string) string {
	values := make([]string, len(lines))
	for i, line := range lines {
		ns, body, _ := strings.Cut(line, " ")
		n, err := strconv.Atoi(ns)
		if err != nil {
			panic(err)
		}
		values[i] = fmt.Sprintf(`["1793966400%09d",%q]`, n, body)
	}
	return success("streams", `[{"stream":{"service_name":"checkout"},"values":[`+strings.Join(values, ",")+`]}]`)
}

func pageRequest(size int, cursor *datasource.PageCursor) datasource.PageRequest {
	return datasource.PageRequest{Query: datasource.Query{Text: checkoutStream}, Range: linesRange, Page: 1, PageSize: size, Cursor: cursor}
}

func replace(fake *fakeLoki, body string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.body = body
}

func TestPageOneSendsPageSizePlusOne(t *testing.T) {
	ds, fake := serve(t, 0, checkoutLines("3 a", "2 b", "1 c"))
	frames, info, err := ds.QueryPage(context.Background(), pageRequest(50, nil))
	if err != nil {
		t.Fatal(err)
	}
	req := fake.last(t)
	if req.params.Get("limit") != "51" || req.params.Get("direction") != "backward" || req.params.Get("end") != "2026-11-06T12:00:00.000000005Z" {
		t.Fatalf("params = %v, want limit 51 backward over the whole range", req.params)
	}
	if info.Page != 1 || info.PageSize != 50 || info.HasNext || info.Next != nil || info.Total != 3 || info.TotalRelation != datasource.TotalExact {
		t.Fatalf("info = %+v, want page 1 of exactly 3 rows", info)
	}
	if len(frames) != 1 || !slices.Equal(fieldValues(t, frames[0], "Body"), []any{"a", "b", "c"}) {
		t.Fatalf("frames = %+v, want the three lines", frames)
	}
}

func TestNextPageReusesTheCursorEndAndSkipsSeenLines(t *testing.T) {
	ds, fake := serve(t, 0, checkoutLines("3 a", "2 b", "2 c"))
	frames, info, err := ds.QueryPage(context.Background(), pageRequest(2, nil))
	if err != nil {
		t.Fatal(err)
	}
	if fake.last(t).params.Get("limit") != "3" {
		t.Fatalf("params = %v, want limit 3", fake.last(t).params)
	}
	if !info.HasNext || info.Total != 2 || info.TotalRelation != datasource.TotalAtLeast || info.Next == nil {
		t.Fatalf("info = %+v, want at least 2 rows and a next cursor", info)
	}
	shown := fieldValues(t, frames[0], "Body")
	if len(shown) != 2 || shown[0] != "a" {
		t.Fatalf("page 1 = %v, want a then one of the boundary lines", shown)
	}
	boundary, _ := shown[1].(string)
	if info.Next.Page != 2 || info.Next.EndNs != 1793966400000000003 || !slices.Equal(info.Next.Seen, []string{datasource.RowFingerprint(checkoutLabels, boundary)}) {
		t.Fatalf("next = %+v, want page 2 ending before ns 3 with %q seen", info.Next, boundary)
	}

	replace(fake, checkoutLines("2 b", "2 c", "1 d"))
	frames, info, err = ds.QueryPage(context.Background(), pageRequest(2, info.Next))
	if err != nil {
		t.Fatal(err)
	}
	req := fake.last(t)
	if req.params.Get("end") != "2026-11-06T12:00:00.000000003Z" || req.params.Get("limit") != "4" || req.params.Get("start") != "2026-11-06T11:00:00.123456789Z" {
		t.Fatalf("params = %v, want end at the cursor, limit 4 and the pinned start", req.params)
	}
	other := "b"
	if boundary == "b" {
		other = "c"
	}
	if got := fieldValues(t, frames[0], "Body"); !slices.Equal(got, []any{other, "d"}) {
		t.Fatalf("page 2 = %v, want the unseen boundary line then d", got)
	}
	if info.Page != 2 || info.HasNext || info.Next != nil || info.Total != 4 || info.TotalRelation != datasource.TotalExact {
		t.Fatalf("info = %+v, want the last page with exactly 4 rows", info)
	}
	if frames[0].Meta != nil {
		t.Fatalf("meta = %+v, want none on a page", frames[0].Meta)
	}
}

func TestLastPageIsExact(t *testing.T) {
	ds, _ := serve(t, 0, checkoutLines("1 z"))
	_, info, err := ds.QueryPage(context.Background(), pageRequest(2, &datasource.PageCursor{Page: 4, EndNs: 1793966400000000002}))
	if err != nil {
		t.Fatal(err)
	}
	if info.Page != 4 || info.HasNext || info.Total != 7 || info.TotalRelation != datasource.TotalExact {
		t.Fatalf("info = %+v, want page 4 closing an exact total of 7", info)
	}
}

func TestAPageOfOneTimestampCannotAdvance(t *testing.T) {
	ds, _ := serve(t, 0, checkoutLines("2 a", "2 b", "2 c"))
	_, _, err := ds.QueryPage(context.Background(), pageRequest(2, nil))
	if !errors.Is(err, datasource.ErrLimitExceeded) || !strings.Contains(err.Error(), "more than page_size rows share one timestamp") {
		t.Fatalf("err = %v, want the shared timestamp limit error", err)
	}
}

func TestPageKeepsOneFramePerStreamAndDropsEmptiedStreams(t *testing.T) {
	ds, _ := serve(t, 0, success("streams", `[
		{"stream":{"service_name":"frontend"},"values":[["1793966400000000001","old"]]},
		{"stream":{"service_name":"checkout"},"values":[["1793966400000000003","new"],["1793966400000000002","mid"]]}
	]`))
	frames, info, err := ds.QueryPage(context.Background(), pageRequest(2, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasNext || len(frames) != 1 || frames[0].Labels["service_name"] != "checkout" {
		t.Fatalf("frames = %+v, want only the checkout stream on page 1", frames)
	}
	if got := fieldValues(t, frames[0], "Body"); !slices.Equal(got, []any{"new", "mid"}) {
		t.Fatalf("bodies = %v", got)
	}
	if got := fieldValues(t, frames[0], "TimeNs"); !slices.Equal(got, []any{"1793966400000000003", "1793966400000000002"}) {
		t.Fatalf("TimeNs = %v", got)
	}
}
