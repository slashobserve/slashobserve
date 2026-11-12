package activity

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	defaultInboxLimit = 50
	maxInboxLimit     = 100
	maxReadBodyBytes  = 1 << 10
	inboxSessionText  = "the inbox belongs to a signed-in person; service-account keys have none"
)

type InboxItemJSON struct {
	EventJSON
	Reason *string `json:"reason"`
	Unread bool    `json:"unread"`
}

type InboxJSON struct {
	Items       []InboxItemJSON `json:"items"`
	NextCursor  *string         `json:"next_cursor"`
	UnreadCount int             `json:"unread_count"`
}

func InboxJSONOf(page InboxPage) InboxJSON {
	body := InboxJSON{Items: make([]InboxItemJSON, 0, len(page.Items)), UnreadCount: page.UnreadCount}
	for _, item := range page.Items {
		body.Items = append(body.Items, InboxItemJSON{
			EventJSON: JSONOf(item.Event),
			Reason:    httpapi.OptionalString(string(item.Reason)),
			Unread:    item.Unread,
		})
	}
	if page.Next != "" {
		body.NextCursor = &page.Next
	}
	return body
}

type markReadRequest struct {
	Seq *int64 `json:"seq"`
}

func (h *Handler) registerInbox(api auth.Routes) {
	api.GET("/inbox", auth.RoleViewer, requireSession, h.inbox)
	api.POST("/inbox/read", auth.RoleViewer, requireSession, h.markRead)
}

func requireSession(c *gin.Context) {
	if httpapi.Caller(c).Kind != auth.KindUser {
		httpapi.AbortError(c, http.StatusForbidden, "user_session_required", inboxSessionText)
		return
	}
	c.Next()
}

func inboxLimit(c *gin.Context) (int, bool) {
	raw, present := c.GetQuery("limit")
	if !present {
		return defaultInboxLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxInboxLimit || strconv.Itoa(n) != raw {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_limit", "limit must be an integer from 1 to "+strconv.Itoa(maxInboxLimit))
		return 0, false
	}
	return n, true
}

func (h *Handler) inbox(c *gin.Context) {
	view := ViewForYou
	if raw, present := c.GetQuery("view"); present {
		parsed, ok := ParseView(raw)
		if !ok {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_view", "view must be for_you, everything, proposals or mentions")
			return
		}
		view = parsed
	}
	limit, ok := inboxLimit(c)
	if !ok {
		return
	}
	cursor := c.Query("cursor")
	if cursor != "" {
		if _, err := decodeCursor(cursor); err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_cursor", "cursor is not valid")
			return
		}
	}
	page, err := h.store.Inbox(c.Request.Context(), httpapi.Caller(c), view, cursor, limit)
	if err != nil {
		h.fail(c, "list inbox", err)
		return
	}
	body := InboxJSONOf(page)
	c.JSON(http.StatusOK, body)
}

func (h *Handler) markRead(c *gin.Context) {
	var body markReadRequest
	if !httpapi.DecodeBody(c, &body, maxReadBodyBytes) {
		return
	}
	if body.Seq == nil || *body.Seq < 1 {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "seq must be a positive integer")
		return
	}
	err := h.store.MarkRead(c.Request.Context(), httpapi.Caller(c), *body.Seq)
	if errors.Is(err, ErrUnknownSeq) {
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "unknown_seq", "no activity has that seq")
		return
	}
	if err != nil {
		h.fail(c, "mark inbox read", err)
		return
	}
	c.Status(http.StatusNoContent)
}
