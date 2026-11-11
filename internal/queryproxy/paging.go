package queryproxy

import (
	"context"
	"errors"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	NotPageSource    = "paging reads logs and trace datasources"
	PageNeedsCursor  = "this datasource pages by cursor; pass the cursor of the previous page"
	PagingIsInstant  = "paging applies to instant queries"
	LimitWithPage    = "limit and page_size cannot both be given"
	PageNeedsSize    = "page and cursor need page_size"
	PageSizeOutside  = "page_size must be from 50 to 500"
	PageBelowOne     = "page must be at least 1"
	CursorTooLong    = "cursor must be at most 2 KiB"
	PageWalkSentence = "pass next as cursor for more"
)

type PageParams struct {
	UID      string
	Query    datasource.Query
	Range    datasource.TimeRange
	Page     int
	PageSize int
	Cursor   string
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
	req := datasource.PageRequest{Query: p.Query, Range: p.Range, Page: max(p.Page, 1), PageSize: p.PageSize}
	if p.Cursor != "" {
		cursor, err := datasource.DecodeCursor(p.Cursor, scope)
		if err != nil {
			return PageResult{}, invalidPage(cursorMessage(err))
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
	if err != nil {
		return PageResult{}, err
	}
	if frames == nil {
		frames = []datasource.Frame{}
	}
	res := PageResult{Frames: frames, Range: req.Range, Info: info, HasPrev: info.Page > 1}
	if info.Next != nil {
		next := *info.Next
		next.Scope, next.FromNs, next.ToNs = scope, req.Range.From.UnixNano(), req.Range.To.UnixNano()
		res.Next = datasource.EncodeCursor(next)
	}
	if res.HasPrev {
		res.Prev = p.Cursor
	}
	return res, nil
}

func cursorMessage(err error) string {
	var upstream *datasource.UpstreamError
	if errors.As(err, &upstream) {
		return upstream.Message
	}
	return err.Error()
}
