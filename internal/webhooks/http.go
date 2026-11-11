package webhooks

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxBodyBytes     = 16 << 10
	defaultPageSize  = 50
	maxDeliveryLimit = 100
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/webhooks")
	g.GET("", auth.RoleAdmin, h.list)
	g.POST("", auth.RoleAdmin, h.create)
	g.PATCH("/:id", auth.RoleAdmin, h.update)
	g.DELETE("/:id", auth.RoleAdmin, h.remove)
	g.POST("/:id/secret", auth.RoleAdmin, h.rotate)
	g.POST("/:id/test", auth.RoleAdmin, h.test)
	g.GET("/:id/deliveries", auth.RoleAdmin, h.deliveries)
}

type webhookJSON struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	EventTypes          []string   `json:"event_types"`
	Owner               actor.JSON `json:"owner"`
	Enabled             bool       `json:"enabled"`
	State               string     `json:"state"`
	DisabledReason      *string    `json:"disabled_reason"`
	DisabledAt          *string    `json:"disabled_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	NextAttemptAt       string     `json:"next_attempt_at"`
	CreatedAt           string     `json:"created_at"`
	UpdatedAt           string     `json:"updated_at"`
}

type createdJSON struct {
	Webhook webhookJSON `json:"webhook"`
	Secret  string      `json:"secret"`
}

type deliveryJSON struct {
	ID          string  `json:"id"`
	DeliveryID  string  `json:"delivery_id"`
	EventType   string  `json:"event_type"`
	ActivitySeq *int64  `json:"activity_seq"`
	AttemptedAt string  `json:"attempted_at"`
	DurationMs  int     `json:"duration_ms"`
	StatusCode  *int    `json:"status_code"`
	Outcome     string  `json:"outcome"`
	ErrorCode   *string `json:"error_code"`
}

func newWebhookJSON(w Webhook) webhookJSON {
	out := webhookJSON{
		ID: w.ID, Name: w.Name, URL: w.URL, EventTypes: w.EventTypes, Owner: w.Owner.JSON(), Enabled: w.Enabled, State: stateOf(w.Enabled, w.ConsecutiveFailures),
		DisabledReason: httpapi.OptionalString(w.DisabledReason), ConsecutiveFailures: w.ConsecutiveFailures,
		NextAttemptAt: httpapi.FormatTime(w.NextAttemptAt), CreatedAt: httpapi.FormatTime(w.CreatedAt), UpdatedAt: httpapi.FormatTime(w.UpdatedAt),
	}
	if w.DisabledAt != nil {
		at := httpapi.FormatTime(*w.DisabledAt)
		out.DisabledAt = &at
	}
	return out
}

func newDeliveryJSON(d Delivery) deliveryJSON {
	return deliveryJSON{
		ID: strconv.FormatInt(d.ID, 10), DeliveryID: d.DeliveryID, EventType: d.EventType, ActivitySeq: d.ActivitySeq,
		AttemptedAt: httpapi.FormatTime(d.AttemptedAt), DurationMs: d.DurationMs, StatusCode: d.StatusCode,
		Outcome: d.Outcome, ErrorCode: httpapi.OptionalString(d.ErrorCode),
	}
}

func (h *Handler) list(c *gin.Context) {
	list, err := h.svc.List(c.Request.Context())
	if err != nil {
		h.fail(c, "list webhooks", err)
		return
	}
	out := make([]webhookJSON, 0, len(list))
	for _, w := range list {
		out = append(out, newWebhookJSON(w))
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": out})
}

func (h *Handler) create(c *gin.Context) {
	var body struct {
		Name       *string   `json:"name"`
		URL        *string   `json:"url"`
		EventTypes *[]string `json:"event_types"`
	}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	if body.Name == nil || body.URL == nil || body.EventTypes == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "name, url and event_types are required")
		return
	}
	created, err := h.svc.Create(c.Request.Context(), httpapi.Caller(c), Input{Name: *body.Name, URL: *body.URL, EventTypes: *body.EventTypes})
	if err != nil {
		h.fail(c, "create webhook", err)
		return
	}
	c.JSON(http.StatusCreated, createdJSON{Webhook: newWebhookJSON(created.Webhook), Secret: created.Secret})
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var body struct {
		Name       *string   `json:"name"`
		URL        *string   `json:"url"`
		EventTypes *[]string `json:"event_types"`
		Enabled    *bool     `json:"enabled"`
	}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	updated, err := h.svc.Update(c.Request.Context(), httpapi.Caller(c), id, Patch{Name: body.Name, URL: body.URL, EventTypes: body.EventTypes, Enabled: body.Enabled})
	if err != nil {
		h.fail(c, "update webhook", err)
		return
	}
	c.JSON(http.StatusOK, newWebhookJSON(updated))
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		h.fail(c, "delete webhook", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) rotate(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	rotated, err := h.svc.RotateSecret(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "rotate webhook secret", err)
		return
	}
	c.JSON(http.StatusOK, createdJSON{Webhook: newWebhookJSON(rotated.Webhook), Secret: rotated.Secret})
}

func (h *Handler) test(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	tested, err := h.svc.Test(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "test webhook", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"delivery": newDeliveryJSON(tested)})
}

func (h *Handler) deliveries(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	limit, ok := httpapi.LimitParam(c, defaultPageSize, maxDeliveryLimit)
	if !ok {
		return
	}
	page, next, err := h.svc.Deliveries(c.Request.Context(), id, c.Query("cursor"), limit)
	if err != nil {
		h.fail(c, "list webhook deliveries", err)
		return
	}
	out := make([]deliveryJSON, 0, len(page))
	for _, d := range page {
		out = append(out, newDeliveryJSON(d))
	}
	body := gin.H{"deliveries": out, "next_cursor": nil}
	if next != "" {
		body["next_cursor"] = next
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	var invalid *InvalidError
	var hostErr *egress.HostError
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	switch {
	case errors.As(err, &invalid):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", invalid.Message)
	case errors.Is(err, ErrInvalidCursor):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrNoSecretKey):
		httpapi.AbortError(c, http.StatusConflict, "secret_key_missing", err.Error())
	case errors.Is(err, ErrSecretUnreadable):
		httpapi.AbortError(c, http.StatusConflict, "secret_unreadable", err.Error())
	case errors.Is(err, ErrTooMany):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "too_many_webhooks", "at most 20 webhooks can be set up")
	case errors.Is(err, egress.ErrNotAllowed) && errors.As(err, &hostErr):
		httpapi.AbortError(c, http.StatusForbidden, "outbound_domain_not_allowed", hostErr.Host+" is not on the outbound allowlist.")
	default:
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
