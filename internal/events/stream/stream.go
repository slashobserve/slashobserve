package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	pingInterval        = 15 * time.Second
	maxStreamLifetime   = 30 * time.Minute
	retryMilliseconds   = 3000
	maxStreamsPerUser   = 20
	maxStreamsPerProc   = 1000
	uuidTextBytes       = 36
	eventStreamContent  = "text/event-stream"
	writeTimeout        = 30 * time.Second
	sessionCheckTimeout = 5 * time.Second
	maxFailedChecks     = 2
)

type Dashboards interface {
	LatestVersion(ctx context.Context, id pgtype.UUID) (int, error)
}

type Sessions interface {
	Valid(r *http.Request) (bool, error)
}

type Access interface {
	Dashboard(ctx context.Context, p auth.Principal, id pgtype.UUID) (folders.DashboardAccess, error)
}

type Handler struct {
	broker       *events.Broker
	dashboards   Dashboards
	sessions     Sessions
	access       Access
	view         gin.HandlerFunc
	logger       *slog.Logger
	pingInterval time.Duration

	mu          sync.Mutex
	total       int
	byPrincipal map[string]int
}

func NewHandler(b *events.Broker, d Dashboards, sessions Sessions, access Access, view gin.HandlerFunc, logger *slog.Logger) *Handler {
	return &Handler{broker: b, dashboards: d, sessions: sessions, access: access, view: view, logger: logger, pingInterval: pingInterval, byPrincipal: map[string]int{}}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/dashboards/:id/events", auth.RoleViewer, h.view, h.serve)
}

type readyData struct {
	DashboardID   string `json:"dashboard_id"`
	LatestVersion int    `json:"latest_version"`
}

type resyncData struct {
	DashboardID string `json:"dashboard_id"`
}

func (h *Handler) serve(c *gin.Context) {
	raw := c.Param("id")
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil || len(raw) != uuidTextBytes {
		abort(c, http.StatusBadRequest, "invalid_request", "id must be a UUID")
		return
	}
	principal, _ := auth.FromContext(c.Request.Context())
	key := string(principal.Kind) + ":" + principal.ID
	if status, code := h.acquire(key); status != 0 {
		abort(c, status, code, "too many open event streams")
		return
	}
	defer h.release(key)

	dashboardID := id.String()
	sub, err := h.broker.Subscribe(dashboardID)
	if err != nil {
		abort(c, http.StatusServiceUnavailable, "shutting_down", "server is shutting down")
		return
	}
	defer sub.Close()

	latest, ok := h.latestVersion(c, id)
	if !ok {
		return
	}

	header := c.Writer.Header()
	header.Set("Content-Type", eventStreamContent)
	header.Set("Cache-Control", "no-store")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	defer func() { _ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}) }()
	if !write(c, fmt.Sprintf("retry: %d\n\n", retryMilliseconds)) ||
		!writeEvent(c, "ready", readyData{DashboardID: dashboardID, LatestVersion: latest}) {
		return
	}
	h.pump(c, sub, id)
}

func (h *Handler) latestVersion(c *gin.Context, id pgtype.UUID) (int, bool) {
	latest, err := h.dashboards.LatestVersion(c.Request.Context(), id)
	switch {
	case err == nil:
		return latest, true
	case clientgone.Is(c.Request.Context(), err):
		clientgone.Abort(c, h.logger, "read latest version for event stream")
	case errors.Is(err, dashboards.ErrNotFound):
		abort(c, http.StatusNotFound, "not_found", err.Error())
	default:
		h.logger.ErrorContext(c.Request.Context(), "read latest version for event stream", logattrs.Failure("latest_version_failed", err, slog.String("dashboard_id", id.String()))...)
		abort(c, http.StatusInternalServerError, "internal", "internal error")
	}
	return 0, false
}

func (h *Handler) pump(c *gin.Context, sub *events.Subscription, id pgtype.UUID) {
	ping := time.NewTicker(h.pingInterval)
	defer ping.Stop()
	lifetime := time.NewTimer(maxStreamLifetime)
	defer lifetime.Stop()
	failedChecks := 0
	for {
		select {
		case ev := <-sub.Events():
			var data any = ev
			if ev.Type == events.TypeResync {
				data = resyncData{DashboardID: ev.DashboardID}
			}
			if !writeEvent(c, ev.Type, data) {
				return
			}
		case <-ping.C:
			valid, failed := h.stillAllowed(c, id)
			if failed {
				failedChecks++
			} else {
				failedChecks = 0
			}
			if !valid || failedChecks >= maxFailedChecks || !write(c, ": ping\n\n") {
				return
			}
		case <-lifetime.C:
			return
		case <-sub.Done():
			return
		case <-c.Request.Context().Done():
			return
		}
	}
}

func (h *Handler) credentialsValid(c *gin.Context) (valid, failed bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sessionCheckTimeout)
	defer cancel()
	ok, err := h.sessions.Valid(c.Request.WithContext(ctx))
	if err != nil {
		if c.Request.Context().Err() == nil {
			h.logger.WarnContext(c.Request.Context(), "re-check event stream credentials", logattrs.Failure("stream_auth_check_failed", err)...)
		}
		return c.Request.Context().Err() == nil, true
	}
	return ok, false
}

func (h *Handler) stillAllowed(c *gin.Context, id pgtype.UUID) (valid, failed bool) {
	valid, failed = h.credentialsValid(c)
	if !valid || failed {
		return valid, failed
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), sessionCheckTimeout)
	defer cancel()
	principal, _ := auth.FromContext(c.Request.Context())
	access, err := h.access.Dashboard(ctx, principal, id)
	switch {
	case err == nil:
		return access.Abilities.View, false
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		return false, false
	}
	if c.Request.Context().Err() == nil {
		h.logger.WarnContext(c.Request.Context(), "re-check event stream dashboard access", logattrs.Failure("stream_access_check_failed", err, slog.String("dashboard_id", id.String()))...)
	}
	return c.Request.Context().Err() == nil, true
}

func (h *Handler) acquire(key string) (int, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byPrincipal[key] >= maxStreamsPerUser {
		return http.StatusTooManyRequests, "too_many_streams"
	}
	if h.total >= maxStreamsPerProc {
		return http.StatusServiceUnavailable, "too_many_streams"
	}
	h.byPrincipal[key]++
	h.total++
	return 0, ""
}

func (h *Handler) release(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.total--
	if h.byPrincipal[key]--; h.byPrincipal[key] <= 0 {
		delete(h.byPrincipal, key)
	}
}

func writeEvent(c *gin.Context, name string, data any) bool {
	body, err := json.Marshal(data)
	if err != nil {
		return false
	}
	return write(c, fmt.Sprintf("event: %s\ndata: %s\n\n", name, body))
}

func write(c *gin.Context, s string) bool {
	rc := http.NewResponseController(c.Writer)
	if err := rc.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return false
	}
	if _, err := c.Writer.WriteString(s); err != nil {
		return false
	}
	c.Writer.Flush()
	return true
}

func abort(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code})
}
