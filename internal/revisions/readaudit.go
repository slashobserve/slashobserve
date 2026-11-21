package revisions

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	actionRunView  audit.Action = "run.view"
	actionRunList  audit.Action = "run.list"
	actionChatView audit.Action = "chat.view"
	actionChatList audit.Action = "chat.list"

	dashboardRunsScope = "dashboard"
)

func (s *Service) recordRunView(ctx context.Context, row dbsqlc.AgentRequest) {
	s.recordFirstRead(ctx, row.ID.String(), audit.Event{
		Action: actionRunView,
		Target: audit.Target{Kind: "run", ID: row.ID.String(), DashboardID: nullUUID(row.DashboardID), ObservationID: nullUUID(row.ObservationID)},
		Chat:   nullUUID(row.ChatID),
		Data:   map[string]any{"owner_id": row.RequestedByID.String()},
	})
}

func (s *Service) RecordAnswerRead(ctx context.Context, runID pgtype.UUID) {
	row, err := s.GetByID(ctx, runID)
	if err != nil {
		s.logger.WarnContext(ctx, "answer read not recorded", logattrs.Failure("audit_record_failed", err, slog.String("agent_request_id", runID.String()))...)
		return
	}
	s.recordRunView(ctx, row)
}

func (s *Service) recordChatView(ctx context.Context, chat Chat) {
	s.recordFirstRead(ctx, chat.ID.String(), audit.Event{
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

func (s *Service) recordList(ctx context.Context, action audit.Action, scope ListScope) {
	s.recordFirstRead(ctx, string(scope), audit.Event{Action: action, Data: map[string]any{"scope": string(scope)}})
}

func (s *Service) recordDashboardRunList(ctx context.Context, dashboardID pgtype.UUID) {
	s.recordFirstRead(ctx, dashboardRunsScope+":"+dashboardID.String(), audit.Event{
		Action: actionRunList,
		Target: audit.Target{Kind: "dashboard", ID: dashboardID.String(), DashboardID: nullUUID(dashboardID)},
		Data:   map[string]any{"scope": dashboardRunsScope},
	})
}

func (s *Service) recordFirstRead(ctx context.Context, subject string, e audit.Event) {
	if s.reads.SeenRecently(ctx, string(e.Action)+":"+subject, time.Now()) {
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
