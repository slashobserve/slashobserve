package discoveryhttp

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type Reader interface {
	DatasourceSchema(ctx context.Context, uid, table string, offset *int) (any, error)
	DatasourceSample(ctx context.Context, uid, table string, limit *int) (any, error)
}

type Handler struct {
	reader Reader
	logger *slog.Logger
}

func NewHandler(r Reader, logger *slog.Logger) *Handler {
	return &Handler{reader: r, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/datasources/:uid/schema", auth.RoleViewer, h.schema)
	api.GET("/datasources/:uid/sample", auth.RoleViewer, h.sample)
}

type refusal struct {
	status int
	code   string
}

var refusals = map[string]refusal{
	"invalid_request":    {http.StatusBadRequest, "invalid_request"},
	"unknown_datasource": {http.StatusNotFound, "not_found"},
	"not_found":          {http.StatusNotFound, "not_found"},
	"query_invalid":      {http.StatusUnprocessableEntity, "query_invalid"},
	"query_forbidden":    {http.StatusUnprocessableEntity, "query_forbidden"},
	"limit_exceeded":     {http.StatusUnprocessableEntity, "limit_exceeded"},
	"too_many_requests":  {http.StatusTooManyRequests, "too_many_requests"},
	"datasource_down":    {http.StatusBadGateway, "datasource_down"},
	"query_timeout":      {http.StatusGatewayTimeout, "query_timeout"},
}

func (h *Handler) schema(c *gin.Context) {
	query, ok := readQuery(c, "table", "offset")
	if !ok {
		return
	}
	offset, ok := optionalInt(c, query, "offset")
	if !ok {
		return
	}
	out, err := h.reader.DatasourceSchema(c.Request.Context(), c.Param("uid"), query.Get("table"), offset)
	h.answer(c, "datasource schema", out, err)
}

func (h *Handler) sample(c *gin.Context) {
	query, ok := readQuery(c, "table", "limit")
	if !ok {
		return
	}
	limit, ok := optionalInt(c, query, "limit")
	if !ok {
		return
	}
	out, err := h.reader.DatasourceSample(c.Request.Context(), c.Param("uid"), query.Get("table"), limit)
	h.answer(c, "datasource sample", out, err)
}

func (h *Handler) answer(c *gin.Context, what string, out any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, out)
		return
	}
	if clientgone.Handle(c, h.logger, what, err) {
		return
	}
	var refused *tools.ReadError
	if !errors.As(err, &refused) {
		h.logger.ErrorContext(c.Request.Context(), what+" failed", logattrs.Failure("internal", err)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	r, known := refusals[refused.Code]
	if !known {
		r = refusal{http.StatusBadGateway, refused.Code}
	}
	if r.status == http.StatusTooManyRequests {
		c.Header("Retry-After", "1")
	}
	httpapi.AbortError(c, r.status, r.code, refused.Message)
}

func readQuery(c *gin.Context, allowed ...string) (url.Values, bool) {
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "the query string is malformed")
		return nil, false
	}
	for _, name := range slices.Sorted(maps.Keys(query)) {
		if !slices.Contains(allowed, name) {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "unknown query parameter "+strconv.Quote(name)+"; this route takes "+strconv.Quote(allowed[0])+" and "+strconv.Quote(allowed[1]))
			return nil, false
		}
		if len(query[name]) != 1 {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be given once")
			return nil, false
		}
	}
	return query, true
}

func optionalInt(c *gin.Context, query url.Values, name string) (*int, bool) {
	if !query.Has(name) {
		return nil, true
	}
	n, err := strconv.Atoi(query.Get(name))
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be a whole number")
		return nil, false
	}
	return &n, true
}
