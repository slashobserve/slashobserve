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
