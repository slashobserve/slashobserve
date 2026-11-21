package trail

import (
	"context"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	ActionSearch audit.Action = "audit.search"
	ActionExport audit.Action = "audit.export"

	viewEvents    = "events"
	viewFacets    = "facets"
	viewHistogram = "histogram"
	viewAggregate = "aggregate"
	viewEvent     = "event"
)

func windowData(p Prepared) map[string]any {
	return map[string]any{
		"form":  string(p.Query.Form),
		"since": p.Window.From.UTC().Format(time.RFC3339Nano),
		"until": p.Window.To.UTC().Format(time.RFC3339Nano),
		"query": audit.RecordedQuery(p.Text),
	}
}

func (r *Reader) Prepare(ctx context.Context, action audit.Action, req Request) (Prepared, error) {
	p, err := Prepare(req, r.now())
	if err != nil {
		if problem, ok := ProblemOf(err); ok {
			r.write(ctx, audit.Event{
				Action: action, Outcome: audit.OutcomeFailed, Reason: problem.Code, Status: problem.Status,
				Data: map[string]any{"query": audit.RecordedQuery(req.Text)},
			})
		}
		return Prepared{}, err
	}
	return p, nil
}

type read struct {
	view  string
	field string
}

func (v read) data() map[string]any {
	data := map[string]any{"view": v.view}
	if v.field != "" {
		data["field"] = v.field
	}
	return data
}

func (v read) subject(text string) string {
	return strings.Join([]string{v.view, v.field, text}, "\x00")
}

func (r *Reader) recordRead(ctx context.Context, p Prepared, v read, rows int) {
	if r.searches.SeenRecently(ctx, v.subject(p.Text), r.now()) {
		audit.Skip(ctx)
		return
	}
	data := v.data()
	data["row_count"] = rows
	r.record(ctx, p, audit.Event{Action: ActionSearch, Status: http.StatusOK, Data: data})
}

func (r *Reader) readFailed(ctx context.Context, p Prepared, v read, err error) error {
	if problem, ok := ProblemOf(err); ok {
		r.record(ctx, p, audit.Event{
			Action: ActionSearch, Outcome: audit.OutcomeFailed, Reason: problem.Code, Status: problem.Status,
			Data: v.data(),
		})
	}
	return err
}

func (r *Reader) writeFailed(ctx context.Context, v read, text string, err error) {
	if problem, ok := ProblemOf(err); ok {
		data := v.data()
		data["query"] = audit.RecordedQuery(text)
		r.write(ctx, audit.Event{Action: ActionSearch, Outcome: audit.OutcomeFailed, Reason: problem.Code, Status: problem.Status, Data: data})
	}
}

func (r *Reader) record(ctx context.Context, p Prepared, e audit.Event) {
	data := windowData(p)
	maps.Copy(data, e.Data)
	e.Data = data
	r.write(ctx, e)
}

func (r *Reader) write(ctx context.Context, e audit.Event) {
	if err := audit.RecordNow(ctx, e); err != nil {
		r.logger.WarnContext(ctx, "audit read not recorded", logattrs.Failure("audit_record_failed", err)...)
	}
}
