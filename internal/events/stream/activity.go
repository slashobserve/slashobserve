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

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxActivityStreamsPerUser = 5
	maxActivityStreamsPerProc = 500
	activityBatch             = 100
	heldRetryInterval         = time.Second
)

type ActivityLog interface {
	Audience(ctx context.Context, p auth.Principal) (folders.Audience, error)
	Start(ctx context.Context, lastEventID string) (activity.Position, bool, error)
	Next(ctx context.Context, a folders.Audience, f activity.Filter, from activity.Position, limit int) ([]activity.Event, activity.Position, bool, error)
}

type Principals interface {
	RequestPrincipal(r *http.Request) (auth.Principal, bool, error)
}

type ActivityHandler struct {
	broker        *events.Broker
	log           ActivityLog
	principals    Principals
	logger        *slog.Logger
	pingInterval  time.Duration
	retryInterval time.Duration
	slots         *slots
}

func NewActivityHandler(b *events.Broker, log ActivityLog, principals Principals, logger *slog.Logger) *ActivityHandler {
	return &ActivityHandler{
		broker:        b,
		log:           log,
		principals:    principals,
		logger:        logger,
		pingInterval:  pingInterval,
		retryInterval: heldRetryInterval,
		slots:         newSlots(maxActivityStreamsPerUser, maxActivityStreamsPerProc),
	}
}

func (h *ActivityHandler) Register(api auth.Routes) {
	api.GET("/activity/stream", auth.RoleViewer, h.serve)
}

type activityReader struct {
	c        *gin.Context
	log      ActivityLog
	logger   *slog.Logger
	filter   activity.Filter
	audience folders.Audience
	from     activity.Position

	failedChecks int
}

func (h *ActivityHandler) serve(c *gin.Context) {
	ctx := c.Request.Context()
	filter, err := activity.ParseFilter(c.Request.URL.Query())
	if err != nil {
		abort(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	principal, _ := auth.FromContext(ctx)
	key := string(principal.Kind) + ":" + principal.ID
	if status, code := h.slots.acquire(key); status != 0 {
		abort(c, status, code, "too many open event streams")
		return
	}
	defer h.slots.release(key)

	follower, err := h.broker.Follow()
	if err != nil {
		abort(c, http.StatusServiceUnavailable, "shutting_down", "server is shutting down")
		return
	}
	defer follower.Close()

	from, reset, err := h.log.Start(ctx, c.GetHeader("Last-Event-ID"))
	if err != nil {
		h.refuse(c, "start activity stream", err)
		return
	}
	audience, err := h.log.Audience(ctx, principal)
	if err != nil {
		h.refuse(c, "read activity stream audience", err)
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
	if reset && !writeEvent(c, "reset", struct{}{}) {
		return
	}
	reader := &activityReader{c: c, log: h.log, logger: h.logger, filter: filter, audience: audience, from: from}
	h.pump(reader, follower)
}

func (h *ActivityHandler) refuse(c *gin.Context, action string, err error) {
	switch {
	case errors.Is(err, activity.ErrInvalidLastEventID):
		abort(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, folders.ErrForbidden):
		abort(c, http.StatusForbidden, forbidden.Code, forbidden.AccessMessage)
	case clientgone.Handle(c, h.logger, action, err):
	default:
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("activity_stream_failed", err)...)
		abort(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

func (h *ActivityHandler) pump(r *activityReader, follower *events.Follower) {
	ping := time.NewTicker(h.pingInterval)
	defer ping.Stop()
	lifetime := time.NewTimer(maxStreamLifetime)
	defer lifetime.Stop()
	var retry <-chan time.Time
	drain := func() bool {
		held, ok := r.drain()
		retry = nil
		if held {
			retry = time.After(h.retryInterval)
		}
		return ok
	}
	if !drain() {
		return
	}
	for {
		select {
		case <-follower.Wakes():
			if !drain() {
				return
			}
		case <-retry:
			if !drain() {
				return
			}
		case <-ping.C:
			if !h.ping(r) || !drain() {
				return
			}
		case <-lifetime.C:
			return
		case <-follower.Done():
			return
		case <-r.c.Request.Context().Done():
			return
		}
	}
}

func (r *activityReader) drain() (held, ok bool) {
	ctx := r.c.Request.Context()
	for {
		page, to, held, err := r.log.Next(ctx, r.audience, r.filter, r.from, activityBatch)
		if err != nil {
			if ctx.Err() == nil {
				r.logger.ErrorContext(ctx, "read activity stream", logattrs.Failure("activity_stream_failed", err)...)
			}
			return false, false
		}
		for _, e := range page {
			if !writeFrame(r.c, e.Seq, "activity", activity.JSONOf(e)) {
				return false, false
			}
		}
		r.from = to
		if len(page) < activityBatch {
			return held, true
		}
	}
}

func (h *ActivityHandler) ping(r *activityReader) bool {
	valid, failed := h.refresh(r)
	if failed {
		r.failedChecks++
	} else {
		r.failedChecks = 0
	}
	return valid && r.failedChecks < maxFailedChecks && write(r.c, ": ping\n\n")
}

func (h *ActivityHandler) refresh(r *activityReader) (valid, failed bool) {
	request := r.c.Request
	ctx, cancel := context.WithTimeout(request.Context(), sessionCheckTimeout)
	defer cancel()
	principal, ok, err := h.principals.RequestPrincipal(request.WithContext(ctx))
	if err != nil {
		return h.checkFailed(request, "re-check activity stream credentials", "stream_auth_check_failed", err)
	}
	if !ok {
		return false, false
	}
	audience, err := h.log.Audience(ctx, principal)
	switch {
	case err == nil:
		r.audience = audience
		return true, false
	case errors.Is(err, folders.ErrForbidden):
		return false, false
	}
	return h.checkFailed(request, "re-check activity stream access", "stream_access_check_failed", err)
}

func (h *ActivityHandler) checkFailed(r *http.Request, message, code string, err error) (valid, failed bool) {
	if r.Context().Err() == nil {
		h.logger.WarnContext(r.Context(), message, logattrs.Failure(code, err)...)
	}
	return r.Context().Err() == nil, true
}

func writeFrame(c *gin.Context, id int64, name string, data any) bool {
	body, err := json.Marshal(data)
	if err != nil {
		return false
	}
	return write(c, fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", id, name, body))
}
