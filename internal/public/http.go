package public

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxBodyBytes = 16 << 10

type Handler struct {
	links  *Service
	access *folders.Access
	logger *slog.Logger
}

func NewHandler(links *Service, access *folders.Access, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{links: links, access: access, logger: logger}
}

type creatorJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type linkJSON struct {
	ID           string              `json:"id"`
	Prefix       string              `json:"prefix"`
	Variables    Mode                `json:"variables"`
	Selections   map[string][]string `json:"selections"`
	ExpiresAt    *string             `json:"expires_at"`
	CreatedBy    creatorJSON         `json:"created_by"`
	CreatedAt    string              `json:"created_at"`
	LastViewedAt *string             `json:"last_viewed_at"`
	Expired      bool                `json:"expired"`
	Usable       bool                `json:"usable"`
}

type listJSON struct {
	PublicLinks []linkJSON `json:"public_links"`
}

type createdJSON struct {
	Link  linkJSON `json:"link"`
	Token string   `json:"token"`
	URL   string   `json:"url"`
}

type createRequest struct {
	Variables  Mode                `json:"variables"`
	Selections map[string][]string `json:"selections"`
	ExpiresAt  *string             `json:"expires_at"`
}

type datasourceJSON struct {
	UID  string `json:"uid"`
	Type string `json:"type"`
}

type viewJSON struct {
	DashboardID string              `json:"dashboard_id"`
	Title       string              `json:"title"`
	Version     int                 `json:"version"`
	Spec        *spec.Dashboard     `json:"spec"`
	Variables   Mode                `json:"variables"`
	Selections  map[string][]string `json:"selections"`
	ExpiresAt   *string             `json:"expires_at"`
	Datasources []datasourceJSON    `json:"datasources"`
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/dashboards/:id/public-links")
	g.GET("", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.list)
	g.POST("", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.create)
	g.DELETE("/:link_id", auth.RoleViewer, h.access.RequireDashboard(folders.NeedView), h.remove)
}

func (h *Handler) RegisterPublic(g *gin.RouterGroup) {
	g.GET("/:token", h.view)
}

func (h *Handler) list(c *gin.Context) {
	id, _ := httpapi.UUIDParam(c, "id")
	found, err := h.links.List(c.Request.Context(), httpapi.Caller(c), id)
	if h.refuse(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "list public links", err)
		return
	}
	body := listJSON{PublicLinks: make([]linkJSON, 0, len(found))}
	for _, link := range found {
		body.PublicLinks = append(body.PublicLinks, linkJSONOf(link))
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) create(c *gin.Context) {
	id, _ := httpapi.UUIDParam(c, "id")
	var req createRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	in := CreateInput{Mode: req.Variables, Selections: req.Selections}
	if req.ExpiresAt != nil {
		at, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "expires_at must be an RFC 3339 time")
			return
		}
		in.ExpiresAt = &at
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	created, err := h.links.Create(ctx, caller, id, in)
	if h.refuse(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "create a public link", err)
		return
	}
	h.logger.InfoContext(ctx, "created a public link", slog.String("actor_id", caller.ID), slog.String("dashboard_id", created.Link.DashboardID), slog.String("public_link_id", created.Link.ID))
	c.Header("Location", "/api/v1/dashboards/"+created.Link.DashboardID+"/public-links/"+created.Link.ID)
	c.JSON(http.StatusCreated, createdJSON{Link: linkJSONOf(created.Link), Token: created.Token, URL: created.URL})
}

func (h *Handler) remove(c *gin.Context) {
	id, _ := httpapi.UUIDParam(c, "id")
	linkID, ok := httpapi.UUIDParam(c, "link_id")
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	err := h.links.Delete(ctx, caller, id, linkID)
	if h.refuse(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "delete a public link", err)
		return
	}
	h.logger.InfoContext(ctx, "deleted a public link", slog.String("actor_id", caller.ID), slog.String("dashboard_id", id.String()), slog.String("public_link_id", linkID.String()))
	c.Status(http.StatusNoContent)
}

func (h *Handler) view(c *gin.Context) {
	v, err := h.links.View(c.Request.Context(), c.Param("token"))
	if errors.Is(err, ErrNotFound) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "public link not found")
		return
	}
	if err != nil {
		h.fail(c, "open a public link", err)
		return
	}
	body := viewJSON{
		DashboardID: v.DashboardID.String(),
		Title:       v.Title,
		Version:     v.Version,
		Spec:        v.Spec,
		Variables:   v.Mode,
		Selections:  v.Selections,
		ExpiresAt:   optionalTime(v.ExpiresAt),
		Datasources: make([]datasourceJSON, 0, len(v.Datasources)),
	}
	for _, ds := range v.Datasources {
		body.Datasources = append(body.Datasources, datasourceJSON{UID: ds.UID, Type: ds.Type})
	}
	c.JSON(http.StatusOK, body)
}

func linkJSONOf(link Link) linkJSON {
	selections := link.Selections
	if selections == nil {
		selections = map[string][]string{}
	}
	return linkJSON{
		ID:           link.ID,
		Prefix:       link.Prefix,
		Variables:    link.Mode,
		Selections:   selections,
		ExpiresAt:    optionalTime(link.ExpiresAt),
		CreatedBy:    creatorJSON(link.CreatedBy),
		CreatedAt:    httpapi.FormatTime(link.CreatedAt),
		LastViewedAt: optionalTime(link.LastViewedAt),
		Expired:      link.Expired,
		Usable:       link.Usable,
	}
}

func optionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := httpapi.FormatTime(*t)
	return &s
}

func (h *Handler) refuse(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrInvalidMode):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, dashboards.ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "dashboard not found")
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "public link not found")
	case errors.Is(err, folders.ErrNoAccess):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.AccessMessage)
	case errors.Is(err, folders.ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	case errors.Is(err, ErrPersonalFolder):
		httpapi.AbortError(c, http.StatusConflict, "personal_folder", err.Error())
	case errors.Is(err, ErrUserScoped):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "user_scoped", err.Error())
	case errors.Is(err, ErrInvalidExpiry):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_expiry", err.Error())
	case errors.Is(err, ErrUnknownVariable):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "unknown_variable", err.Error())
	case errors.Is(err, ErrTooManyLinks):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "too_many_links", err.Error())
	default:
		return false
	}
	return true
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err)...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}
