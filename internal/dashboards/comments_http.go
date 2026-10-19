package dashboards

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	maxCommentRequestBytes = 64 << 10
	defaultCommentLimit    = 100
	maxCommentLimit        = 500
)

type createCommentRequest struct {
	Version         *int    `json:"version"`
	PanelID         *string `json:"panel_id"`
	ParentID        *string `json:"parent_id"`
	Body            *string `json:"body"`
	RequestRevision *bool   `json:"request_revision"`
}

type setCommentStatusRequest struct {
	Status            *string `json:"status"`
	ResolvedByVersion *int    `json:"resolved_by_version"`
}

type commentRevisionJSON struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type commentJSON struct {
	ID                string               `json:"id"`
	DashboardID       string               `json:"dashboard_id"`
	ParentID          *string              `json:"parent_id"`
	Version           int                  `json:"version"`
	PanelID           *string              `json:"panel_id"`
	Body              string               `json:"body"`
	Status            string               `json:"status"`
	Author            authorJSON           `json:"author"`
	CreatedAt         string               `json:"created_at"`
	UpdatedAt         string               `json:"updated_at"`
	ResolvedAt        *string              `json:"resolved_at"`
	ResolvedBy        *authorJSON          `json:"resolved_by"`
	ResolvedByVersion *int                 `json:"resolved_by_version"`
	PanelPresent      bool                 `json:"panel_present"`
	Revision          *commentRevisionJSON `json:"revision"`
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
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req createCommentRequest
	if !httpapi.DecodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	if req.Body == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "body is required")
		return
	}
	requestRevision := req.RequestRevision != nil && *req.RequestRevision
	in := NewComment{Body: *req.Body, RequestRevision: requestRevision}
	if req.ParentID != nil {
		parentID, err := db.ParseUUID(*req.ParentID)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "parent_id must be a UUID")
			return
		}
		if req.Version != nil || req.PanelID != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "a reply takes its version and panel from its thread; omit version and panel_id")
			return
		}
		in.ParentID = parentID
	} else {
		if req.Version == nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "version is required")
			return
		}
		in.Version = *req.Version
		in.PanelID = req.PanelID
	}
	comment, err := cm.Create(c.Request.Context(), httpapi.Caller(c), id, in)
	if err != nil {
		cm.fail(c, "create comment", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+comment.DashboardID+"/comments/"+comment.ID)
	c.JSON(http.StatusCreated, commentEnvelopeJSON{Comment: newCommentJSON(comment)})
}

func (cm *Comments) listComments(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	limit, ok := httpapi.LimitParam(c, defaultCommentLimit, maxCommentLimit)
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
	body := commentListJSON{Comments: make([]commentJSON, 0, len(items)), NextCursor: httpapi.OptionalString(next)}
	for _, item := range items {
		body.Comments = append(body.Comments, newCommentJSON(item))
	}
	c.JSON(http.StatusOK, body)
}

func (cm *Comments) patchComment(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	commentID, ok := httpapi.UUIDParam(c, "comment_id")
	if !ok {
		return
	}
	var req setCommentStatusRequest
	if !httpapi.DecodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	if req.Status == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "status is required")
		return
	}
	comment, _, err := cm.SetStatus(c.Request.Context(), httpapi.Caller(c), id, commentID, *req.Status, req.ResolvedByVersion)
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
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrUnknownPanel):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "unknown_panel", err.Error())
	case errors.Is(err, ErrUnknownVersion):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "unknown_version", err.Error())
	case errors.Is(err, revisions.ErrTooManyRevisions):
		httpapi.AbortError(c, http.StatusTooManyRequests, "too_many_revisions", err.Error())
	case errors.As(err, &invalid):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", invalid.Message)
	default:
		cm.dashboards.fail(c, action, err)
	}
}

func nonEmptyQuery(c *gin.Context, name string) (string, bool) {
	value, present := c.GetQuery(name)
	if present && value == "" {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must not be empty")
		return "", false
	}
	return value, true
}

func newCommentJSON(c Comment) commentJSON {
	out := commentJSON{
		ID:                c.ID,
		DashboardID:       c.DashboardID,
		ParentID:          httpapi.OptionalString(c.ParentID),
		Version:           c.Version,
		PanelID:           httpapi.OptionalString(c.PanelID),
		Body:              c.Body,
		Status:            c.Status,
		Author:            newAuthorJSON(c.Author),
		CreatedAt:         httpapi.FormatTime(c.CreatedAt),
		UpdatedAt:         httpapi.FormatTime(c.UpdatedAt),
		ResolvedByVersion: c.ResolvedByVersion,
		PanelPresent:      c.PanelPresent,
	}
	if c.ResolvedAt != nil {
		at := httpapi.FormatTime(*c.ResolvedAt)
		out.ResolvedAt = &at
	}
	if c.ResolvedBy != nil {
		by := newAuthorJSON(*c.ResolvedBy)
		out.ResolvedBy = &by
	}
	if c.Revision != nil {
		out.Revision = &commentRevisionJSON{ID: c.Revision.ID, Status: c.Revision.Status}
	}
	return out
}
