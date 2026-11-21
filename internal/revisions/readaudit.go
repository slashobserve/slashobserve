package revisions

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	actionRunView  audit.Action = "run.view"
	actionRunList  audit.Action = "run.list"
	actionChatView audit.Action = "chat.view"
	actionChatList audit.Action = "chat.list"
)

var everyonesList = map[string]any{"scope": string(ScopeAll)}

func (s *Service) recordRunView(ctx context.Context, row dbsqlc.AgentRequest, by auth.Principal) {
	if sameActor(requesterOf(row), by) {
		audit.Skip(ctx)
		return
	}
	s.recordFirstRead(ctx, audit.Event{
		Action: actionRunView,
		Target: audit.Target{Kind: "run", ID: row.ID.String(), DashboardID: nullUUID(row.DashboardID), ObservationID: nullUUID(row.ObservationID)},
		Chat:   nullUUID(row.ChatID),
		Data:   map[string]any{"owner_id": row.RequestedByID.String()},
	})
}

func (s *Service) recordChatView(ctx context.Context, chat Chat) {
	if !chat.ReadOnly {
		audit.Skip(ctx)
		return
	}
	s.recordFirstRead(ctx, audit.Event{
		Action: actionChatView,
		Target: audit.Target{Kind: "chat", ID: chat.ID.String(), DashboardID: nullUUID(chat.DashboardID), ObservationID: nullUUID(chat.ObservationID)},
		Chat:   nullUUID(chat.ID),
		Data:   map[string]any{"owner_id": chat.Owner.ID.String()},
	})
}

func (s *Service) recordRefusedBuildRead(ctx context.Context, run pgtype.UUID, err error) {
	if !errors.Is(err, ErrBuildNotFound) || errors.Is(err, ErrNoBuild) {
		return
	}
	reply, _ := ReplyFor(err)
	s.recordRead(ctx, audit.Event{
		Action:  actionRunView,
		Outcome: audit.OutcomeDenied,
		Reason:  reply.Code,
		Status:  reply.Status,
		Target:  audit.Target{Kind: "run", ID: run.String()},
	})
}

func (s *Service) recordEveryonesList(ctx context.Context, action audit.Action) {
	s.recordFirstRead(ctx, audit.Event{Action: action, Data: everyonesList})
}

func (s *Service) recordFirstRead(ctx context.Context, e audit.Event) {
	if s.reads.SeenRecently(ctx, string(e.Action)+":"+e.Target.ID, time.Now()) {
		audit.Skip(ctx)
		return
	}
	s.recordRead(ctx, e)
}

func (s *Service) recordRead(ctx context.Context, e audit.Event) {
	if err := audit.RecordNow(ctx, e); err != nil && !errors.Is(err, audit.ErrNotInstalled) {
		s.logger.WarnContext(ctx, "read not recorded", logattrs.Failure("audit_record_failed", err, slog.String("action", string(e.Action)))...)
	}
}
