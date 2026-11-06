package revisions

import (
	"context"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (s *Service) SetLeaseTTL(d time.Duration) { s.leaseTTL = d }

func (w *Worker) SetHeartbeat(d time.Duration) { w.heartbeat = d }

func (s *Service) Claim(ctx context.Context) (dbsqlc.AgentRequest, bool, error) {
	return s.claim(ctx)
}

func (s *Service) RenewLease(ctx context.Context, claimed dbsqlc.AgentRequest) error {
	_, err := s.renewLease(ctx, claimed)
	return err
}
