package tools

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	actionRunView  audit.Action = "run.view"
	actionChatView audit.Action = "chat.view"
)

func (r *Registry) recordRefusedRead(ctx context.Context, action audit.Action, kind, id string, err error) {
	reply, ok := revisions.ReplyFor(err)
	if !ok || (reply.Status != http.StatusForbidden && reply.Status != http.StatusNotFound) {
		return
	}
	recorded := audit.RecordNow(ctx, audit.Event{
		Action:  action,
		Outcome: audit.OutcomeDenied,
		Reason:  reply.Code,
		Status:  reply.Status,
		Target:  audit.Target{Kind: kind, ID: id},
	})
	if recorded != nil && !errors.Is(recorded, audit.ErrNotInstalled) {
		r.deps.Logger.WarnContext(ctx, "refused read not recorded", logattrs.Failure("audit_record_failed", recorded, slog.String("action", string(action)))...)
	}
}
