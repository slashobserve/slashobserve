package annotations

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxBodyBytes = 16 << 10
	defaultLimit = 100
	maxLimit     = 500
)

type Annotations interface {
	Create(ctx context.Context, by auth.Principal, in CreateInput) (Annotation, error)
	Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error
	List(ctx context.Context, by auth.Principal, q ListQuery) (Page, error)
}

type annotationJSON struct {
	ID            string   `json:"id"`
	DashboardID   *string  `json:"dashboard_id"`
	PanelID       *string  `json:"panel_id"`
	TimeFrom      string   `json:"time_from"`
	TimeTo        *string  `json:"time_to"`
	Text          string   `json:"text"`
	Tags          []string `json:"tags"`
	CreatedBy     *string  `json:"created_by"`
	CreatedByName *string  `json:"created_by_name"`
	CreatedAt     string   `json:"created_at"`
}

type listJSON struct {
	Annotations []annotationJSON `json:"annotations"`
	NextCursor  *string          `json:"next_cursor"`
}

type Handler struct {
	annotations Annotations
	logger      *slog.Logger
}

func NewHandler(annotations Annotations, logger *slog.Logger) *Handler {
	return &Handler{annotations: annotations, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/annotations")
	g.GET("", auth.RoleViewer, h.list)
	g.POST("", auth.RoleViewer, h.create)
	g.DELETE("/:id", auth.RoleViewer, h.remove)
}

func annotationJSONOf(a Annotation) annotationJSON {
	out := annotationJSON{
		ID:            a.ID,
		DashboardID:   httpapi.OptionalString(a.DashboardID),
		PanelID:       httpapi.OptionalString(a.PanelID),
		TimeFrom:      httpapi.FormatTime(a.From),
		Text:          a.Text,
		Tags:          a.Tags,
		CreatedBy:     httpapi.OptionalString(a.CreatedBy),
		CreatedByName: httpapi.OptionalString(a.CreatedByName),
		CreatedAt:     httpapi.FormatTime(a.CreatedAt),
	}
	if a.To != nil {
		to := httpapi.FormatTime(*a.To)
		out.TimeTo = &to
	}
	return out
}

func (h *Handler) list(c *gin.Context) {
	q, ok := listQuery(c)
	if !ok {
		return
	}
	page, err := h.annotations.List(c.Request.Context(), httpapi.Caller(c), q)
	if h.refuse(c, err, "dashboard not found") {
		return
	}
	if err != nil {
		h.fail(c, "list annotations", err)
		return
	}
	body := listJSON{Annotations: make([]annotationJSON, 0, len(page.Annotations))}
	for _, a := range page.Annotations {
		body.Annotations = append(body.Annotations, annotationJSONOf(a))
	}
	if page.Next != nil {
		next := httpapi.EncodeTimeCursor(page.Next.Time, page.Next.ID.String())
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func listQuery(c *gin.Context) (ListQuery, bool) {
	var q ListQuery
	var ok bool
	if q.Limit, ok = httpapi.LimitParam(c, defaultLimit, maxLimit); !ok {
		return q, false
	}
	if raw := c.Query("dashboard_id"); raw != "" {
		id, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "dashboard_id must be a UUID")
			return q, false
		}
		q.DashboardID = id
	}
	if q.From, ok = timeParam(c, "from"); !ok {
		return q, false
	}
	if q.To, ok = timeParam(c, "to"); !ok {
		return q, false
	}
	if q.From != nil && q.To != nil && q.To.Before(*q.From) {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "to must not be before from")
		return q, false
	}
	if raw := c.Query("tags"); raw != "" {
		if q.Tags, ok = CleanTags(strings.Split(raw, ",")); !ok {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", MessageInvalidTags)
			return q, false
		}
	}
	if q.After, ok = httpapi.DecodeTimeCursor(c.Query("cursor")); !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
		return q, false
	}
	return q, true
}

func timeParam(c *gin.Context, name string) (*time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be an RFC 3339 time")
		return nil, false
	}
	return &t, true
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	in, err := ParseCreate(req)
	if errors.Is(err, ErrInvalidDashboardID) {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if h.refuse(c, err, "dashboard not found") {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	a, err := h.annotations.Create(ctx, caller, in)
	if h.refuse(c, err, "dashboard not found") {
		return
	}
	if err != nil {
		h.fail(c, "create annotation", err)
		return
	}
	h.logger.InfoContext(ctx, "created an annotation", slog.String("actor_id", caller.ID), slog.String("annotation_id", a.ID), slog.String("dashboard_id", a.DashboardID))
	c.Header("Location", "/api/v1/annotations/"+a.ID)
	c.JSON(http.StatusCreated, annotationJSONOf(a))
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	err := h.annotations.Delete(ctx, caller, id)
	if h.refuse(c, err, "annotation not found") {
		return
	}
	if err != nil {
		h.fail(c, "delete annotation", err)
		return
	}
	h.logger.InfoContext(ctx, "deleted an annotation", slog.String("actor_id", caller.ID), slog.String("annotation_id", id.String()))
	c.Status(http.StatusNoContent)
}

func (h *Handler) refuse(c *gin.Context, err error, notFound string) bool {
	var invalid *InvalidError
	switch {
	case err == nil:
		return false
	case errors.As(err, &invalid):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, invalid.Code, invalid.Message)
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", notFound)
	case errors.Is(err, ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	case errors.Is(err, ErrNotCreator):
		httpapi.AbortError(c, http.StatusForbidden, "not_creator", ErrNotCreator.Error())
	default:
		return false
	}
	return true
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}
