package proxyhttp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/public"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type PublicLinks interface {
	Resolve(ctx context.Context, token string) (public.Resolved, error)
}

type PublicData struct {
	h     *Handler
	links PublicLinks
}

func NewPublicData(h *Handler, links PublicLinks) *PublicData {
	return &PublicData{h: h, links: links}
}

func (d *PublicData) RegisterPublic(g *gin.RouterGroup) {
	g.GET("/:token/panels/:panel_id/data", d.data)
	g.GET("/:token/variables/:name/values", d.values)
}

func publicSlot(link public.Resolved) string {
	return "public:" + link.LinkID.String()
}

func (d *PublicData) data(c *gin.Context) {
	link, v, p, release, ok := d.open(c, true)
	if !ok {
		return
	}
	defer release()
	selections, ok := d.selections(c, link, v, p, len(v.dashboard.Variables))
	if !ok {
		return
	}
	p.selections = selections
	d.h.serveData(c, p, v, c.Param("panel_id"), &cachedVersion{id: link.DashboardID, version: link.Version})
}

func (d *PublicData) values(c *gin.Context) {
	link, v, p, release, ok := d.open(c, false)
	if !ok {
		return
	}
	defer release()
	name := c.Param("name")
	upto := max(0, slices.IndexFunc(v.dashboard.Variables, func(v spec.Variable) bool { return v.Name == name }))
	selections, ok := d.selections(c, link, v, p, upto)
	if !ok {
		return
	}
	p.selections = selections
	d.h.serveValues(c, p, v, name, &cachedVersion{id: link.DashboardID, version: link.Version})
}

func (d *PublicData) open(c *gin.Context, allowMaxDataPoints bool) (link public.Resolved, v loadedVersion, p params, release func(), ok bool) {
	ctx := c.Request.Context()
	query, ok := d.h.readQuery(c)
	if !ok {
		return link, v, p, nil, false
	}
	link, err := d.links.Resolve(ctx, c.Param("token"))
	switch {
	case errors.Is(err, public.ErrNotFound):
		linkNotFound(c)
		return link, v, p, nil, false
	case err != nil:
		d.h.logger.ErrorContext(ctx, "resolve a public link", failureAttrs(err)...)
		d.h.fail(c, errInternal)
		return link, v, p, nil, false
	}
	release, ok = d.h.acquireKey(c, publicSlot(link))
	if !ok {
		return link, v, p, nil, false
	}
	if v, p, ok = d.load(c, query, link, allowMaxDataPoints); !ok {
		release()
		return link, v, p, nil, false
	}
	return link, v, p, release, true
}

func (d *PublicData) load(c *gin.Context, query url.Values, link public.Resolved, allowMaxDataPoints bool) (loadedVersion, params, bool) {
	p, err := parseParams(query, allowMaxDataPoints)
	if err != nil {
		d.h.fail(c, err)
		return loadedVersion{}, params{}, false
	}
	v, err := d.h.dashboard(c.Request.Context(), link.DashboardID, link.Version)
	switch {
	case errors.Is(err, dashboards.ErrNotFound), err == nil && v.usesUser:
		linkNotFound(c)
		return loadedVersion{}, params{}, false
	case err != nil:
		d.h.fail(c, err)
		return loadedVersion{}, params{}, false
	}
	return v, p, true
}

func (d *PublicData) selections(c *gin.Context, link public.Resolved, v loadedVersion, p params, upto int) (map[string]queryproxy.Selection, bool) {
	ctx := c.Request.Context()
	rng, _, err := p.timeRange(v.dashboard, time.Now())
	if err != nil {
		d.h.fail(c, err)
		return nil, false
	}
	cached := &cachedVersion{id: link.DashboardID, version: link.Version}
	options := func(name string, earlier map[string][]string) ([]string, error) {
		entry, _, err := d.h.variableOptions(ctx, v, cached, name, valuesRange(rng), selectionsOf(earlier))
		return entry.values, err
	}
	effective, err := public.EffectiveSelections(link.Mode, v.dashboard, link.Selections, rawSelections(p.selections), options)
	if errors.Is(err, public.ErrSelectionNotAllowed) {
		abortError(c, http.StatusBadRequest, "selection_not_allowed", "a variable value is not one this public link allows")
		return nil, false
	}
	if err != nil {
		d.h.fail(c, err)
		return nil, false
	}
	allowed := map[string]bool{}
	for _, variable := range v.dashboard.Variables[:upto] {
		allowed[variable.Name] = true
	}
	out := map[string]queryproxy.Selection{}
	for name, values := range effective {
		if allowed[name] {
			out[name] = selectionOf(values)
		}
	}
	return out, true
}

func rawSelections(selections map[string]queryproxy.Selection) map[string][]string {
	out := make(map[string][]string, len(selections))
	for name, sel := range selections {
		values := slices.Clone(sel.Values)
		if sel.All {
			values = append([]string{allSelection}, values...)
		}
		if values == nil {
			values = []string{}
		}
		out[name] = values
	}
	return out
}

func selectionsOf(raw map[string][]string) map[string]queryproxy.Selection {
	out := make(map[string]queryproxy.Selection, len(raw))
	for name, values := range raw {
		out[name] = selectionOf(values)
	}
	return out
}

func linkNotFound(c *gin.Context) {
	abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "public link not found")
}
