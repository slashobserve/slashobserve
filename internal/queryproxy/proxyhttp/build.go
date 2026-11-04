package proxyhttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const userSessionRequiredMessage = "the data of a build is for signed-in users; MCP clients and scripts use query_range"

func (h *Handler) buildData(c *gin.Context) {
	query, ok := h.readQuery(c)
	if !ok {
		return
	}
	release, ok := h.acquireBuild(c)
	if !ok {
		return
	}
	defer release()
	p, err := parseParams(query, true)
	if err != nil {
		h.fail(c, err)
		return
	}
	v, ok := h.loadBuild(c)
	if !ok {
		return
	}
	h.serveData(c, p, v, c.Param("panel_id"), nil)
}

func (h *Handler) buildValues(c *gin.Context) {
	query, ok := h.readQuery(c)
	if !ok {
		return
	}
	release, ok := h.acquireBuild(c)
	if !ok {
		return
	}
	defer release()
	p, err := parseParams(query, false)
	if err != nil {
		h.fail(c, err)
		return
	}
	v, ok := h.loadBuild(c)
	if !ok {
		return
	}
	h.serveValues(c, p, v, c.Param("name"), nil)
}

func (h *Handler) acquireBuild(c *gin.Context) (release func(), ok bool) {
	principal, _ := auth.FromContext(c.Request.Context())
	if principal.Kind != auth.KindUser {
		abortError(c, http.StatusForbidden, "user_session_required", userSessionRequiredMessage)
		return nil, false
	}
	return h.acquire(c)
}

func (h *Handler) loadBuild(c *gin.Context) (loadedVersion, bool) {
	ctx := c.Request.Context()
	id, ok := parseID(c.Param("id"))
	if !ok {
		h.fail(c, invalidf("id must be a UUID"))
		return loadedVersion{}, false
	}
	principal, _ := auth.FromContext(ctx)
	build, err := h.builds.ViewBuild(ctx, principal, id)
	switch {
	case err == nil:
		return loadedVersion{dashboard: build.Spec}, true
	case errors.Is(err, dashboards.ErrBuildNotFound):
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "build not found")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		h.fail(c, err)
	default:
		h.logger.ErrorContext(ctx, "load build", failureAttrs(err)...)
		h.fail(c, errInternal)
	}
	return loadedVersion{}, false
}
