package stream

import (
	"context"
	"encoding/json"
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
	maxAuditStreamsPerProc = 100
	auditPollInterval      = 2 * time.Second
	auditPingInterval      = 5 * time.Second
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
	if status, code := h.slots.acquire(key); status != 0 {
		abort(c, status, code, "too many open audit streams")
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
	if !write(c, fmt.Sprintf("retry: %d\n\n", retryMilliseconds)) || !writeEvent(c, "ready", struct{}{}) {
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

func (h *AuditHandler) pump(c *gin.Context, follow *trail.Follow, follower *events.Follower) {
	poll := time.NewTicker(h.pollInterval)
	defer poll.Stop()
	ping := time.NewTicker(h.pingInterval)
	defer ping.Stop()
	lifetime := time.NewTimer(h.lifetime)
	defer lifetime.Stop()
	failedChecks := 0
	if !h.drain(c, follow) {
		return
	}
	for {
		select {
		case <-poll.C:
			if !h.drain(c, follow) {
				return
			}
		case <-ping.C:
			still, failed := h.stillAdmin(c.Request)
			if failed {
				failedChecks++
			} else {
				failedChecks = 0
			}
			if !still {
				writeEvent(c, "forbidden", forbidden.NeedsRole(string(auth.RoleAdmin)))
				return
			}
			if failedChecks >= maxFailedChecks || !write(c, ": ping\n\n") {
				return
			}
		case <-lifetime.C:
			return
		case <-follower.Done():
			return
		case <-c.Request.Context().Done():
			return
		}
	}
}

func (h *AuditHandler) drain(c *gin.Context, follow *trail.Follow) bool {
	ctx := c.Request.Context()
	for {
		page, err := follow.Next(ctx)
		if err != nil {
			if ctx.Err() == nil {
				h.logger.ErrorContext(ctx, "read audit stream", logattrs.Failure("audit_stream_failed", err)...)
			}
			return false
		}
		for _, f := range page {
			body, err := json.Marshal(f.Event)
			if err != nil || !write(c, "id: "+f.ID+"\nevent: audit\ndata: "+string(body)+"\n\n") {
				return false
			}
		}
		if len(page) < trail.FollowBatch {
			return true
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
