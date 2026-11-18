package queryproxy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type fakePageSource struct {
	*fakeDatasource
	byOffset bool
	onPage   func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error)
	pages    []datasource.PageRequest
}

func (f *fakePageSource) PagesByOffset() bool { return f.byOffset }

func (f *fakePageSource) QueryPage(_ context.Context, req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
	f.pages = append(f.pages, req)
	if f.onPage != nil {
		return f.onPage(req)
	}
	return dataFrames(), datasource.PageInfo{Page: req.Page, PageSize: req.PageSize, Total: 3, TotalRelation: datasource.TotalExact}, nil
}

func pageRunner(t *testing.T, source *fakePageSource) (*Runner, datasource.Instance) {
	t.Helper()
	inst := datasource.Instance{UID: "logs", Datasource: source}
	return newRunnerOf(t, discardLogger(), inst), inst
}

func pageParams(page int, cursor string) PageParams {
	return PageParams{UID: "logs", Query: datasource.Query{Text: `{a="b"}`}, Range: testRange(), Page: page, PageSize: 100, Cursor: cursor}
}

func TestRunPageRefusesDatasourcesThatDoNotPage(t *testing.T) {
	r := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": newFake("prometheus")})
	inst, _ := r.reg.Get("prom")
	_, err := r.RunPage(context.Background(), inst, pageParams(1, ""))
	if !errors.Is(err, ErrInvalidRequest) || err.Error() != NotPageSource {
		t.Fatalf("err = %v, want %q", err, NotPageSource)
	}
}

func TestRunPageNeedsACursorPastPageOneOnCursorStores(t *testing.T) {
	for _, tc := range []struct {
		name     string
		byOffset bool
		wantErr  bool
	}{
		{"cursor store", false, true},
		{"offset store", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &fakePageSource{fakeDatasource: newFake("loki"), byOffset: tc.byOffset}
			r, inst := pageRunner(t, source)
			_, err := r.RunPage(context.Background(), inst, pageParams(3, ""))
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidRequest) || err.Error() != PageNeedsCursor {
					t.Fatalf("err = %v, want %q", err, PageNeedsCursor)
				}
				return
			}
			if err != nil || len(source.pages) != 1 || source.pages[0].Page != 3 || source.pages[0].Cursor != nil {
				t.Fatalf("pages = %+v, err %v; want page 3 without a cursor", source.pages, err)
			}
		})
	}
}

func TestRunPageEncodesTheNextCursorWithTheScopeAndRange(t *testing.T) {
	source := &fakePageSource{fakeDatasource: newFake("loki"), onPage: func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return nil, datasource.PageInfo{Page: 1, PageSize: 100, Total: 100, TotalRelation: datasource.TotalAtLeast, HasNext: true, Next: &datasource.PageCursor{Page: 2, EndNs: 42, Seen: []string{"fp"}}}, nil
	}}
	r, inst := pageRunner(t, source)
	res, err := r.RunPage(context.Background(), inst, pageParams(1, ""))
	if err != nil {
		t.Fatal(err)
	}
	if res.HasPrev || res.Prev != "" || res.Next == "" || res.Frames == nil || len(res.Frames) != 0 {
		t.Fatalf("result = %+v, want a next cursor, no prev and an empty frame list", res)
	}
	cursor, err := datasource.DecodeCursor(res.Next, datasource.CursorScope("logs", `{a="b"}`))
	if err != nil {
		t.Fatal(err)
	}
	rng := testRange()
	if cursor.Page != 2 || cursor.EndNs != 42 || cursor.FromNs != rng.From.UnixNano() || cursor.ToNs != rng.To.UnixNano() || len(cursor.Seen) != 1 {
		t.Fatalf("cursor = %+v, want page 2 pinned to the request range", cursor)
	}
}

func TestRunPagePinsTheRangeFromTheCursorAndEchoesItAsPrev(t *testing.T) {
	source := &fakePageSource{fakeDatasource: newFake("loki")}
	r, inst := pageRunner(t, source)
	pinned := datasource.TimeRange{From: testNow.Add(-2 * time.Hour), To: testNow.Add(-time.Hour)}
	cursor := datasource.EncodeCursor(datasource.PageCursor{Scope: datasource.CursorScope("logs", `{a="b"}`), Page: 2, FromNs: pinned.From.UnixNano(), ToNs: pinned.To.UnixNano(), EndNs: 7})
	res, err := r.RunPage(context.Background(), inst, pageParams(1, cursor))
	if err != nil {
		t.Fatal(err)
	}
	got := source.pages[0]
	if got.Page != 2 || got.Cursor == nil || got.Cursor.EndNs != 7 || !got.Range.From.Equal(pinned.From) || !got.Range.To.Equal(pinned.To) {
		t.Fatalf("page request = %+v, want page 2 over the cursor's range", got)
	}
	if !res.HasPrev || res.Prev != cursor || !res.Range.From.Equal(pinned.From) || res.Info.Page != 2 {
		t.Fatalf("result = %+v, want prev echoed and the pinned range", res)
	}
}

func TestRunPageRefusesCursorsOfOtherQueriesAndDatasources(t *testing.T) {
	source := &fakePageSource{fakeDatasource: newFake("loki")}
	r, inst := pageRunner(t, source)
	for name, scope := range map[string]string{"other query": datasource.CursorScope("logs", `{a="c"}`), "other datasource": datasource.CursorScope("traces", `{a="b"}`)} {
		t.Run(name, func(t *testing.T) {
			cursor := datasource.EncodeCursor(datasource.PageCursor{Scope: scope, Page: 2})
			_, err := r.RunPage(context.Background(), inst, pageParams(1, cursor))
			if !errors.Is(err, ErrInvalidRequest) || err.Error() != "cursor does not belong to this query" {
				t.Fatalf("err = %v", err)
			}
		})
	}
	_, err := r.RunPage(context.Background(), inst, pageParams(1, strings.Repeat("x", 10)))
	if !errors.Is(err, ErrInvalidRequest) || err.Error() != "cursor is not readable" {
		t.Fatalf("err = %v", err)
	}
	if len(source.pages) != 0 {
		t.Fatalf("%d pages ran for refused cursors", len(source.pages))
	}
}

type sortablePageSource struct{ *fakePageSource }

func (sortablePageSource) SortsPages() bool { return true }

func TestRunPagePassesTheSortToDatasourcesThatSort(t *testing.T) {
	source := &fakePageSource{fakeDatasource: newFake("clickhouse"), byOffset: true}
	inst := datasource.Instance{UID: "logs", Datasource: sortablePageSource{source}}
	r := newRunnerOf(t, discardLogger(), inst)
	params := pageParams(1, "")
	params.Sort = "-Duration"
	if _, err := r.RunPage(context.Background(), inst, params); err != nil {
		t.Fatal(err)
	}
	if len(source.pages) != 1 || source.pages[0].Sort == nil || source.pages[0].Sort.Column != "Duration" || !source.pages[0].Sort.Descending {
		t.Fatalf("pages = %+v, want the sort -Duration", source.pages)
	}
}

func TestRunPageRefusesASortOnDatasourcesThatCannotSort(t *testing.T) {
	source := &fakePageSource{fakeDatasource: newFake("tempo")}
	r, inst := pageRunner(t, source)
	params := pageParams(1, "")
	params.Sort = "-Duration"
	_, err := r.RunPage(context.Background(), inst, params)
	if !errors.Is(err, ErrInvalidRequest) || err.Error() != SortUnsupported {
		t.Fatalf("err = %v, want %q", err, SortUnsupported)
	}
	if len(source.pages) != 0 {
		t.Fatalf("the datasource ran %d pages, want none", len(source.pages))
	}
}

func TestRunPageRefusesASortThatIsNotAColumnName(t *testing.T) {
	source := &fakePageSource{fakeDatasource: newFake("clickhouse"), byOffset: true}
	inst := datasource.Instance{UID: "logs", Datasource: sortablePageSource{source}}
	r := newRunnerOf(t, discardLogger(), inst)
	params := pageParams(1, "")
	params.Sort = "Duration; DROP"
	_, err := r.RunPage(context.Background(), inst, params)
	if !errors.Is(err, ErrInvalidRequest) || err.Error() != datasource.SortRule {
		t.Fatalf("err = %v, want %q", err, datasource.SortRule)
	}
}

func TestLimitNoteTellsHowToGetMore(t *testing.T) {
	cases := []struct {
		name                                 string
		truncated, partial, hasNext, canPage bool
		want                                 string
	}{
		{"nothing to say", false, false, false, false, ""},
		{"row limit where paging is offered", true, false, false, true, MoreRowsNote},
		{"row limit where it is not", true, false, false, false, NarrowNote},
		{"full page", false, false, true, false, FullPageNote},
		{"time limit", false, true, false, false, PartialNote},
		{"full page and time limit", false, true, true, false, FullPageNote + ". " + PartialNote},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LimitNote(tc.truncated, tc.partial, tc.hasNext, tc.canPage); got != tc.want {
				t.Fatalf("note = %q, want %q", got, tc.want)
			}
		})
	}
	if strings.Contains(NarrowNote, "page_size") {
		t.Errorf("%q offers paging", NarrowNote)
	}
	for note, want := range map[string]string{MoreRowsNote: "page_size", FullPageNote: "cursor", PartialNote: "add a filter"} {
		if !strings.Contains(note, want) {
			t.Errorf("%q does not name %q", note, want)
		}
	}
}

func TestRunPageTiesACursorToTheSortItWasMadeWith(t *testing.T) {
	source := &fakePageSource{fakeDatasource: newFake("clickhouse"), byOffset: true, onPage: func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return dataFrames(), datasource.PageInfo{Page: req.Page, PageSize: req.PageSize, Next: &datasource.PageCursor{Page: req.Page + 1}}, nil
	}}
	inst := datasource.Instance{UID: "logs", Datasource: sortablePageSource{source}}
	r := newRunnerOf(t, discardLogger(), inst)
	first := pageParams(1, "")
	first.Sort = "-Duration"
	res, err := r.RunPage(context.Background(), inst, first)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct{ sort, want string }{
		"no sort":      {"", "this cursor was made with sort -Duration; send sort -Duration with it"},
		"another sort": {"Duration", "this cursor was made with sort -Duration; send sort -Duration with it"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			params := pageParams(1, res.Next)
			params.Sort = tc.sort
			_, err := r.RunPage(context.Background(), inst, params)
			if !errors.Is(err, ErrInvalidRequest) || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	same := pageParams(1, res.Next)
	same.Sort = "-Duration"
	if _, err := r.RunPage(context.Background(), inst, same); err != nil {
		t.Fatalf("same sort refused: %v", err)
	}
	unsorted, err := r.RunPage(context.Background(), inst, pageParams(1, ""))
	if err != nil {
		t.Fatal(err)
	}
	withSort := pageParams(1, unsorted.Next)
	withSort.Sort = "-Duration"
	if _, err := r.RunPage(context.Background(), inst, withSort); err == nil || err.Error() != "this cursor was made without a sort; leave sort out" {
		t.Fatalf("err = %v, want the cursor made without a sort refused", err)
	}
	if len(source.pages) != 3 {
		t.Fatalf("%d pages ran, want 3 (two firsts and the walk with the same sort)", len(source.pages))
	}
}
