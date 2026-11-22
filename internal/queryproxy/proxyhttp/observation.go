package proxyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	observationSessionRequiredMessage  = "the data of an observation panel is for signed-in users; MCP clients and scripts use query_range"
	blockPreviewSessionRequiredMessage = "the draft preview of an observation panel is for signed-in users; MCP clients and scripts use query_range"
)

type ObservationPanels interface {
	PanelData(ctx context.Context, by auth.Principal, blockID pgtype.UUID) (*spec.Dashboard, folders.Folder, error)
	DraftPanelData(ctx context.Context, by auth.Principal, observationID, blockID pgtype.UUID, panel json.RawMessage) (*spec.Dashboard, folders.Folder, error)
}

type blockPreviewRequest struct {
	BlockID *string         `json:"block_id"`
	Panel   json.RawMessage `json:"panel"`
	draftWindow
}

type ObservationData struct {
	h      *Handler
	panels ObservationPanels
}

func NewObservationData(h *Handler, panels ObservationPanels) *ObservationData {
	return &ObservationData{h: h, panels: panels}
}

func (d *ObservationData) Register(api auth.Routes) {
	api.GET("/blocks/:id/data", auth.RoleViewer, d.data)
	api.POST("/observations/:id/blocks/preview", auth.RoleViewer, d.preview)
}

func (d *ObservationData) preview(c *gin.Context) {
	principal, _ := auth.FromContext(c.Request.Context())
	if principal.Kind != auth.KindUser {
		abortError(c, http.StatusForbidden, "user_session_required", blockPreviewSessionRequiredMessage)
		return
	}
	release, ok := d.h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req blockPreviewRequest
	if !httpapi.DecodeBody(c, &req, maxPreviewBodyBytes) {
		return
	}
	observationID, blockID, message := req.validate(c.Param("id"))
	if message != "" {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message)
		return
	}
	p, err := parseParams(req.values(), true)
	if err != nil {
		d.h.fail(c, err)
		return
	}
	dashboard, folder, err := d.panels.DraftPanelData(c.Request.Context(), principal, observationID, blockID, req.Panel)
	if !d.loaded(c, err, "observation not found") {
		return
	}
	readingStored(c, folder)
	d.h.serveData(c, p, loadedVersion{dashboard: dashboard}, observations.BlockPanelID, nil)
}

func (r blockPreviewRequest) validate(observation string) (observationID, blockID pgtype.UUID, message string) {
	observationID, ok := parseID(observation)
	if !ok {
		return observationID, blockID, "id must be a UUID"
	}
	if r.BlockID != nil {
		if blockID, ok = parseID(*r.BlockID); !ok {
			return observationID, blockID, "block_id must be a UUID"
		}
	}
	if !isJSONObject(r.Panel) {
		return observationID, blockID, "panel must be a JSON object"
	}
	return observationID, blockID, r.draftWindow.validate()
}

func (d *ObservationData) data(c *gin.Context) {
	query, ok := d.h.readQuery(c)
	if !ok {
		return
	}
	principal, _ := auth.FromContext(c.Request.Context())
	if principal.Kind != auth.KindUser {
		abortError(c, http.StatusForbidden, "user_session_required", observationSessionRequiredMessage)
		return
	}
	release, ok := d.h.acquire(c)
	if !ok {
		return
	}
	defer release()
	p, err := parseParams(query, true)
	if err != nil {
		d.h.fail(c, err)
		return
	}
	v, ok := d.load(c, principal)
	if !ok {
		return
	}
	d.h.serveData(c, p, v, observations.BlockPanelID, nil)
}

func (d *ObservationData) load(c *gin.Context, principal auth.Principal) (loadedVersion, bool) {
	ctx := c.Request.Context()
	id, ok := parseID(c.Param("id"))
	if !ok {
		d.h.fail(c, invalidf("id must be a UUID"))
		return loadedVersion{}, false
	}
	dashboard, folder, err := d.panels.PanelData(ctx, principal, id)
	if !d.loaded(c, err, "block not found") {
		return loadedVersion{}, false
	}
	readingStored(c, folder)
	return loadedVersion{dashboard: dashboard}, true
}

func (d *ObservationData) loaded(c *gin.Context, err error, notFound string) bool {
	var invalid *observations.InvalidInputError
	switch {
	case err == nil:
		return true
	case errors.Is(err, observations.ErrNotFound):
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), notFound)
	case errors.Is(err, folders.ErrForbidden):
		abortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	case errors.As(err, &invalid):
		issues := invalid.Issues
		if issues == nil {
			issues = []spec.Issue{}
		}
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, httpapi.InvalidSpecBody{Error: invalid.Message, Code: invalid.Code, Issues: issues})
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		d.h.fail(c, err)
	default:
		d.h.logger.ErrorContext(c.Request.Context(), "load observation panel", failureAttrs(err)...)
		d.h.fail(c, errInternal)
	}
	return false
}
