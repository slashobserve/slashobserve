package dashboards

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	maxCommentRequestBytes = 64 << 10
	defaultCommentLimit    = 100
	maxCommentLimit        = 500
)

type createCommentRequest struct {
	Version *int    `json:"version"`
	PanelID *string `json:"panel_id"`
	Body    *string `json:"body"`
}

type setCommentStatusRequest struct {
	Status *string `json:"status"`
}

type commentRevisionJSON struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type commentJSON struct {
	ID           string               `json:"id"`
	DashboardID  string               `json:"dashboard_id"`
	Version      int                  `json:"version"`
	PanelID      string               `json:"panel_id"`
	Body         string               `json:"body"`
	Status       string               `json:"status"`
	Author       authorJSON           `json:"author"`
	CreatedAt    string               `json:"created_at"`
	UpdatedAt    string               `json:"updated_at"`
	ResolvedAt   *string              `json:"resolved_at"`
	ResolvedBy   *authorJSON          `json:"resolved_by"`
	PanelPresent bool                 `json:"panel_present"`
	Revision     *commentRevisionJSON `json:"revision"`
}

type commentEnvelopeJSON struct {
	Comment commentJSON `json:"comment"`
}

type commentListJSON struct {
	Comments   []commentJSON `json:"comments"`
	NextCursor *string       `json:"next_cursor"`
}

func (c *Comments) Register(api *gin.RouterGroup) {
	g := api.Group("/dashboards/:id/comments")
	g.POST("", c.createComment)
	g.GET("", c.listComments)
	g.PATCH("/:comment_id", c.patchComment)
}

func (c *Comments) createComment(c2 *gin.Context) {
	id, ok := uuidParam(c2, "id")
	if !ok {
		return
	}
	c2.Request.Body = http.MaxBytesReader(c2.Writer, c2.Request.Body, maxCommentRequestBytes)
	var req createCommentRequest
	if !decodeBody(c2, &req) {
		return
	}
	if req.Version == nil || req.PanelID == nil || req.Body == nil {
		abortError(c2, http.StatusBadRequest, "invalid_request", "version, panel_id and body are required")
		return
	}
	comment, err := c.Create(c2.Request.Context(), caller(c2), id, *req.Version, *req.PanelID, *req.Body)
	if err != nil {
		c.fail(c2, "create comment", err)
		return
	}
	c2.Header("Location", "/api/v1/dashboards/"+comment.DashboardID+"/comments/"+comment.ID)
	c2.JSON(http.StatusCreated, commentEnvelopeJSON{Comment: newCommentJSON(comment)})
}

func (c *Comments) listComments(c2 *gin.Context) {
	id, ok := uuidParam(c2, "id")
	if !ok {
		return
	}
	limit, ok := commentLimitParam(c2)
	if !ok {
		return
	}
	items, next, err := c.List(c2.Request.Context(), id, c2.Query("status"), c2.Query("panel_id"), limit, c2.Query("cursor"))
	if err != nil {
		c.fail(c2, "list comments", err)
		return
	}
	body := commentListJSON{Comments: make([]commentJSON, 0, len(items)), NextCursor: optionalString(next)}
	for _, item := range items {
		body.Comments = append(body.Comments, newCommentJSON(item))
	}
	c2.JSON(http.StatusOK, body)
}

func (c *Comments) patchComment(c2 *gin.Context) {
	id, ok := uuidParam(c2, "id")
	if !ok {
		return
	}
	commentID, ok := uuidParam(c2, "comment_id")
	if !ok {
		return
	}
	c2.Request.Body = http.MaxBytesReader(c2.Writer, c2.Request.Body, maxCommentRequestBytes)
	var req setCommentStatusRequest
	if !decodeBody(c2, &req) {
		return
	}
	if req.Status == nil {
		abortError(c2, http.StatusBadRequest, "invalid_request", "status is required")
		return
	}
	comment, err := c.SetStatus(c2.Request.Context(), caller(c2), id, commentID, *req.Status)
	if err != nil {
		c.fail(c2, "set comment status", err)
		return
	}
	c2.JSON(http.StatusOK, commentEnvelopeJSON{Comment: newCommentJSON(comment)})
}

func (c *Comments) fail(c2 *gin.Context, action string, err error) {
	ctx := c2.Request.Context()
	var invalid *InvalidInputError
	switch {
	case errors.Is(err, context.Canceled):
		c.logger.DebugContext(ctx, action+": client went away")
		c2.AbortWithStatus(statusClientClosedRequest)
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrCommentNotFound):
		abortError(c2, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrUnknownPanel):
		abortError(c2, http.StatusUnprocessableEntity, "unknown_panel", err.Error())
	case errors.As(err, &invalid):
		abortError(c2, http.StatusBadRequest, "invalid_request", invalid.Message)
	case errors.Is(err, ErrInvalidCursor):
		abortError(c2, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		c.logger.ErrorContext(ctx, action, "error", err)
		abortError(c2, http.StatusInternalServerError, "internal", "internal error")
	}
}

func commentLimitParam(c *gin.Context) (int, bool) {
	raw, present := c.GetQuery("limit")
	if !present {
		return defaultCommentLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxCommentLimit || strconv.Itoa(n) != raw {
		abortError(c, http.StatusBadRequest, "invalid_request", "limit must be an integer from 1 to 500")
		return 0, false
	}
	return n, true
}

func newCommentJSON(c Comment) commentJSON {
	out := commentJSON{
		ID:           c.ID,
		DashboardID:  c.DashboardID,
		Version:      c.Version,
		PanelID:      c.PanelID,
		Body:         c.Body,
		Status:       c.Status,
		Author:       newAuthorJSON(c.Author),
		CreatedAt:    formatTime(c.CreatedAt),
		UpdatedAt:    formatTime(c.UpdatedAt),
		PanelPresent: c.PanelPresent,
	}
	if c.ResolvedAt != nil {
		at := formatTime(*c.ResolvedAt)
		out.ResolvedAt = &at
	}
	if c.ResolvedBy != nil {
		by := newAuthorJSON(*c.ResolvedBy)
		out.ResolvedBy = &by
	}
	return out
}
