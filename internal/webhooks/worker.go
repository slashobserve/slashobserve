package webhooks

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/sealed"
)

const (
	leaseTTL      = 60 * time.Second
	pollInterval  = 5 * time.Second
	maxConcurrent = 4
	pageSize      = 20
	maxPerClaim   = 50
	maxClaimTime  = 20 * time.Second
	finishTimeout = 5 * time.Second
)

var errLeaseLost = errors.New("webhook lease lost")

type Principals interface {
	Principal(ctx context.Context, kind auth.PrincipalKind, id string) (auth.Principal, error)
}

type Followers interface {
	Follow() (*events.Follower, error)
}

type WorkerDeps struct {
	DB         *db.DB
	Activity   *activity.Store
	Principals Principals
	Sender     Sender
	Key        sealed.Key
	Followers  Followers
	InstanceID string
	Logger     *slog.Logger
	Now        func() time.Time
}

type Worker struct {
	WorkerDeps
	source source
}

func NewWorker(d WorkerDeps) *Worker {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Worker{WorkerDeps: d, source: source{store: d.Activity}}
}

func (w *Worker) Run(ctx context.Context) {
	var wakes <-chan struct{}
	var gone <-chan struct{}
	if w.Followers != nil {
		follower, err := w.Followers.Follow()
		if err == nil {
			defer follower.Close()
			wakes, gone = follower.Wakes(), follower.Done()
		} else if !errors.Is(err, events.ErrClosed) {
			w.Logger.WarnContext(ctx, "webhook worker cannot follow activity", logattrs.Failure("follow_failed", err)...)
		}
	}
	timer := time.NewTicker(pollInterval)
	defer timer.Stop()
	for {
		w.pass(ctx)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-wakes:
		case <-gone:
			wakes, gone = nil, nil
		}
	}
}

func (w *Worker) pass(ctx context.Context) {
	claimed := []pgtype.UUID{}
	slots := make(chan struct{}, maxConcurrent)
	var running sync.WaitGroup
	defer running.Wait()
	for {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		row, err := w.DB.Queries().ClaimWebhook(ctx, dbsqlc.ClaimWebhookParams{
			InstanceID: pgtype.Text{String: w.InstanceID, Valid: true}, LeaseSeconds: leaseTTL.Seconds(), Done: claimed,
		})
		if err != nil {
			<-slots
			if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
				w.Logger.ErrorContext(ctx, "claim webhook", logattrs.Failure("webhook_claim_failed", err)...)
			}
			return
		}
		claimed = append(claimed, row.ID)
		running.Go(func() {
			defer func() { <-slots }()
			w.serve(ctx, row)
		})
	}
}

type claim struct {
	row      dbsqlc.Webhook
	failures int32
}

func (w *Worker) audienceOf(ctx context.Context, row dbsqlc.Webhook) (audience, bool) {
	owner, err := w.Principals.Principal(ctx, auth.PrincipalKind(row.OwnerKind), row.OwnerID.String())
	switch {
	case errors.Is(err, auth.ErrPrincipalGone) || (err == nil && owner.Role != auth.RoleAdmin):
		w.disable(ctx, row, ReasonOwner)
		return audience{}, false
	case err != nil:
		if ctx.Err() == nil {
			w.logFailure(ctx, "read webhook owner", "webhook_owner_failed", err, row)
		}
		return audience{}, false
	}
	got, err := w.source.audience(ctx, owner)
	if err != nil {
		if ctx.Err() == nil {
			w.logFailure(ctx, "read webhook audience", "webhook_audience_failed", err, row)
		}
		return audience{}, false
	}
	return got, true
}

func (w *Worker) serve(ctx context.Context, row dbsqlc.Webhook) {
	defer w.release(ctx, row)
	audience, ok := w.audienceOf(ctx, row)
	if !ok {
		return
	}
	c := &claim{row: row, failures: row.ConsecutiveFailures}
	at := cursor{xid: row.CursorXid.Uint64, seq: row.CursorSeq}
	started := time.Now()
	sent := 0
	for {
		page, to, err := w.source.next(ctx, audience, row.EventTypes, w.Now().Add(-expiry), at, pageSize)
		if err != nil {
			if ctx.Err() == nil {
				w.logFailure(ctx, "read activity for webhook", "webhook_read_failed", err, row)
			}
			return
		}
		for _, e := range page {
			if sent >= maxPerClaim || time.Since(started) >= maxClaimTime {
				return
			}
			if !w.send(ctx, c, e) {
				return
			}
			sent++
		}
		if to != at && !w.advanceTo(ctx, c, to) {
			return
		}
		at = to
		if len(page) < pageSize {
			return
		}
	}
}

func (w *Worker) send(ctx context.Context, c *claim, e activity.Event) bool {
	current, err := w.DB.Queries().RenewWebhookLease(ctx, dbsqlc.RenewWebhookLeaseParams{
		ID: c.row.ID, LeaseID: c.row.LeaseID, LeaseSeconds: leaseTTL.Seconds(),
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			w.logFailure(ctx, "renew webhook lease", "webhook_lease_failed", err, c.row)
		}
		return false
	}
	if current.OwnerKind != c.row.OwnerKind || current.OwnerID != c.row.OwnerID || !slices.Equal(current.EventTypes, c.row.EventTypes) {
		return false
	}
	secret, err := openSecret(w.Key, c.row.ID, current.SecretCiphertext)
	if err != nil {
		w.disable(ctx, c.row, ReasonSecretUnreadable)
		return false
	}
	body, deliveryID, err := eventPayload(c.row.ID.String(), e)
	if err != nil {
		w.logFailure(ctx, "encode webhook payload", "webhook_payload_failed", err, c.row)
		return false
	}
	got, err := deliver(ctx, w.Sender, w.Now(), current.Url, secret, deliveryID, e.Type, body)
	if err != nil {
		return false
	}
	return w.record(ctx, c, e, deliveryID, got)
}

func (w *Worker) record(ctx context.Context, c *claim, e activity.Event, deliveryID string, got attempt) bool {
	recordCtx, cancel := finishContext(ctx)
	defer cancel()
	failed := got.outcome == outcomeFailed
	failures := int32(0)
	var reason string
	err := w.DB.InTx(recordCtx, func(q *dbsqlc.Queries) error {
		if _, err := q.InsertWebhookDelivery(recordCtx, deliveryParams(c.row.ID, deliveryID, e.Type, pgtype.Int8{Int64: e.Seq, Valid: true}, got)); err != nil {
			return err
		}
		var changed int64
		var err error
		if failed {
			failures = c.failures + 1
			reason = disableReason(failures, got)
			changed, err = q.FailWebhook(recordCtx, dbsqlc.FailWebhookParams{
				ID: c.row.ID, LeaseID: c.row.LeaseID, ConsecutiveFailures: failures,
				NextAttemptAt:  pgtype.Timestamptz{Time: w.Now().Add(nextAttempt(int(failures), got.status, got.retryAfter)), Valid: true},
				DisabledAt:     pgtype.Timestamptz{Time: w.Now(), Valid: reason != ""},
				DisabledReason: pgtype.Text{String: reason, Valid: reason != ""},
			})
		} else {
			changed, err = q.AdvanceWebhook(recordCtx, dbsqlc.AdvanceWebhookParams{
				ID: c.row.ID, LeaseID: c.row.LeaseID,
				CursorXid: pgtype.Uint64{Uint64: cursorOf(e).xid, Valid: true}, CursorSeq: cursorOf(e).seq,
			})
		}
		if err != nil {
			return err
		}
		if changed == 0 {
			return errLeaseLost
		}
		return nil
	})
	if err != nil {
		if !errors.Is(err, errLeaseLost) {
			w.logFailure(ctx, "record webhook delivery", "webhook_record_failed", err, c.row)
		}
		return false
	}
	c.failures = failures
	if failed {
		w.Logger.WarnContext(ctx, "webhook delivery failed",
			slog.String("webhook_id", c.row.ID.String()), slog.Int("status_code", got.status), slog.String("error_code", got.errorCode))
		if reason != "" {
			w.Logger.InfoContext(ctx, "webhook disabled", slog.String("webhook_id", c.row.ID.String()), slog.String("reason", reason))
		}
	}
	return !failed
}

func disableReason(failures int32, got attempt) string {
	switch {
	case got.gone:
		return ReasonGone
	case failures >= failingAfter:
		return ReasonFailing
	}
	return ""
}

func (w *Worker) advanceTo(ctx context.Context, c *claim, to cursor) bool {
	recordCtx, cancel := finishContext(ctx)
	defer cancel()
	changed, err := w.DB.Queries().AdvanceWebhook(recordCtx, dbsqlc.AdvanceWebhookParams{
		ID: c.row.ID, LeaseID: c.row.LeaseID,
		CursorXid: pgtype.Uint64{Uint64: to.xid, Valid: true}, CursorSeq: to.seq,
	})
	if err != nil {
		w.logFailure(ctx, "advance webhook cursor", "webhook_advance_failed", err, c.row)
		return false
	}
	c.failures = 0
	return changed > 0
}

func (w *Worker) disable(ctx context.Context, row dbsqlc.Webhook, reason string) {
	disableCtx, cancel := finishContext(ctx)
	defer cancel()
	changed, err := w.DB.Queries().DisableLeasedWebhook(disableCtx, dbsqlc.DisableLeasedWebhookParams{
		ID: row.ID, LeaseID: row.LeaseID, Reason: pgtype.Text{String: reason, Valid: true},
	})
	if err != nil {
		w.logFailure(ctx, "disable webhook", "webhook_disable_failed", err, row)
		return
	}
	if changed > 0 {
		w.Logger.InfoContext(ctx, "webhook disabled", slog.String("webhook_id", row.ID.String()), slog.String("reason", reason))
	}
}

func (w *Worker) release(ctx context.Context, row dbsqlc.Webhook) {
	releaseCtx, cancel := finishContext(ctx)
	defer cancel()
	if _, err := w.DB.Queries().ReleaseWebhook(releaseCtx, dbsqlc.ReleaseWebhookParams{ID: row.ID, LeaseID: row.LeaseID}); err != nil {
		w.logFailure(ctx, "release webhook", "webhook_release_failed", err, row)
	}
}

func (w *Worker) logFailure(ctx context.Context, message, code string, err error, row dbsqlc.Webhook) {
	w.Logger.ErrorContext(ctx, message, logattrs.Failure(code, err, slog.String("webhook_id", row.ID.String()))...)
}

func finishContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
}
