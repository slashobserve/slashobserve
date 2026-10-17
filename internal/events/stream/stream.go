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
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/events"
)

const (
	pingInterval       = 15 * time.Second
	maxStreamLifetime  = 30 * time.Minute
	retryMilliseconds  = 3000
	maxStreamsPerUser  = 20
	maxStreamsPerProc  = 1000
	uuidTextBytes      = 36
	eventStreamContent = "text/event-stream"
	writeTimeout       = 30 * time.Second
)

type Dashboards interface {
	LatestVersion(ctx context.Context, id pgtype.UUID) (int, error)
}

type Handler struct {
	broker     *events.Broker
	dashboards Dashboards
	logger     *slog.Logger

	mu          sync.Mutex
	total       int
	byPrincipal map[string]int
}

func NewHandler(b *events.Broker, d Dashboards, logger *slog.Logger) *Handler {
	return &Handler{broker: b, dashboards: d, logger: logger, byPrincipal: map[string]int{}}
}

func (h *Handler) Register(api *gin.RouterGroup) {
	api.GET("/dashboards/:id/events", h.serve)
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
	pump(c, sub)
}

func (h *Handler) latestVersion(c *gin.Context, id pgtype.UUID) (int, bool) {
	latest, err := h.dashboards.LatestVersion(c.Request.Context(), id)
	switch {
	case err == nil:
		return latest, true
	case errors.Is(err, context.Canceled):
		c.Abort()
	case errors.Is(err, dashboards.ErrNotFound):
		abort(c, http.StatusNotFound, "not_found", err.Error())
	default:
		h.logger.ErrorContext(c.Request.Context(), "read latest version for event stream", slog.Any("error", err))
		abort(c, http.StatusInternalServerError, "internal", "internal error")
	}
	return 0, false
}

func pump(c *gin.Context, sub *events.Subscription) {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	lifetime := time.NewTimer(maxStreamLifetime)
	defer lifetime.Stop()
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
			if !write(c, ": ping\n\n") {
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
