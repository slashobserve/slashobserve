package queryproxy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	NotPageSource      = "paging reads logs and trace datasources"
	PageNeedsCursor    = "this datasource pages by cursor; pass the cursor of the previous page"
	PagingIsInstant    = "paging applies to instant queries"
	LimitWithPage      = "limit and page_size cannot both be given"
	PageNeedsSize      = "page and cursor need page_size"
	PageSizeOutside    = "page_size must be from 50 to 500"
	PageBelowOne       = "page must be at least 1"
	CursorTooLong      = "cursor must be at most 2 KiB"
	PageWalkSentence   = "pass next as cursor for more"
	SortNeedsSize      = "sort needs page_size"
	AsWrittenNeedsSize = "as_written needs page_size"
	SortUnsupported    = "sort applies to ClickHouse datasources; this datasource pages in its own order"

	MoreRowsNote = "more rows exist than were returned; repeat with page_size to read them in pages, or narrow from and to"
	NarrowNote   = "more rows exist than were returned; narrow from and to"
	FullPageNote = "the page is full and more rows exist; " + PageWalkSentence
	PartialNote  = "the datasource stopped before it searched everything, so some matches may be missing; narrow from and to or add a filter, then repeat"
)

func SeriesCapNote(shown, total int) string {
	return fmt.Sprintf("showing the %d of %d series with the highest peaks; narrow the query or add a filter to see the rest", shown, total)
}

func LimitNote(truncated, partial, hasNext, canPage bool) string {
	var notes []string
	switch {
	case hasNext:
		notes = append(notes, FullPageNote)
	case truncated && canPage:
		notes = append(notes, MoreRowsNote)
	case truncated:
		notes = append(notes, NarrowNote)
	}
	if partial {
		notes = append(notes, PartialNote)
	}
	return strings.Join(notes, ". ")
}

type PageParams struct {
	UID      string
	Query    datasource.Query
	Range    datasource.TimeRange
	Page     int
	PageSize int
	Cursor   string
	Sort     string

	AsWritten bool
}

type PageResult struct {
	Frames  []datasource.Frame
	Range   datasource.TimeRange
	Info    datasource.PageInfo
	HasPrev bool
	Next    string
	Prev    string
}

type pageError struct{ message string }

func (e *pageError) Error() string        { return e.message }
func (e *pageError) Is(target error) bool { return target == ErrInvalidRequest }

func invalidPage(message string) error { return &pageError{message: message} }

func (r *Runner) RunPage(ctx context.Context, inst datasource.Instance, p PageParams) (PageResult, error) {
	source, ok := inst.Datasource.(datasource.PageSource)
	if !ok {
		return PageResult{}, invalidPage(NotPageSource)
	}
	scope := datasource.CursorScope(p.UID, p.Query.Text)
	req := datasource.PageRequest{Query: p.Query, Range: p.Range, Page: max(p.Page, 1), PageSize: p.PageSize, AsWritten: p.AsWritten}
	sort, err := pageSort(source, p.Sort)
	if err != nil {
		return PageResult{}, err
	}
	req.Sort = sort
	if p.Cursor != "" {
		cursor, err := datasource.DecodeCursor(p.Cursor, scope)
		if err != nil {
			return PageResult{}, invalidPage(cursorMessage(err))
		}
		if cursor.Sort != p.Sort {
			return PageResult{}, invalidPage(cursorSortMessage(cursor.Sort))
		}
		req.Cursor, req.Page = cursor, cursor.Page
		req.Range = datasource.TimeRange{From: time.Unix(0, cursor.FromNs).UTC(), To: time.Unix(0, cursor.ToNs).UTC()}
	} else if req.Page > 1 && !source.PagesByOffset() {
		return PageResult{}, invalidPage(PageNeedsCursor)
	}
	release, err := r.AcquireUpstream(ctx, inst.UID)
	if err != nil {
		return PageResult{}, err
	}
	defer release()
	frames, info, err := source.QueryPage(ctx, req)
	datasource.NoteQuery(ctx, inst.UID, p.Query.Text, req.Range, datasource.FrameRows(frames))
	if err != nil {
		return PageResult{}, err
	}
	if frames == nil {
		frames = []datasource.Frame{}
	}
	res := PageResult{Frames: frames, Range: req.Range, Info: info, HasPrev: info.Page > 1}
	if info.Next != nil {
		next := *info.Next
		next.Sort = p.Sort
		next.Scope, next.FromNs, next.ToNs = scope, req.Range.From.UnixNano(), req.Range.To.UnixNano()
		res.Next = datasource.EncodeCursor(next)
	}
	if res.HasPrev {
		res.Prev = p.Cursor
	}
	return res, nil
}

func pageSort(source datasource.PageSource, text string) (*datasource.PageSort, error) {
	if text == "" {
		return nil, nil
	}
	sort, err := datasource.ParsePageSort(text)
	if err != nil {
		return nil, invalidPage(cursorMessage(err))
	}
	if sortable, ok := source.(datasource.SortablePages); !ok || !sortable.SortsPages() {
		return nil, invalidPage(SortUnsupported)
	}
	return sort, nil
}

func cursorSortMessage(made string) string {
	if made == "" {
		return "this cursor was made without a sort; leave sort out"
	}
	return "this cursor was made with sort " + made + "; send sort " + made + " with it"
}

func cursorMessage(err error) string {
	var upstream *datasource.UpstreamError
	if errors.As(err, &upstream) {
		return upstream.Message
	}
	return err.Error()
}
