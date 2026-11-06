package findings

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

type FindingAuthorJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FindingJSON struct {
	ID             string            `json:"id"`
	AgentRequestID *string           `json:"agent_request_id"`
	DashboardID    *string           `json:"dashboard_id"`
	Title          string            `json:"title"`
	Markdown       string            `json:"markdown"`
	Evidence       []spec.Panel      `json:"evidence"`
	From           string            `json:"from"`
	To             string            `json:"to"`
	AnnotationID   *string           `json:"annotation_id"`
	CreatedBy      FindingAuthorJSON `json:"created_by"`
	CreatedAt      string            `json:"created_at"`
	URL            string            `json:"url"`
}

type findingEnvelopeJSON struct {
	Finding FindingJSON `json:"finding"`
}

type findingListJSON struct {
	Findings   []FindingJSON `json:"findings"`
	NextCursor *string       `json:"next_cursor"`
}

func JSONOf(f Finding, url string) FindingJSON {
	evidence := f.Evidence
	if evidence == nil {
		evidence = []spec.Panel{}
	}
	return FindingJSON{
		ID:             f.ID,
		AgentRequestID: httpapi.OptionalString(f.AgentRequestID),
		DashboardID:    httpapi.OptionalString(f.DashboardID),
		Title:          f.Title,
		Markdown:       f.Markdown,
		Evidence:       evidence,
		From:           httpapi.FormatTime(f.From),
		To:             httpapi.FormatTime(f.To),
		AnnotationID:   httpapi.OptionalString(f.AnnotationID),
		CreatedBy:      FindingAuthorJSON{Kind: f.CreatedBy.Kind, ID: f.CreatedBy.ID, Name: f.CreatedBy.Name},
		CreatedAt:      httpapi.FormatTime(f.CreatedAt),
		URL:            url,
	}
}

type Handler struct {
	service    *Service
	findingURL func(id string) string
	logger     *slog.Logger
}

func NewHandler(service *Service, findingURL func(id string) string, logger *slog.Logger) *Handler {
	return &Handler{service: service, findingURL: findingURL, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/findings", auth.RoleViewer, h.list)
	api.GET("/findings/:id", auth.RoleViewer, h.get)
	api.POST("/findings/:id/pin", auth.RoleViewer, h.pin)
	api.POST("/findings/:id/dashboard", auth.RoleViewer, h.saveDashboard)
}

func (h *Handler) render(f Finding) FindingJSON {
	return JSONOf(f, h.findingURL(f.ID))
}

func (h *Handler) list(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	var dashboardID pgtype.UUID
	if raw, present := c.GetQuery("dashboard_id"); present {
		id, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "dashboard_id must be a UUID")
			return
		}
		dashboardID = id
	}
	rows, next, err := h.service.List(c.Request.Context(), httpapi.Caller(c), dashboardID, limit, c.Query("cursor"))
	if err != nil {
		h.fail(c, "list findings", err)
		return
	}
	body := findingListJSON{Findings: make([]FindingJSON, len(rows))}
	for i, f := range rows {
		body.Findings[i] = h.render(f)
	}
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	f, err := h.service.Get(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "get finding", err)
		return
	}
	c.JSON(http.StatusOK, findingEnvelopeJSON{Finding: h.render(f)})
}

func (h *Handler) pin(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	a, created, err := h.service.Pin(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "pin finding", err)
		return
	}
	if !created {
		c.JSON(http.StatusOK, annotations.JSONOf(a))
		return
	}
	c.Header("Location", "/api/v1/annotations/"+a.ID)
	c.JSON(http.StatusCreated, annotations.JSONOf(a))
}

func (h *Handler) saveDashboard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	dash, ver, err := h.service.SaveAsDashboard(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "save finding as dashboard", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+dash.ID)
	c.JSON(http.StatusCreated, dashboards.NewDashboardVersionJSON(dash, ver))
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	var invalid *dashboards.InvalidSpecError
	switch {
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", ErrNotFound.Error())
	case errors.Is(err, ErrInvalidCursor):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
	case errors.Is(err, ErrNoDashboard):
		httpapi.AbortError(c, http.StatusConflict, "finding_has_no_dashboard", err.Error())
	case errors.Is(err, ErrNoEvidence):
		httpapi.AbortError(c, http.StatusConflict, "finding_has_no_evidence", err.Error())
	case errors.Is(err, annotations.ErrForbidden), errors.Is(err, folders.ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	case errors.Is(err, folders.ErrNoAccess):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.AccessMessage)
	case errors.As(err, &invalid):
		httpapi.AbortInvalidSpec(c, "spec is invalid", invalid.Issues)
	default:
		principal := httpapi.Caller(c)
		attrs := []any{slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID)}
		if id, parseErr := db.ParseUUID(c.Param("id")); parseErr == nil {
			attrs = append(attrs, slog.String("finding_id", id.String()))
		}
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, attrs...)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
