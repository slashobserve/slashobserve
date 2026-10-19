package proxyhttp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const namesTimeout = 15 * time.Second

var (
	metricsParams = []string{"search", "limit"}
	labelsParams  = []string{"metric", "table", "search", "limit"}
)

type metricsResponse struct {
	Datasource string              `json:"datasource"`
	Metrics    []datasource.Metric `json:"metrics"`
	Truncated  bool                `json:"truncated"`
}

type labelsResponse struct {
	Datasource string   `json:"datasource"`
	Labels     []string `json:"labels"`
	Truncated  bool     `json:"truncated"`
}

type labelValuesResponse struct {
	Datasource string   `json:"datasource"`
	Label      string   `json:"label"`
	Values     []string `json:"values"`
	Truncated  bool     `json:"truncated"`
}

func (h *Handler) metrics(c *gin.Context) {
	names, ok := h.names(c, datasource.DiscoverMetrics, metricsParams, "")
	if !ok {
		return
	}
	c.JSON(http.StatusOK, metricsResponse{Datasource: names.Datasource, Metrics: names.Metrics, Truncated: names.Truncated})
}

func (h *Handler) labels(c *gin.Context) {
	names, ok := h.names(c, datasource.DiscoverLabels, labelsParams, "")
	if !ok {
		return
	}
	c.JSON(http.StatusOK, labelsResponse{Datasource: names.Datasource, Labels: names.Labels, Truncated: names.Truncated})
}

func (h *Handler) labelValues(c *gin.Context) {
	names, ok := h.names(c, datasource.DiscoverLabelValues, labelsParams, c.Param("name"))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, labelValuesResponse{Datasource: names.Datasource, Label: names.Label, Values: names.Values, Truncated: names.Truncated})
}

func (h *Handler) names(c *gin.Context, kind datasource.DiscoverKind, allowed []string, label string) (queryproxy.Names, bool) {
	release, ok := h.acquire(c)
	if !ok {
		return queryproxy.Names{}, false
	}
	defer release()
	uid := c.Param("uid")
	if _, found := h.registry.Get(uid); !found {
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
		return queryproxy.Names{}, false
	}
	req, message := namesRequest(c.Request.URL.RawQuery, allowed)
	if message != "" {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message)
		return queryproxy.Names{}, false
	}
	req.Datasource, req.Kind, req.Label = uid, kind, label
	ctx, cancel := context.WithTimeout(c.Request.Context(), namesTimeout)
	defer cancel()
	names, err := h.runner.Names(ctx, req)
	switch {
	case err == nil:
		return names, true
	case errors.Is(err, queryproxy.ErrUnknownDatasource):
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
	case errors.Is(err, queryproxy.ErrUpstreamBusy):
		tooManyRequests(c)
	default:
		h.fail(c, err)
	}
	return queryproxy.Names{}, false
}

func namesRequest(rawQuery string, allowed []string) (req queryproxy.NamesRequest, message string) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return req, "the query string is malformed"
	}
	for _, name := range slices.Sorted(maps.Keys(query)) {
		values := query[name]
		if !slices.Contains(allowed, name) {
			return req, "unknown query parameter"
		}
		if len(values) != 1 {
			return req, fmt.Sprintf("%s must be given once", name)
		}
		switch name {
		case "search":
			req.Search = values[0]
		case "metric":
			req.Metric = values[0]
		case "table":
			req.Table = values[0]
		case "limit":
			n, err := strconv.Atoi(values[0])
			if err != nil || strconv.Itoa(n) != values[0] || n < 1 || n > queryproxy.MaxNamesLimit {
				return req, fmt.Sprintf("limit must be between 1 and %d", queryproxy.MaxNamesLimit)
			}
			req.Limit = n
		}
	}
	return req, ""
}
