package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxAuditStreamsPerUser = 5
	maxAuditStreamsPerProc = 20
	auditPollInterval      = 2 * time.Second
	auditPingInterval      = 5 * time.Second
	maxAuditPollTimeouts   = 2
	codeTooExpensive       = "query_too_expensive"
)

var (
	tooManyAuditStreams = fmt.Sprintf("too many open audit streams: a caller may hold %d and the server %d; close one and try again", maxAuditStreamsPerUser, maxAuditStreamsPerProc)
	tooExpensive        = fmt.Sprintf("the query is too expensive to follow: %d polls in a row ran past %s; narrow it, or read the events with a search", maxAuditPollTimeouts, trail.FollowTimeout)
)

type AuditTrail interface {
	Follow(ctx context.Context, text, lastEventID string) (*trail.Follow, error)
}

type AuditHandler struct {
	broker       *events.Broker
	trail        AuditTrail
	principals   Principals
	logger       *slog.Logger
	pollInterval time.Duration
	pingInterval time.Duration
	lifetime     time.Duration
	slots        *slots
}

func NewAuditHandler(b *events.Broker, t AuditTrail, principals Principals, logger *slog.Logger) *AuditHandler {
	return &AuditHandler{
		broker:       b,
		trail:        t,
		principals:   principals,
		logger:       logger,
		pollInterval: auditPollInterval,
		pingInterval: auditPingInterval,
		lifetime:     maxStreamLifetime,
		slots:        newSlots(maxAuditStreamsPerUser, maxAuditStreamsPerProc),
	}
}

func (h *AuditHandler) Register(api auth.Routes) {
	api.GET("/audit/stream", auth.RoleAdmin, h.serve)
}

func (h *AuditHandler) serve(c *gin.Context) {
	ctx := c.Request.Context()
	principal, _ := auth.FromContext(ctx)
	key := string(principal.Kind) + ":" + principal.ID
	if status, reason := h.slots.acquire(key); status != 0 {
		abortTooManyStreams(c, http.StatusTooManyRequests, reason, tooManyAuditStreams)
		return
	}
	defer h.slots.release(key)

	follower, err := h.broker.Follow()
	if err != nil {
		abort(c, http.StatusServiceUnavailable, "shutting_down", "server is shutting down")
		return
	}
	defer follower.Close()

	follow, err := h.trail.Follow(ctx, c.Query("q"), c.GetHeader("Last-Event-ID"))
	if err != nil {
		h.refuse(c, err)
		return
	}

	header := c.Writer.Header()
	header.Set("Content-Type", eventStreamContent)
	header.Set("Cache-Control", "no-store")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	defer func() { _ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}) }()
	if !write(c, fmt.Sprintf("retry: %d\n\n", retryMilliseconds)) || !write(c, "id: "+follow.Start()+"\nevent: ready\ndata: {}\n\n") {
		return
	}
	h.pump(c, follow, follower)
}

func (h *AuditHandler) refuse(c *gin.Context, err error) {
	if problem, ok := trail.ProblemOf(err); ok {
		c.AbortWithStatusJSON(problem.Status, problem)
		return
	}
	if clientgone.Handle(c, h.logger, "open audit stream", err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), "open audit stream", logattrs.Failure("audit_stream_failed", err)...)
	abort(c, http.StatusInternalServerError, "internal", "internal error")
}

type auditPump struct {
	h            *AuditHandler
	c            *gin.Context
	follow       *trail.Follow
	timeouts     int
	failedChecks int
}

func (h *AuditHandler) pump(c *gin.Context, follow *trail.Follow, follower *events.Follower) {
	sayClosed(c, h.follow(c, follow, follower))
}

func (h *AuditHandler) follow(c *gin.Context, follow *trail.Follow, follower *events.Follower) closeReason {
	poll := time.NewTicker(h.pollInterval)
	defer poll.Stop()
	ping := time.NewTicker(h.pingInterval)
	defer ping.Stop()
	lifetime := time.NewTimer(h.lifetime)
	defer lifetime.Stop()
	p := &auditPump{h: h, c: c, follow: follow}
	if end := p.poll(); end != stillOpen {
		return end
	}
	for {
		select {
		case <-poll.C:
			if end := p.poll(); end != stillOpen {
				return end
			}
		case <-ping.C:
			if end := p.ping(); end != stillOpen {
				return end
			}
		case <-lifetime.C:
			return closedLifetime
		case <-follower.Done():
			return closedStopping
		case <-c.Request.Context().Done():
			return unannounced
		}
	}
}

func (p *auditPump) poll() closeReason {
	err := p.h.drain(p.c, p.follow)
	switch {
	case err == nil:
		p.timeouts = 0
		return stillOpen
	case errors.Is(err, trail.ErrTimeout):
		if p.timeouts++; p.timeouts < maxAuditPollTimeouts {
			return stillOpen
		}
		writeEvent(p.c, "error", trail.Problem{Code: codeTooExpensive, Message: tooExpensive})
		return unannounced
	case errors.Is(err, errWriteFailed), p.c.Request.Context().Err() != nil:
		return unannounced
	}
	p.h.logger.ErrorContext(p.c.Request.Context(), "read audit stream", logattrs.Failure("audit_stream_failed", err)...)
	return closedReadFailed
}

func (p *auditPump) ping() closeReason {
	still, failed := p.h.stillAdmin(p.c.Request)
	if failed {
		p.failedChecks++
	} else {
		p.failedChecks = 0
	}
	if !still {
		writeEvent(p.c, "forbidden", forbidden.NeedsRole(string(auth.RoleAdmin)))
		return unannounced
	}
	if p.failedChecks >= maxFailedChecks {
		return closedCheckFailed
	}
	if !write(p.c, ": ping\n\n") {
		return unannounced
	}
	return stillOpen
}

var errWriteFailed = errors.New("write audit stream frame")

func (h *AuditHandler) drain(c *gin.Context, follow *trail.Follow) error {
	for {
		page, err := follow.Next(c.Request.Context())
		if err != nil {
			return err
		}
		for _, f := range page {
			body, err := json.Marshal(f.Event)
			if err != nil {
				return err
			}
			if !write(c, "id: "+f.ID+"\nevent: audit\ndata: "+string(body)+"\n\n") {
				return errWriteFailed
			}
		}
		if len(page) < trail.FollowBatch {
			return nil
		}
	}
}

func (h *AuditHandler) stillAdmin(r *http.Request) (still, failed bool) {
	still, err := trail.StillAdmin(r, h.principals)
	if err != nil {
		if r.Context().Err() == nil {
			h.logger.WarnContext(r.Context(), "re-check audit stream credentials", logattrs.Failure("stream_auth_check_failed", err)...)
		}
		return r.Context().Err() == nil, true
	}
	return still, false
}
