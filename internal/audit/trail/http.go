package trail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	RowLimitHeader  = "X-Export-Row-Limit"
	TruncatedHeader = "X-Export-Truncated"
)

type Handler struct {
	reader     *Reader
	principals Principals
	logger     *slog.Logger
}

func NewHandler(reader *Reader, principals Principals, logger *slog.Logger) *Handler {
	return &Handler{reader: reader, principals: principals, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/audit")
	g.GET("/events", auth.RoleAdmin, h.search)
	g.GET("/events/:id", auth.RoleAdmin, h.event)
	g.GET("/facets", auth.RoleAdmin, h.facets)
	g.GET("/histogram", auth.RoleAdmin, h.histogram)
	g.GET("/aggregate", auth.RoleAdmin, h.aggregate)
	g.GET("/fields", auth.RoleAdmin, h.fields)
	g.GET("/export", auth.RoleAdmin, h.export)
}

type windowJSON struct {
	Form     string   `json:"form"`
	Since    string   `json:"since"`
	Until    string   `json:"until"`
	Warnings []string `json:"warnings"`
}

func windowOf(p Prepared) windowJSON {
	return windowJSON{Form: string(p.Query.Form), Since: httpapi.FormatTime(p.Window.From), Until: httpapi.FormatTime(p.Window.To), Warnings: append([]string{}, p.Query.Warnings...)}
}

type SearchJSON struct {
	Events     []Event `json:"events"`
	NextCursor *string `json:"next_cursor"`
	windowJSON
}

func SearchOf(p Prepared, page Page) SearchJSON {
	out := SearchJSON{Events: page.Events, windowJSON: windowOf(p)}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out
}

type DetailJSON struct {
	Event       Event   `json:"event"`
	SameRequest []Event `json:"same_request"`
	SameRun     []Event `json:"same_run"`
}

func DetailOf(d Detail) DetailJSON {
	return DetailJSON(d)
}

type FacetsJSON struct {
	Field  string  `json:"field"`
	Values []Facet `json:"values"`
	windowJSON
}

func FacetsOf(p Prepared, field string, values []Facet) FacetsJSON {
	return FacetsJSON{Field: field, Values: values, windowJSON: windowOf(p)}
}

type HistogramJSON struct {
	Histogram
	windowJSON
}

func HistogramOf(p Prepared, h Histogram) HistogramJSON {
	return HistogramJSON{Histogram: h, windowJSON: windowOf(p)}
}

type levelJSON struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Field string `json:"field"`
}

type metricJSON struct {
	Name string  `json:"name"`
	Type string  `json:"type"`
	Key  *string `json:"key"`
}

type AggregationJSON struct {
	Levels    []levelJSON  `json:"levels"`
	Metrics   []metricJSON `json:"metrics"`
	Rows      []AggRow     `json:"rows"`
	Truncated bool         `json:"truncated"`
	windowJSON
}

func AggregationOf(p Prepared, a Aggregation) AggregationJSON {
	out := AggregationJSON{Levels: []levelJSON{}, Metrics: []metricJSON{}, Rows: a.Rows, Truncated: a.Truncated, windowJSON: windowOf(p)}
	for _, l := range a.Levels {
		out.Levels = append(out.Levels, levelJSON{Name: l.Name, Type: l.Type, Field: l.Field})
	}
	for _, m := range a.Metrics {
		mj := metricJSON{Name: m.Name, Type: m.Type}
		if m.Key != "" {
			mj.Key = &m.Key
		}
		out.Metrics = append(out.Metrics, mj)
	}
	return out
}

type FieldJSON struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Values      []string `json:"values"`
}

type FieldListJSON struct {
	Fields []FieldJSON `json:"fields"`
}

func FieldsOf() []FieldJSON {
	fields := filter.Fields()
	out := make([]FieldJSON, 0, len(fields))
	for _, f := range fields {
		fj := FieldJSON{Name: f.Name, Type: string(f.Type), Description: f.Description, Values: f.Values}
		fj.Aliases = append([]string{}, f.Aliases...)
		if fj.Values == nil {
			fj.Values = []string{}
		}
		out = append(out, fj)
	}
	return out
}

func (h *Handler) prepare(c *gin.Context, action audit.Action, withCursor bool) (Prepared, bool) {
	req := Request{Text: c.Query("q"), Since: c.Query("since"), Until: c.Query("until")}
	if withCursor {
		req.Cursor = c.Query("cursor")
	}
	p, err := h.reader.Prepare(c.Request.Context(), action, req)
	if err != nil {
		h.fail(c, "prepare audit read", err)
		return Prepared{}, false
	}
	return p, true
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if problem, ok := ProblemOf(err); ok {
		c.AbortWithStatusJSON(problem.Status, problem)
		return
	}
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action+" failed", logattrs.Failure("audit_read_failed", err)...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "the audit trail could not be read")
}

func (h *Handler) search(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, 0, filter.MaxRows)
	if !ok {
		return
	}
	p, ok := h.prepare(c, ActionSearch, true)
	if !ok {
		return
	}
	page, err := h.reader.Search(c.Request.Context(), p, limit)
	if err != nil {
		h.fail(c, "search audit events", err)
		return
	}
	c.JSON(http.StatusOK, SearchOf(p, page))
}

func (h *Handler) event(c *gin.Context) {
	d, err := h.reader.Event(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.fail(c, "read audit event", err)
		return
	}
	c.JSON(http.StatusOK, DetailOf(d))
}

func (h *Handler) facets(c *gin.Context) {
	field := c.Query("field")
	if field == "" {
		httpapi.AbortError(c, http.StatusBadRequest, CodeInvalidRequest, "field is required: name the field to count, such as actor_name; GET /audit/fields lists them")
		return
	}
	limit, ok := httpapi.LimitParam(c, 0, filter.MaxFacets)
	if !ok {
		return
	}
	p, ok := h.prepare(c, ActionSearch, false)
	if !ok {
		return
	}
	values, err := h.reader.Facets(c.Request.Context(), p, field, limit)
	if err != nil {
		h.fail(c, "count audit facets", err)
		return
	}
	c.JSON(http.StatusOK, FacetsOf(p, field, values))
}

func (h *Handler) histogram(c *gin.Context) {
	buckets, ok := bucketsParam(c)
	if !ok {
		return
	}
	p, ok := h.prepare(c, ActionSearch, false)
	if !ok {
		return
	}
	hist, err := h.reader.Histogram(c.Request.Context(), p, buckets)
	if err != nil {
		h.fail(c, "count audit histogram", err)
		return
	}
	c.JSON(http.StatusOK, HistogramOf(p, hist))
}

func bucketsParam(c *gin.Context) (int, bool) {
	raw, present := c.GetQuery("buckets")
	if !present {
		return DefaultBuckets, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > MaxBuckets || strconv.Itoa(n) != raw {
		httpapi.AbortError(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("buckets must be an integer from 1 to %d", MaxBuckets))
		return 0, false
	}
	return n, true
}

func (h *Handler) aggregate(c *gin.Context) {
	p, ok := h.prepare(c, ActionSearch, false)
	if !ok {
		return
	}
	agg, err := h.reader.Aggregate(c.Request.Context(), p)
	if err != nil {
		h.fail(c, "aggregate audit events", err)
		return
	}
	c.JSON(http.StatusOK, AggregationOf(p, agg))
}

func (h *Handler) fields(c *gin.Context) {
	c.JSON(http.StatusOK, FieldListJSON{Fields: FieldsOf()})
}

func (h *Handler) export(c *gin.Context) {
	format := c.DefaultQuery("format", FormatNDJSON)
	contentType, ok := map[string]string{FormatNDJSON: "application/x-ndjson", FormatCSV: "text/csv; charset=utf-8"}[format]
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, CodeInvalidRequest, "format must be ndjson or csv")
		return
	}
	p, ok := h.prepare(c, ActionExport, false)
	if !ok {
		return
	}
	begun := false
	sink := Sink{
		Begin: func() io.Writer {
			begun = true
			name := "audit-" + h.reader.Now().UTC().Format("20060102T150405Z") + "." + format
			c.Header("Content-Type", contentType)
			c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
			c.Header(RowLimitHeader, strconv.Itoa(MaxExportRows))
			c.Header("Trailer", TruncatedHeader)
			c.Status(http.StatusOK)
			return c.Writer
		},
		Flush: c.Writer.Flush,
		Check: func(context.Context) error {
			still, err := StillAdmin(c.Request, h.principals)
			if err == nil && !still {
				err = ErrNoLongerAdmin
			}
			return err
		},
	}
	result, err := h.reader.Export(c.Request.Context(), p, format, MaxExportRows, sink)
	switch {
	case err == nil:
		c.Writer.Header().Set(TruncatedHeader, strconv.FormatBool(result.Truncated))
	case !begun:
		h.fail(c, "export audit events", err)
	case clientgone.Is(c.Request.Context(), err):
	case errors.Is(err, ErrNoLongerAdmin):
		panic(http.ErrAbortHandler)
	default:
		h.logger.ErrorContext(c.Request.Context(), "export audit events failed", logattrs.Failure("audit_export_failed", err)...)
		panic(http.ErrAbortHandler)
	}
}
