package proxyhttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const findingSessionRequiredMessage = "the evidence data of a finding is for signed-in users; MCP clients and scripts use query_range"

type FindingSpecs interface {
	EvidenceDashboard(ctx context.Context, by auth.Principal, id pgtype.UUID) (*spec.Dashboard, error)
}

type FindingData struct {
	h        *Handler
	findings FindingSpecs
}

func NewFindingData(h *Handler, f FindingSpecs) *FindingData {
	return &FindingData{h: h, findings: f}
}

func (d *FindingData) Register(api auth.Routes) {
	api.GET("/findings/:id/panels/:panel_id/data", auth.RoleViewer, d.data)
}

func (d *FindingData) data(c *gin.Context) {
	query, ok := d.h.readQuery(c)
	if !ok {
		return
	}
	principal, _ := auth.FromContext(c.Request.Context())
	if principal.Kind != auth.KindUser {
		abortError(c, http.StatusForbidden, "user_session_required", findingSessionRequiredMessage)
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
	d.h.serveData(c, p, v, c.Param("panel_id"), nil)
}

func (d *FindingData) load(c *gin.Context, principal auth.Principal) (loadedVersion, bool) {
	ctx := c.Request.Context()
	id, ok := parseID(c.Param("id"))
	if !ok {
		d.h.fail(c, invalidf("id must be a UUID"))
		return loadedVersion{}, false
	}
	dashboard, err := d.findings.EvidenceDashboard(ctx, principal, id)
	switch {
	case err == nil:
		return loadedVersion{dashboard: dashboard}, true
	case errors.Is(err, findings.ErrNotFound):
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "finding not found")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		d.h.fail(c, err)
	default:
		d.h.logger.ErrorContext(ctx, "load finding evidence", failureAttrs(err)...)
		d.h.fail(c, errInternal)
	}
	return loadedVersion{}, false
}
