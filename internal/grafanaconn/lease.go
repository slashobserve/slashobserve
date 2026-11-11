package grafanaconn

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	finishTimeout = 5 * time.Second

	exhaustedMessage   = "The server running this import stopped twice; start it again and it skips what it already imported."
	starterGoneMessage = "The person who started this import is no longer an Admin, so it cannot continue; start it again."
)

var (
	errLeaseLost = errors.New("grafana sync lease lost")
	errStopping  = errors.New("server stopping")
)

type held struct {
	row dbsqlc.GrafanaSync
	by  auth.Principal
	req SyncRequest
}

func (s *Service) drive(h held, client grafanaClient) {
	s.wg.Go(func() {
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		stop := context.AfterFunc(s.life, func() { cancel(errStopping) })
		defer stop()
		stopBeat := s.holdLease(ctx, h, cancel)
		err := s.work(ctx, h, client, cancel)
		stopBeat()
		s.settle(h, context.Cause(ctx), err)
	})
}

func (s *Service) settle(h held, cause, err error) {
	switch {
	case errors.Is(cause, errLeaseLost):
		s.logger.WarnContext(context.Background(), "grafana sync lease lost", idAttrs(h.row)...)
	case errors.Is(cause, errStopping):
		if s.moveHeld(h.row, ActionRelease, SyncRunning, "") {
			s.logMove(context.Background(), ActionRelease, SyncRunning, h.row.ConnectionID, h.row.ID)
		}
	case err != nil:
		message := describe(err)
		if message == internalMessage {
			s.logger.ErrorContext(context.Background(), "grafana sync", logattrs.Failure("internal", err, idAttrs(h.row)...)...)
		}
		s.failHeld(h.row, message)
	default:
		if s.moveHeld(h.row, ActionFinish, SyncFinished, "") {
			s.logFinished(h.row)
		}
	}
}

func (s *Service) failHeld(row dbsqlc.GrafanaSync, message string) {
	if s.moveHeld(row, ActionFail, SyncFailed, message) {
		s.logMove(context.Background(), ActionFail, SyncFailed, row.ConnectionID, row.ID)
	}
}

func (s *Service) logFinished(row dbsqlc.GrafanaSync) {
	ctx, cancel := context.WithTimeout(context.Background(), finishTimeout)
	defer cancel()
	var counts []slog.Attr
	done, err := s.store.db.Queries().GetGrafanaSync(ctx, dbsqlc.GetGrafanaSyncParams{ID: row.ID, ConnectionID: row.ConnectionID})
	if err != nil {
		s.logger.WarnContext(ctx, "read finished grafana sync", logattrs.Failure("grafana_sync_read_failed", err, idAttrs(row)...)...)
	} else {
		counts = []slog.Attr{slog.Int("total", int(done.Total)), slog.Int("done", int(done.Done)), slog.Int("failed", int(done.Failed)), slog.Int("skipped", int(done.Skipped))}
	}
	s.logMove(ctx, ActionFinish, SyncFinished, row.ConnectionID, row.ID, counts...)
}

func (s *Service) moveHeld(row dbsqlc.GrafanaSync, a SyncAction, to SyncState, message string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), finishTimeout)
	defer cancel()
	n, err := s.store.db.Queries().MoveHeldGrafanaSync(ctx, dbsqlc.MoveHeldGrafanaSyncParams{
		ToState: string(to), Error: textOf(message), ID: row.ID, LeaseID: row.LeaseID, FromStates: SyncMachine.From(a),
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "move grafana sync", logattrs.Failure("grafana_sync_move_failed", err, idAttrs(row)...)...)
		return false
	}
	return n > 0
}

func (s *Service) holdLease(ctx context.Context, h held, cancel context.CancelCauseFunc) (stop func()) {
	beatCtx, cancelBeat := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(s.renewEvery)
		defer ticker.Stop()
		renewedAt := time.Now()
		for {
			select {
			case <-beatCtx.Done():
				return
			case <-ticker.C:
			}
			renewCtx, cancelRenew := context.WithTimeout(beatCtx, s.leaseTTL)
			n, err := s.store.db.Queries().RenewGrafanaSyncLease(renewCtx, dbsqlc.RenewGrafanaSyncLeaseParams{
				LeaseSeconds: s.leaseTTL.Seconds(), ID: h.row.ID, LeaseID: h.row.LeaseID, FromStates: SyncMachine.From(ActionClaim),
			})
			cancelRenew()
			switch {
			case err == nil && n == 0:
				cancel(errLeaseLost)
				return
			case err == nil:
				renewedAt = time.Now()
			case beatCtx.Err() != nil:
				return
			case time.Since(renewedAt) > s.leaseTTL:
				cancel(errLeaseLost)
				return
			default:
				s.logger.WarnContext(ctx, "renew grafana sync lease; retrying at the next beat", logattrs.Failure("grafana_sync_lease_renew_failed", err, idAttrs(h.row)...)...)
			}
		}
	}()
	return func() {
		cancelBeat()
		<-done
	}
}

func (s *Service) work(ctx context.Context, h held, client grafanaClient, cancel context.CancelCauseFunc) error {
	p, err := s.plan(ctx, h.by, client, h.req)
	if err != nil {
		return err
	}
	q := s.store.db.Queries()
	uids, err := q.ListGrafanaSyncItemUIDs(ctx, h.row.ID)
	if err != nil {
		return err
	}
	recorded := make(map[string]bool, len(uids))
	for _, uid := range uids {
		recorded[uid] = true
	}
	hits := remainingHits(uniqueHits(p.hits), recorded)
	n, err := q.SetGrafanaSyncTotal(ctx, dbsqlc.SetGrafanaSyncTotalParams{
		Total: httpapi.Int32(len(recorded) + len(hits)), ID: h.row.ID, LeaseID: h.row.LeaseID, FromStates: SyncMachine.From(ActionClaim),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		cancel(errLeaseLost)
		return nil
	}
	for f := range fetchAll(ctx, client, hits) {
		res := s.importOne(ctx, h.by, p, f)
		if ctx.Err() != nil {
			break
		}
		ok, err := s.record(ctx, h, res)
		if err != nil && ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "record grafana sync item", logattrs.Failure("grafana_sync_item_failed", err, idAttrs(h.row)...)...)
		}
		if !ok || err != nil {
			cancel(errLeaseLost)
			break
		}
	}
	return nil
}

func (s *Service) record(ctx context.Context, h held, res SyncResult) (bool, error) {
	params, err := itemParams(h.row.ID, res)
	if err != nil {
		return false, err
	}
	holds := true
	err = s.store.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		_, err := q.LockHeldGrafanaSync(ctx, dbsqlc.LockHeldGrafanaSyncParams{ID: h.row.ID, LeaseID: h.row.LeaseID, FromStates: SyncMachine.From(ActionClaim)})
		if errors.Is(err, pgx.ErrNoRows) {
			holds = false
			return nil
		}
		if err != nil {
			return err
		}
		inserted, err := q.InsertGrafanaSyncItem(ctx, params)
		if err != nil || inserted == 0 {
			return err
		}
		_, err = q.CountGrafanaSyncItem(ctx, dbsqlc.CountGrafanaSyncItemParams{Outcome: res.Outcome, ID: h.row.ID})
		return err
	})
	return holds, err
}

func (s *Service) Start() {
	s.wg.Go(func() { s.resumeLoop(s.life) })
}

func (s *Service) resumeLoop(ctx context.Context) {
	s.resumePass(ctx)
	ticker := time.NewTicker(s.resumeEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.resumePass(ctx)
		}
	}
}

func (s *Service) resumePass(ctx context.Context) {
	q := s.store.db.Queries()
	exhausted, err := q.ExhaustGrafanaSyncs(ctx, dbsqlc.ExhaustGrafanaSyncsParams{
		ToState: string(SyncFailed), Error: exhaustedMessage, FromStates: SyncMachine.From(ActionExhaust),
	})
	if err != nil {
		if ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "fail grafana syncs that lost their lease twice", logattrs.Failure("grafana_sync_exhaust_failed", err)...)
		}
		return
	}
	for _, row := range exhausted {
		s.logMove(ctx, ActionExhaust, SyncFailed, row.ConnectionID, row.ID)
	}
	for ctx.Err() == nil {
		row, err := q.ClaimGrafanaSync(ctx, dbsqlc.ClaimGrafanaSyncParams{
			InstanceID: s.instanceID, LeaseSeconds: s.leaseTTL.Seconds(), FromStates: SyncMachine.From(ActionClaim),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}
		if err != nil {
			if ctx.Err() == nil {
				s.logger.ErrorContext(ctx, "claim grafana sync", logattrs.Failure("grafana_sync_claim_failed", err)...)
			}
			return
		}
		s.resume(ctx, row)
	}
}

func (s *Service) resume(ctx context.Context, row dbsqlc.GrafanaSync) {
	req, err := decodeRequest(row.Request)
	if err != nil {
		s.logger.ErrorContext(ctx, "read grafana sync request", logattrs.Failure("grafana_sync_request_unreadable", err, idAttrs(row)...)...)
		s.failHeld(row, describe(err))
		return
	}
	by, err := s.principals.Principal(ctx, auth.PrincipalKind(row.StartedByKind.String), row.StartedByID.String())
	switch {
	case errors.Is(err, auth.ErrPrincipalGone) || (err == nil && by.Role != auth.RoleAdmin):
		s.failHeld(row, starterGoneMessage)
		return
	case err != nil:
		s.logResumeFailure(ctx, "read grafana sync starter", "grafana_sync_starter_failed", err, row)
		return
	}
	if err := s.checkMapping(req.Mapping); err != nil {
		s.failHeld(row, describe(err))
		return
	}
	_, client, err := s.client(ctx, row.ConnectionID)
	switch {
	case errors.Is(err, ErrNoSecretKey), errors.Is(err, ErrUnreadableToken):
		s.failHeld(row, describe(err))
		return
	case errors.Is(err, ErrNotFound):
		return
	case err != nil:
		s.logResumeFailure(ctx, "open grafana connection to resume a sync", "grafana_sync_connection_failed", err, row)
		return
	}
	s.logMove(ctx, ActionClaim, SyncRunning, row.ConnectionID, row.ID, slog.Int("lease_losses", int(row.LeaseLosses)))
	s.drive(held{row: row, by: by, req: req}, client)
}

func (s *Service) logResumeFailure(ctx context.Context, message, code string, err error, row dbsqlc.GrafanaSync) {
	if ctx.Err() == nil {
		s.logger.ErrorContext(ctx, message, logattrs.Failure(code, err, idAttrs(row)...)...)
	}
}
