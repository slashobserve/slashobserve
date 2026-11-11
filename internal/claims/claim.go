package claims

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const observePrompt = "observe"

type Offer struct {
	Run               *OfferedRun `json:"run"`
	Session           *RunSession `json:"session,omitempty"`
	LeaseSeconds      int         `json:"lease_seconds,omitempty"`
	SystemPrompt      *PromptRef  `json:"system_prompt,omitempty"`
	RetryAfterSeconds int         `json:"retry_after_seconds,omitempty"`
}

type OfferedRun struct {
	ID           string    `json:"id"`
	Attempt      int       `json:"attempt"`
	Kind         string    `json:"kind"`
	Source       string    `json:"source"`
	Prompt       string    `json:"prompt"`
	DashboardID  *string   `json:"dashboard_id"`
	BaseVersion  *int      `json:"base_version"`
	PanelID      *string   `json:"panel_id"`
	From         *string   `json:"from"`
	To           *string   `json:"to"`
	ContinueNote *string   `json:"continue_note"`
	RequestedBy  Requester `json:"requested_by"`
}

type Requester struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RunSession struct {
	URL       string `json:"url"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type PromptRef struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

func (s *Service) Claim(ctx context.Context, c Claimer, wait time.Duration) (Offer, error) {
	if err := s.eligible(ctx, c); err != nil {
		return Offer{}, err
	}
	wait = clampWait(wait)
	touchedAt := s.touch(ctx, c)
	woken := s.hub.wait()
	offer, found, err := s.tryClaim(ctx, c)
	if err != nil || found {
		return offer, err
	}
	key := c.claimant().Key()
	if wait == 0 || !s.hub.acquire(key) {
		return Offer{RetryAfterSeconds: int(retryAfter / time.Second)}, nil
	}
	defer s.hub.release(key)
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		poll := time.NewTimer(s.poll)
		select {
		case <-ctx.Done():
			poll.Stop()
			return Offer{}, ctx.Err()
		case <-deadline.C:
			poll.Stop()
			return Offer{}, nil
		case <-woken:
			poll.Stop()
		case <-poll.C:
		}
		woken = s.hub.wait()
		if time.Since(touchedAt) >= touchEvery {
			touchedAt = s.touch(ctx, c)
		}
		offer, found, err = s.tryClaim(ctx, c)
		if err != nil || found {
			return offer, err
		}
	}
}

func clampWait(wait time.Duration) time.Duration {
	return min(max(wait, 0), maxWait)
}

func (s *Service) tryClaim(ctx context.Context, c Claimer) (Offer, bool, error) {
	claimed, found, err := s.runs.Claim(ctx, c.claimant())
	if err != nil || !found {
		return Offer{}, false, err
	}
	offer, err := s.offer(ctx, claimed)
	return offer, err == nil, err
}

func (s *Service) offer(ctx context.Context, claimed revisions.Claimed) (Offer, error) {
	session, err := s.openSession(ctx, claimed.Row)
	if err != nil {
		s.logger.ErrorContext(ctx, "open run session; releasing the run", logattrs.Failure("run_session_failed", err, slog.String("agent_request_id", claimed.Row.ID.String()), slog.Int("attempt", int(claimed.Row.Attempts)))...)
		s.runs.Release(ctx, claimed.Row)
		return Offer{}, err
	}
	run := offeredRun(claimed.Request)
	return Offer{
		Run:          &run,
		Session:      &session,
		LeaseSeconds: int(revisions.LeaseTTL / time.Second),
		SystemPrompt: &PromptRef{Name: observePrompt, Arguments: map[string]string{"kind": run.Kind}},
	}, nil
}

func (s *Service) openSession(ctx context.Context, row dbsqlc.AgentRequest) (RunSession, error) {
	secret, hash, err := opaquetoken.New(tokenPrefix)
	if err != nil {
		return RunSession{}, err
	}
	expires := time.Now().Add(s.runTime())
	err = s.db.Queries().InsertRunToken(ctx, dbsqlc.InsertRunTokenParams{
		TokenHash: hash, AgentRequestID: row.ID, Attempt: row.Attempts,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		return RunSession{}, err
	}
	return RunSession{URL: s.publicURL.JoinPath("mcp").String(), Token: secret, ExpiresAt: expires.UTC().Format(time.RFC3339)}, nil
}

func offeredRun(req revisions.Request) OfferedRun {
	run := OfferedRun{
		ID:           req.ID,
		Attempt:      req.Attempt,
		Kind:         revisions.Kind(req.Source),
		Source:       string(req.Source),
		Prompt:       req.Prompt,
		DashboardID:  nonEmpty(req.DashboardID),
		PanelID:      nonEmpty(req.PanelID),
		ContinueNote: nonEmpty(req.ContinueNote),
		From:         formatTime(req.From),
		To:           formatTime(req.To),
		RequestedBy:  Requester{Kind: string(req.RequestedBy.Kind), ID: req.RequestedBy.ID, Name: req.RequestedBy.Name},
	}
	if req.BaseVersion > 0 {
		run.BaseVersion = &req.BaseVersion
	}
	return run
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func (s *Service) touch(ctx context.Context, c Claimer) time.Time {
	id, err := db.ParseUUID(c.ID)
	if err == nil {
		err = s.db.Queries().TouchAgentWorker(ctx, dbsqlc.TouchAgentWorkerParams{
			ID: c.claimant().Key(), PrincipalKind: string(c.Kind), PrincipalID: id, Client: cleanText(c.Client, maxClientRunes),
		})
	}
	if err != nil {
		s.logger.WarnContext(ctx, "record outside agent presence", logattrs.Failure("agent_worker_touch_failed", err, slog.String("claimer", c.claimant().Key()))...)
	}
	return time.Now()
}
