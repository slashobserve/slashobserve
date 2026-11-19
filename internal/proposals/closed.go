package proposals

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type Closure struct {
	ProposalID pgtype.UUID
	State      State
	Version    *int
	Reason     string
}

type ClosedNotifier interface {
	Closed(ctx context.Context, by auth.Principal, c Closure) error
}

func (s *Service) OnClosed(n ClosedNotifier) {
	s.notifier = n
}

func (s *Service) announceClosed(ctx context.Context, by auth.Principal, c Closure) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.Closed(ctx, by, c); err != nil {
		s.logger.WarnContext(ctx, "proposal closure not posted to its discussion",
			logattrs.Failure("proposal_closure_post_failed", err, slog.String("proposal_id", c.ProposalID.String()))...)
	}
}
