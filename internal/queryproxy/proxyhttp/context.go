package proxyhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	defaultContextLines = 50
	maxContextLines     = 500
	defaultWindowMS     = int64(time.Hour / time.Millisecond)
	minWindowMS         = int64(time.Second / time.Millisecond)
	maxWindowMS         = int64(24 * time.Hour / time.Millisecond)
	contextFrameRef     = "A"
	notLineSource       = "log context reads Loki and ClickHouse datasources"
)

func init() {
	registerRoutes(func(h *Handler, api auth.Routes) {
		api.POST("/datasources/:uid/log-context", auth.RoleViewer, h.logContext)
	})
}

type contextRequest struct {
	Query    *string `json:"query"`
	Anchor   *string `json:"anchor"`
	Before   *int    `json:"before"`
	After    *int    `json:"after"`
	WindowMS *int64  `json:"window_ms"`
}

type contextResponse struct {
	Anchor string      `json:"anchor"`
	Before contextSide `json:"before"`
	After  contextSide `json:"after"`
}

type contextSide struct {
	Frames    json.RawMessage `json:"frames"`
	Truncated bool            `json:"truncated"`
}

func (h *Handler) logContext(c *gin.Context) {
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req contextRequest
	if !httpapi.DecodeBody(c, &req, maxExploreBodyBytes) {
		return
	}
	if status, code, message := req.validate(); status != 0 {
		abortError(c, status, code, message)
		return
	}
	uid := c.Param("uid")
	inst, found := h.registry.Get(uid)
	if !found {
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
		return
	}
	source, ok := inst.Datasource.(datasource.LineSource)
	if !ok {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), notLineSource)
		return
	}
	anchor, err := strconv.ParseInt(*req.Anchor, 10, 64)
	if err != nil {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), "anchor must be a nanosecond epoch time")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	query := datasource.Query{Text: *req.Query, Limits: queryproxy.DraftLimits, User: queryproxy.UserFor(principal)}
	at, window := time.Unix(0, anchor), time.Duration(req.window())*time.Millisecond
	sides := [2]datasource.LinesRequest{
		{Query: query, Direction: datasource.Backward, Limit: req.count(req.Before), Range: datasource.TimeRange{From: at.Add(-window), To: at}},
		{Query: query, Direction: datasource.Forward, Limit: req.count(req.After), Range: datasource.TimeRange{From: at.Add(time.Nanosecond), To: at.Add(window)}},
	}
	var results [2]contextSide
	g, gctx := errgroup.WithContext(ctx)
	for i := range sides {
		g.Go(func() error {
			side, err := h.readSide(gctx, source, sides[i])
			results[i] = side
			return err
		})
	}
	if err := g.Wait(); err != nil {
		h.exploreFail(c, uid, err)
		return
	}
	if len(results[0].Frames)+len(results[1].Frames) > maxResponseBytes {
		h.fail(c, errTooLarge)
		return
	}
	c.JSON(http.StatusOK, contextResponse{Anchor: *req.Anchor, Before: results[0], After: results[1]})
}

func (h *Handler) readSide(ctx context.Context, source datasource.LineSource, req datasource.LinesRequest) (contextSide, error) {
	empty := contextSide{Frames: json.RawMessage("[]")}
	if req.Limit == 0 {
		return empty, nil
	}
	if err := req.Range.Validate(); err != nil {
		return empty, err
	}
	release, err := h.acquireUpstreamBounded(ctx)
	if err != nil {
		return empty, err
	}
	defer release()
	frames, err := source.Lines(ctx, req)
	if err != nil {
		return empty, err
	}
	wire, err := appendWireFrames([]wireFrame{}, contextFrameRef, frames)
	if err != nil {
		return empty, err
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return empty, err
	}
	return contextSide{Frames: encoded, Truncated: datasource.MergeMeta(frames).Truncated}, nil
}

func (h *Handler) acquireUpstreamBounded(ctx context.Context) (release func(), err error) {
	waitCtx, cancel := context.WithTimeout(ctx, h.lintTimeout)
	defer cancel()
	release, err = h.runner.AcquireUpstream(waitCtx)
	switch {
	case err == nil:
		return release, nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	}
	return nil, queryproxy.ErrUpstreamBusy
}

func (r contextRequest) validate() (status int, code, message string) {
	invalid := func(message string) (int, string, string) {
		return http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message
	}
	switch {
	case r.Query == nil || *r.Query == "":
		return invalid("query must be from 1 byte to 64 KiB")
	case len(*r.Query) > maxLintQueryBytes:
		return http.StatusBadRequest, string(queryproxy.CodeQueryTooLong), "query must be at most 64 KiB"
	case r.Anchor == nil || *r.Anchor == "":
		return invalid("anchor is required")
	case !lineCount(r.Before) || !lineCount(r.After):
		return invalid("before and after must be from 0 to " + strconv.Itoa(maxContextLines))
	case r.WindowMS != nil && (*r.WindowMS < minWindowMS || *r.WindowMS > maxWindowMS):
		return invalid("window_ms must be from " + strconv.FormatInt(minWindowMS, 10) + " to " + strconv.FormatInt(maxWindowMS, 10))
	}
	return 0, "", ""
}

func lineCount(n *int) bool {
	return n == nil || (*n >= 0 && *n <= maxContextLines)
}

func (r contextRequest) count(n *int) int {
	if n == nil {
		return defaultContextLines
	}
	return *n
}

func (r contextRequest) window() int64 {
	if r.WindowMS == nil {
		return defaultWindowMS
	}
	return *r.WindowMS
}
