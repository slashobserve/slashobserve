package trail

import (
	"context"
	"maps"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	ActionSearch audit.Action = "audit.search"
	ActionExport audit.Action = "audit.export"
	noAction     audit.Action = ""

	viewEvents            = "events"
	MaxRecordedQueryBytes = 4096
	truncatedMark         = " [truncated]"
)

func recordedQuery(text string) string {
	if len(text) <= MaxRecordedQueryBytes {
		return text
	}
	cut := MaxRecordedQueryBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + truncatedMark
}

func windowData(p Prepared) map[string]any {
	return map[string]any{
		"form":  string(p.Query.Form),
		"since": p.Window.From.UTC().Format(time.RFC3339Nano),
		"until": p.Window.To.UTC().Format(time.RFC3339Nano),
		"query": recordedQuery(p.Text),
	}
}

func (r *Reader) Prepare(ctx context.Context, action audit.Action, req Request) (Prepared, error) {
	p, err := Prepare(req, r.now())
	if err != nil {
		if problem, ok := ProblemOf(err); ok && action != noAction {
			r.write(ctx, audit.Event{
				Action: action, Outcome: audit.OutcomeFailed, Reason: problem.Code, Status: problem.Status,
				Data: map[string]any{"query": recordedQuery(req.Text)},
			})
		}
		return Prepared{}, err
	}
	return p, nil
}

func (r *Reader) searchFailed(ctx context.Context, p Prepared, err error) error {
	if problem, ok := ProblemOf(err); ok {
		r.record(ctx, p, audit.Event{
			Action: ActionSearch, Outcome: audit.OutcomeFailed, Reason: problem.Code, Status: problem.Status,
			Data: map[string]any{"view": viewEvents},
		})
	}
	return err
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
