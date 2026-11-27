package proxyhttp

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const servicesRequestTimeout = 8 * time.Second

var servicesParams = []string{"signal", "window"}

type servicesResponse struct {
	Datasource    string   `json:"datasource"`
	Signal        string   `json:"signal"`
	Field         string   `json:"field"`
	WindowSeconds int      `json:"window_seconds"`
	Services      []string `json:"services"`
	Truncated     bool     `json:"truncated"`
}

func init() {
	registerRoutes(func(h *Handler, api auth.Routes) {
		api.GET("/datasources/:uid/services", auth.RoleViewer, h.services)
	})
}

func (h *Handler) services(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	req, err := servicesRequest(c.Request.URL.RawQuery)
	if err != nil {
		h.fail(c, err)
		return
	}
	req.Datasource = c.Param("uid")
	ctx, cancel := context.WithTimeout(c.Request.Context(), servicesRequestTimeout)
	defer cancel()
	out, err := h.runner.Services(ctx, req)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, servicesResponse{
			Datasource:    out.Datasource,
			Signal:        out.Signal,
			Field:         out.Field,
			WindowSeconds: int(out.Window / time.Second),
			Services:      out.Services,
			Truncated:     out.Truncated,
		})
	case errors.Is(err, queryproxy.ErrUnknownDatasource):
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
	default:
		h.fail(c, err)
	}
}

func servicesRequest(rawQuery string) (req queryproxy.ServicesRequest, err error) {
	query, err := parseServicesQuery(rawQuery)
	if err != nil {
		return req, err
	}
	req.Signal = query.Get("signal")
	if !slices.Contains([]string{"", datasource.SignalLogs, datasource.SignalTraces, datasource.SignalMetrics}, req.Signal) {
		return req, invalidf("signal must be logs, traces or metrics")
	}
	if text := query.Get("window"); text != "" {
		window, err := time.ParseDuration(text)
		if err != nil {
			return req, invalidf("window must be a duration such as 15m or 1h")
		}
		req.Window = window
	}
	return req, nil
}

func parseServicesQuery(rawQuery string) (url.Values, error) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, invalidf("the query string is malformed")
	}
	for _, name := range slices.Sorted(maps.Keys(query)) {
		switch {
		case !slices.Contains(servicesParams, name):
			return nil, invalidf("unknown query parameter")
		case len(query[name]) != 1:
			return nil, invalidf("%s must be given once", name)
		}
	}
	return query, nil
}
