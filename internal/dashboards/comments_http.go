package dashboards

import (
	"errors"
	"net/http"

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

func (cm *Comments) Register(api *gin.RouterGroup) {
	g := api.Group("/dashboards/:id/comments")
	g.POST("", cm.createComment)
	g.GET("", cm.listComments)
	g.PATCH("/:comment_id", cm.patchComment)
}

func (cm *Comments) createComment(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req createCommentRequest
	if !decodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	if req.Version == nil || req.PanelID == nil || req.Body == nil {
		abortError(c, http.StatusBadRequest, "invalid_request", "version, panel_id and body are required")
		return
	}
	comment, err := cm.Create(c.Request.Context(), caller(c), id, *req.Version, *req.PanelID, *req.Body)
	if err != nil {
		cm.fail(c, "create comment", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+comment.DashboardID+"/comments/"+comment.ID)
	c.JSON(http.StatusCreated, commentEnvelopeJSON{Comment: newCommentJSON(comment)})
}

func (cm *Comments) listComments(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	limit, ok := limitParam(c, defaultCommentLimit, maxCommentLimit)
	if !ok {
		return
	}
	status, ok := nonEmptyQuery(c, "status")
	if !ok {
		return
	}
	panelID, ok := nonEmptyQuery(c, "panel_id")
	if !ok {
		return
	}
	items, next, err := cm.List(c.Request.Context(), id, status, panelID, limit, c.Query("cursor"))
	if err != nil {
		cm.fail(c, "list comments", err)
		return
	}
	body := commentListJSON{Comments: make([]commentJSON, 0, len(items)), NextCursor: optionalString(next)}
	for _, item := range items {
		body.Comments = append(body.Comments, newCommentJSON(item))
	}
	c.JSON(http.StatusOK, body)
}

func (cm *Comments) patchComment(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	commentID, ok := uuidParam(c, "comment_id")
	if !ok {
		return
	}
	var req setCommentStatusRequest
	if !decodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	if req.Status == nil {
		abortError(c, http.StatusBadRequest, "invalid_request", "status is required")
		return
	}
	comment, err := cm.SetStatus(c.Request.Context(), caller(c), id, commentID, *req.Status)
	if err != nil {
		cm.fail(c, "set comment status", err)
		return
	}
	c.JSON(http.StatusOK, commentEnvelopeJSON{Comment: newCommentJSON(comment)})
}

func (cm *Comments) fail(c *gin.Context, action string, err error) {
	var invalid *InvalidInputError
	switch {
	case errors.Is(err, ErrCommentNotFound):
		abortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrUnknownPanel):
		abortError(c, http.StatusUnprocessableEntity, "unknown_panel", err.Error())
	case errors.As(err, &invalid):
		abortError(c, http.StatusBadRequest, "invalid_request", invalid.Message)
	default:
		cm.dashboards.fail(c, action, err)
	}
}

func nonEmptyQuery(c *gin.Context, name string) (string, bool) {
	value, present := c.GetQuery(name)
	if present && value == "" {
		abortError(c, http.StatusBadRequest, "invalid_request", name+" must not be empty")
		return "", false
	}
	return value, true
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
