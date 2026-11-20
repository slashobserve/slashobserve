package trail

import (
	"context"
	"maps"
	"net/http"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	actionSearch audit.Action = "audit.search"
	actionExport audit.Action = "audit.export"
	targetEvent               = "audit_event"
)

func windowData(p Prepared) map[string]any {
	return map[string]any{
		"form":  string(p.Query.Form),
		"since": p.Window.From.UTC().Format(time.RFC3339Nano),
		"until": p.Window.To.UTC().Format(time.RFC3339Nano),
	}
}

func (r *Reader) record(ctx context.Context, p Prepared, e audit.Event) {
	data := windowData(p)
	maps.Copy(data, e.Data)
	e.Data = data
	r.write(ctx, e)
}

func searched(extra map[string]any) audit.Event {
	return audit.Event{Action: actionSearch, Status: http.StatusOK, Data: extra}
}

func (r *Reader) recordEvent(ctx context.Context, d Detail) {
	r.write(ctx, audit.Event{
		Action: actionSearch, Status: http.StatusOK, Target: audit.Target{Kind: targetEvent, ID: d.Event.ID},
		Data: map[string]any{"view": "event", "row_count": 1 + len(d.SameRequest) + len(d.SameRun)},
	})
}

func (r *Reader) write(ctx context.Context, e audit.Event) {
	if err := audit.RecordNow(ctx, e); err != nil {
		r.logger.WarnContext(ctx, "audit read not recorded", logattrs.Failure("audit_record_failed", err)...)
	}
}
