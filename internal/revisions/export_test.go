package revisions

import (
	"context"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (s *Service) SetLeaseTTL(d time.Duration) { s.leaseTTL = d }

func (w *Worker) SetHeartbeat(d time.Duration) { w.heartbeat = d }

func (s *Service) Claim(ctx context.Context) (dbsqlc.AgentRequest, bool, error) {
	claimed, found, err := s.claim(ctx, Claimant{Kind: AssignBuiltin})
	return claimed.request, found, err
}

func (s *Service) RenewLease(ctx context.Context, claimed dbsqlc.AgentRequest) error {
	_, err := s.renewLease(ctx, claimed)
	return err
}

func (s *Service) Release(ctx context.Context, claimed dbsqlc.AgentRequest, checkFailed bool) (bool, error) {
	return s.release(ctx, claimed, checkFailed)
}
