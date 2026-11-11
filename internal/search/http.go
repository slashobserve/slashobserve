package search

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

type folderJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type targetJSON struct {
	Kind  Kind   `json:"kind"`
	ID    string `json:"id"`
	Title string `json:"title"`
}

type resultJSON struct {
	Kind      Kind        `json:"kind"`
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Snippet   []Segment   `json:"snippet"`
	URL       *string     `json:"url"`
	Folder    *folderJSON `json:"folder"`
	Target    *targetJSON `json:"target"`
	ThreadID  *string     `json:"thread_id"`
	UpdatedAt string      `json:"updated_at"`
}

type listJSON struct {
	Results    []resultJSON `json:"results"`
	NextCursor *string      `json:"next_cursor"`
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/search", auth.RoleViewer, h.search)
}

func (h *Handler) search(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, DefaultLimit, MaxLimit)
	if !ok {
		return
	}
	kinds, ok := kindsParam(c)
	if !ok {
		return
	}
	page, err := h.service.Search(c.Request.Context(), httpapi.Caller(c), Request{
		Q: c.Query("q"), Kinds: kinds, Limit: limit, Cursor: c.Query("cursor"),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	body := listJSON{Results: make([]resultJSON, 0, len(page.Results))}
	for _, r := range page.Results {
		body.Results = append(body.Results, jsonOf(r))
	}
	if page.Next != "" {
		body.NextCursor = &page.Next
	}
	c.JSON(http.StatusOK, body)
}

func kindsParam(c *gin.Context) ([]Kind, bool) {
	var kinds []Kind
	for _, raw := range c.QueryArray("kind") {
		kind := Kind(raw)
		if !kind.Valid() {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "kind must be one of dashboard, finding, annotation, observation, comment, saved_search")
			return nil, false
		}
		kinds = append(kinds, kind)
	}
	return kinds, true
}

func jsonOf(r Result) resultJSON {
	out := resultJSON{
		Kind:      r.Kind,
		ID:        r.ID,
		Title:     r.Title,
		Snippet:   r.Snippet,
		UpdatedAt: r.ChangedAt.UTC().Format(time.RFC3339),
	}
	if r.URL != "" {
		out.URL = &r.URL
	}
	if r.Folder != nil {
		out.Folder = &folderJSON{ID: r.Folder.ID, Title: r.Folder.Title}
	}
	if r.Target != nil {
		out.Target = &targetJSON{Kind: r.Target.Kind, ID: r.Target.ID, Title: r.Target.Title}
		out.ThreadID = &r.ThreadID
	}
	return out
}

func (h *Handler) fail(c *gin.Context, err error) {
	var invalid *InvalidQueryError
	switch {
	case errors.As(err, &invalid):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", invalid.Message)
	case errors.Is(err, ErrInvalidCursor):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
	case errors.Is(err, folders.ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.AccessMessage)
	case clientgone.Handle(c, h.logger, "search workspace", err):
	default:
		h.logger.ErrorContext(c.Request.Context(), "search workspace", logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
