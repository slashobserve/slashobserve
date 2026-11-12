package proxyhttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const observationSessionRequiredMessage = "the data of an observation panel is for signed-in users; MCP clients and scripts use query_range"

type ObservationPanels interface {
	PanelDashboard(ctx context.Context, by auth.Principal, blockID pgtype.UUID) (*spec.Dashboard, time.Time, time.Time, error)
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
	dashboard, _, _, err := d.panels.PanelDashboard(ctx, principal, id)
	switch {
	case err == nil:
		return loadedVersion{dashboard: dashboard}, true
	case errors.Is(err, observations.ErrNotFound):
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "block not found")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		d.h.fail(c, err)
	default:
		d.h.logger.ErrorContext(ctx, "load observation panel", failureAttrs(err)...)
		d.h.fail(c, errInternal)
	}
	return loadedVersion{}, false
}
