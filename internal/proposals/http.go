package proposals

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxBodyBytes   = 64 << 10
	maxChoices     = 400
	proposeModeMsg = "This connection is in propose mode, so it cannot merge; ask a person who can edit to merge it."
)

type Handler struct {
	svc    *Service
	view   gin.HandlerFunc
	logger *slog.Logger
}

func NewHandler(svc *Service, view gin.HandlerFunc, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, view: view, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/dashboards/:id/proposals", auth.RoleViewer, h.view, h.list)
	api.GET("/proposals/:id", auth.RoleViewer, h.get)
	api.POST("/proposals/:id/merge", auth.RoleViewer, h.merge)
	api.POST("/proposals/:id/discard", auth.RoleViewer, h.discard)
}

type choiceRequest struct {
	Kind spec.UnitKind `json:"kind"`
	ID   string        `json:"id"`
	Take spec.Side     `json:"take"`
}

type mergeRequest struct {
	Revision *int            `json:"revision"`
	Choices  []choiceRequest `json:"choices"`
}

type discardRequest struct{}

func (h *Handler) list(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	page, err := h.svc.List(c.Request.Context(), httpapi.Caller(c), ListQuery{
		DashboardID: id,
		State:       c.Query("state"),
		Limit:       limit,
		Cursor:      c.Query("cursor"),
	})
	if err != nil {
		h.fail(c, "list proposals", err)
		return
	}
	body := listJSON{Proposals: make([]SummaryJSON, len(page.Items)), NextCursor: httpapi.OptionalString(page.NextCursor)}
	for i, item := range page.Items {
		body.Proposals[i] = NewSummaryJSON(item)
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	p, err := h.svc.Get(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "get proposal", err)
		return
	}
	c.JSON(http.StatusOK, NewProposalJSON(p))
}

func (h *Handler) merge(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req mergeRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	if req.Revision == nil || *req.Revision < 1 || *req.Revision > math.MaxInt32 {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "revision is required and must be a positive 32-bit integer")
		return
	}
	if len(req.Choices) > maxChoices {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "choices takes at most "+strconv.Itoa(maxChoices)+" items")
		return
	}
	choices := make([]spec.Choice, len(req.Choices))
	for i, choice := range req.Choices {
		choices[i] = spec.Choice(choice)
	}
	p, err := h.svc.Merge(c.Request.Context(), httpapi.Caller(c), id, *req.Revision, choices)
	if err != nil {
		h.fail(c, "merge proposal", err)
		return
	}
	if p.MergedVersion != nil {
		c.Header("Location", "/api/v1/dashboards/"+p.DashboardID+"/versions/"+strconv.Itoa(*p.MergedVersion))
	}
	c.JSON(http.StatusOK, NewSummaryJSON(p.Item))
}

func (h *Handler) discard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req discardRequest
	if !decodeOptionalBody(c, &req) {
		return
	}
	p, err := h.svc.Discard(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "discard proposal", err)
		return
	}
	c.JSON(http.StatusOK, NewSummaryJSON(p.Item))
}

func decodeOptionalBody(c *gin.Context, dst any) bool {
	body := bufio.NewReader(c.Request.Body)
	if _, err := body.Peek(1); errors.Is(err, io.EOF) {
		return true
	}
	c.Request.Body = io.NopCloser(body)
	return httpapi.DecodeBody(c, dst, maxBodyBytes)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	var refusal *lifecycle.Refusal
	var changed *ChangedError
	switch {
	case errors.As(err, &refusal):
		lifecycle.AbortRefusal(c, refusal)
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrProposeMode):
		httpapi.AbortError(c, http.StatusForbidden, "propose_mode", proposeModeMsg)
	case errors.Is(err, ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	case errors.As(err, &changed):
		c.AbortWithStatusJSON(http.StatusConflict, changedJSON{Error: "the proposal has a newer revision", Code: "proposal_changed", Revision: changed.Revision})
	default:
		h.svc.dashboards.Fail(c, action, err)
	}
}
