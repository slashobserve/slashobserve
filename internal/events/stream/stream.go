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
	roleCheckInterval   = 5 * time.Second
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

type Access interface {
	Dashboard(ctx context.Context, p auth.Principal, id pgtype.UUID) (folders.DashboardAccess, error)
}

type Handler struct {
	broker       *events.Broker
	dashboards   Dashboards
	principals   Principals
	access       Access
	view         gin.HandlerFunc
	logger       *slog.Logger
	pingInterval time.Duration
	roleInterval time.Duration
	lifetime     time.Duration
	slots        *slots
}

func NewHandler(b *events.Broker, d Dashboards, principals Principals, access Access, view gin.HandlerFunc, logger *slog.Logger) *Handler {
	return &Handler{broker: b, dashboards: d, principals: principals, access: access, view: view, logger: logger, pingInterval: pingInterval, roleInterval: roleCheckInterval, lifetime: maxStreamLifetime, slots: newSlots(maxStreamsPerUser, maxStreamsPerProc)}
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

type roleChangedData struct {
	DashboardID string    `json:"dashboard_id"`
	Role        auth.Role `json:"role"`
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
	if status, reason := h.slots.acquire(key); status != 0 {
		abortTooManyStreams(c, status, reason, tooManyEventStreams)
		return
	}
	defer h.slots.release(key)

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
	h.pump(c, sub, id, principal.Role)
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

func (h *Handler) pump(c *gin.Context, sub *events.Subscription, id pgtype.UUID, role auth.Role) {
	sayClosed(c, h.follow(c, sub, id, role))
}

func (h *Handler) follow(c *gin.Context, sub *events.Subscription, id pgtype.UUID, role auth.Role) closeReason {
	ping := time.NewTicker(h.pingInterval)
	defer ping.Stop()
	roleCheck := time.NewTicker(h.roleInterval)
	defer roleCheck.Stop()
	lifetime := time.NewTimer(h.lifetime)
	defer lifetime.Stop()
	failedChecks := 0
	for {
		select {
		case ev := <-sub.Events():
			if end := h.forward(c, id, ev); end != stillOpen {
				return end
			}
		case <-roleCheck.C:
			if !h.reportRoleChange(c, id, &role) {
				return unannounced
			}
		case <-ping.C:
			valid, failed := h.stillAllowed(c, id)
			if failed {
				failedChecks++
			} else {
				failedChecks = 0
			}
			if end := endAfterCheck(valid, failed, maxFailedChecks-failedChecks); end != stillOpen {
				return end
			}
			if !write(c, ": ping\n\n") {
				return unannounced
			}
		case <-lifetime.C:
			return closedLifetime
		case <-sub.Done():
			if sub.Dropped() {
				return closedSlowSubscriber
			}
			return closedStopping
		case <-c.Request.Context().Done():
			return unannounced
		}
	}
}

func (h *Handler) reportRoleChange(c *gin.Context, id pgtype.UUID, known *auth.Role) bool {
	now, ok := h.currentRole(c)
	if !ok || now == *known {
		return true
	}
	*known = now
	return writeEvent(c, events.TypeRoleChanged, roleChangedData{DashboardID: id.String(), Role: now})
}

func (h *Handler) currentRole(c *gin.Context) (auth.Role, bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sessionCheckTimeout)
	defer cancel()
	principal, ok, err := h.principals.RequestPrincipal(c.Request.WithContext(ctx))
	if err != nil || !ok {
		return "", false
	}
	return principal.Role, true
}

func (h *Handler) stillAllowed(c *gin.Context, id pgtype.UUID) (valid, failed bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sessionCheckTimeout)
	defer cancel()
	principal, ok, err := h.principals.RequestPrincipal(c.Request.WithContext(ctx))
	if err != nil {
		return h.checkFailed(c, "re-check event stream credentials", "stream_auth_check_failed", err)
	}
	if !ok {
		return false, false
	}
	access, err := h.access.Dashboard(ctx, principal, id)
	switch {
	case err == nil:
		return access.Abilities.View, false
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		return false, false
	}
	return h.checkFailed(c, "re-check event stream dashboard access", "stream_access_check_failed", err, slog.String("dashboard_id", id.String()))
}

func (h *Handler) forward(c *gin.Context, id pgtype.UUID, ev events.Event) closeReason {
	if ev.Type == events.TypeDashboardMoved {
		if end := h.lostWithTheMove(c, id); end != stillOpen {
			return end
		}
	}
	var data any = ev
	if ev.Type == events.TypeResync {
		data = resyncData{DashboardID: ev.DashboardID}
	}
	if !writeEvent(c, ev.Type, data) {
		return unannounced
	}
	return stillOpen
}

func (h *Handler) lostWithTheMove(c *gin.Context, id pgtype.UUID) closeReason {
	valid, failed := h.stillAllowed(c, id)
	return endAfterCheck(valid, failed, 0)
}

func (h *Handler) checkFailed(c *gin.Context, message, code string, err error, attrs ...any) (valid, failed bool) {
	if c.Request.Context().Err() == nil {
		h.logger.WarnContext(c.Request.Context(), message, logattrs.Failure(code, err, attrs...)...)
	}
	return c.Request.Context().Err() == nil, true
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
