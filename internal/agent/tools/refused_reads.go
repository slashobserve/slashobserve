package tools

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	actionRunView  audit.Action = "run.view"
	actionChatView audit.Action = "chat.view"
	actionRunQueue audit.Action = "run.queue"
)

func (r *Registry) recordRefusedRead(ctx context.Context, action audit.Action, kind, id string, err error) {
	reply, ok := revisions.ReplyFor(err)
	if !ok || (reply.Status != http.StatusForbidden && reply.Status != http.StatusNotFound) {
		return
	}
	r.recordRefusal(ctx, audit.Event{
		Action:  action,
		Outcome: audit.OutcomeDenied,
		Reason:  reply.Code,
		Status:  reply.Status,
		Target:  audit.Target{Kind: kind, ID: id},
	})
}

func (r *Registry) recordRefusedStart(ctx context.Context, err error) {
	reply := revisions.StartReplyFor(err)
	if clientgone.Is(ctx, err) {
		reply = revisions.Reply{Status: clientgone.Status}
	}
	r.recordRefusal(ctx, audit.Event{
		Action:  actionRunQueue,
		Outcome: audit.OutcomeForStatus(reply.Status),
		Reason:  reply.Code,
		Status:  reply.Status,
		Target:  audit.Target{Kind: "run"},
	})
}

func (r *Registry) recordRefusal(ctx context.Context, e audit.Event) {
	recorded := audit.RecordNow(context.WithoutCancel(ctx), e)
	if recorded != nil && !errors.Is(recorded, audit.ErrNotInstalled) {
		r.deps.Logger.WarnContext(ctx, "refusal not recorded", logattrs.Failure("audit_record_failed", recorded, slog.String("action", string(e.Action)))...)
	}
}
