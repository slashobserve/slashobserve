package public

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/observations"
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
	State        string              `json:"state"`
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
	api.GET("/observations/:id/rcas/:revision/public-links", auth.RoleViewer, h.listRCALinks)
	api.POST("/observations/:id/rcas/:revision/public-links", auth.RoleViewer, h.createRCALink)
	api.DELETE("/observations/:id/public-links/:link_id", auth.RoleViewer, h.removeRCALink)
}

func (h *Handler) RegisterPublic(g *gin.RouterGroup) {
	g.GET("/:token", h.view)
	g.GET("/:token/rca", h.viewRCA)
	g.GET("/:token/rca/snapshots/:snapshot_id", h.viewRCASnapshot)
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
		body.Datasources = append(body.Datasources, datasourceJSON(ds))
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
		State:        linkState(link.Expired),
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

type rcaLinkJSON struct {
	ID           string     `json:"id"`
	Revision     int        `json:"revision"`
	Prefix       string     `json:"prefix"`
	ExpiresAt    *string    `json:"expires_at"`
	CreatedBy    actor.JSON `json:"created_by"`
	CreatedAt    string     `json:"created_at"`
	LastViewedAt *string    `json:"last_viewed_at"`
	Expired      bool       `json:"expired"`
	State        string     `json:"state"`
}

type rcaLinksJSON struct {
	PublicLinks []rcaLinkJSON `json:"public_links"`
}

type createdRCAJSON struct {
	Link  rcaLinkJSON `json:"link"`
	Token string      `json:"token"`
	URL   string      `json:"url"`
}

type createRCARequest struct {
	ExpiresAt *string `json:"expires_at"`
}

type publicRCAJSON struct {
	Revision         int             `json:"revision"`
	Title            string          `json:"title"`
	Summary          *string         `json:"summary"`
	ObservationState string          `json:"observation_state"`
	From             *string         `json:"from"`
	To               *string         `json:"to"`
	Blocks           json.RawMessage `json:"blocks"`
	PublishedBy      string          `json:"published_by"`
	CreatedAt        string          `json:"created_at"`
}

func rcaLinkJSONOf(link RCALink) rcaLinkJSON {
	return rcaLinkJSON{
		ID:           link.ID,
		Revision:     link.Revision,
		Prefix:       link.Prefix,
		ExpiresAt:    optionalTime(link.ExpiresAt),
		CreatedBy:    link.CreatedBy.JSON(),
		CreatedAt:    httpapi.FormatTime(link.CreatedAt),
		LastViewedAt: optionalTime(link.LastViewedAt),
		Expired:      link.Expired,
		State:        linkState(link.Expired),
	}
}

func (h *Handler) listRCALinks(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	revision, ok := observations.RevisionParam(c)
	if !ok {
		return
	}
	found, err := h.links.ListRCALinks(c.Request.Context(), httpapi.Caller(c), id, revision)
	if h.refuseRCA(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "list RCA public links", err)
		return
	}
	body := rcaLinksJSON{PublicLinks: make([]rcaLinkJSON, len(found))}
	for i, link := range found {
		body.PublicLinks[i] = rcaLinkJSONOf(link)
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) createRCALink(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	revision, ok := observations.RevisionParam(c)
	if !ok {
		return
	}
	var req createRCARequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	var in CreateRCAInput
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
	created, err := h.links.CreateRCALink(ctx, caller, id, revision, in)
	if h.refuseRCA(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "create an RCA public link", err)
		return
	}
	h.logger.InfoContext(ctx, "created an RCA public link", slog.String("actor_id", caller.ID), slog.String("observation_id", id.String()), slog.Int("revision", revision), slog.String("public_link_id", created.Link.ID))
	c.Header("Location", "/api/v1/observations/"+id.String()+"/public-links/"+created.Link.ID)
	c.JSON(http.StatusCreated, createdRCAJSON{Link: rcaLinkJSONOf(created.Link), Token: created.Token, URL: created.URL})
}

func (h *Handler) removeRCALink(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	linkID, ok := httpapi.UUIDParam(c, "link_id")
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	err := h.links.DeleteRCALink(ctx, caller, id, linkID)
	if h.refuseRCA(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "delete an RCA public link", err)
		return
	}
	h.logger.InfoContext(ctx, "deleted an RCA public link", slog.String("actor_id", caller.ID), slog.String("observation_id", id.String()), slog.String("public_link_id", linkID.String()))
	c.Status(http.StatusNoContent)
}

func (h *Handler) viewRCA(c *gin.Context) {
	v, err := h.links.ViewRCA(c.Request.Context(), c.Param("token"))
	if errors.Is(err, ErrNotFound) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "public link not found")
		return
	}
	if err != nil {
		h.fail(c, "open an RCA public link", err)
		return
	}
	c.JSON(http.StatusOK, publicRCAJSON{
		Revision:         v.Revision,
		Title:            v.Title,
		Summary:          httpapi.OptionalString(v.Summary),
		ObservationState: v.State,
		From:             optionalTime(v.From),
		To:               optionalTime(v.To),
		Blocks:           v.Blocks,
		PublishedBy:      v.PublishedBy,
		CreatedAt:        httpapi.FormatTime(v.CreatedAt),
	})
}

func (h *Handler) viewRCASnapshot(c *gin.Context) {
	snapshotID, err := db.ParseUUID(c.Param("snapshot_id"))
	if err != nil {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "public link not found")
		return
	}
	frames, err := h.links.ViewRCASnapshot(c.Request.Context(), c.Param("token"), snapshotID)
	if errors.Is(err, ErrNotFound) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "public link not found")
		return
	}
	if err != nil {
		h.fail(c, "open an RCA snapshot", err)
		return
	}
	c.Data(http.StatusOK, gin.MIMEJSON, frames)
}

func (h *Handler) refuseRCA(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, observations.ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", observations.ErrNotFound.Error())
	case errors.Is(err, observations.ErrRevisionNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", observations.ErrRevisionNotFound.Error())
	case errors.Is(err, ErrRCAPersonalFolder):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, err.Error())
	case errors.Is(err, ErrRCAUserScoped):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "user_scoped", err.Error())
	case errors.Is(err, ErrTooManyRCALinks):
		httpapi.AbortError(c, http.StatusConflict, "too_many_links", err.Error())
	default:
		return h.refuse(c, err)
	}
	return true
}
