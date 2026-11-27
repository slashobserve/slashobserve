package accessrequests

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxBodyBytes      = 4 << 10
	CodeCanComment    = "already_can_comment"
	CodeRequestOpen   = "request_open"
	CodeInvalidNote   = "invalid_note"
	CanCommentMessage = "you can already comment here; there is nothing to request"
)

type ObjectJSON struct {
	Kind  Kind   `json:"kind"`
	ID    string `json:"id"`
	Title string `json:"title"`
}

type FolderJSON struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	General bool   `json:"general"`
}

type RequesterJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RequestJSON struct {
	ID          string        `json:"id"`
	Object      ObjectJSON    `json:"object"`
	Folder      FolderJSON    `json:"folder"`
	RequestedBy RequesterJSON `json:"requested_by"`
	Note        string        `json:"note"`
	CreatedAt   string        `json:"created_at"`
}

type ListJSON struct {
	Items []RequestJSON `json:"items"`
}

func JSONOf(r Request) RequestJSON {
	return RequestJSON{
		ID:          r.ID,
		Object:      ObjectJSON{Kind: r.Object.Kind, ID: r.Object.ID.String(), Title: r.Title},
		Folder:      FolderJSON(r.Folder),
		RequestedBy: RequesterJSON(r.RequestedBy),
		Note:        r.Note,
		CreatedAt:   httpapi.FormatTime(r.CreatedAt),
	}
}

type statedJSON struct {
	RequestJSON
	State State `json:"state"`
}

type statedListJSON struct {
	Items []statedJSON `json:"items"`
}

func statedJSONOf(r Request) statedJSON {
	return statedJSON{RequestJSON: JSONOf(r), State: r.State}
}

func statedListJSONOf(items []Request) statedListJSON {
	out := statedListJSON{Items: make([]statedJSON, 0, len(items))}
	for _, r := range items {
		out.Items = append(out.Items, statedJSONOf(r))
	}
	return out
}

const (
	scopeManaged = "managed"
	scopeMine    = "mine"
)

func ListJSONOf(items []Request) ListJSON {
	out := ListJSON{Items: make([]RequestJSON, 0, len(items))}
	for _, r := range items {
		out.Items = append(out.Items, JSONOf(r))
	}
	return out
}

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.POST("/access-requests", auth.RoleViewer, h.create)
	api.GET("/access-requests", auth.RoleViewer, h.list)
}

type createBody struct {
	Object *struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"object"`
	Note string `json:"note"`
}

func (h *Handler) create(c *gin.Context) {
	var body createBody
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	if body.Object == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "object is required")
		return
	}
	kind, ok := ParseKind(body.Object.Kind)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "object.kind must be dashboard or observation")
		return
	}
	id, err := db.ParseUUID(body.Object.ID)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "object.id must be a UUID")
		return
	}
	request, err := h.service.Request(c.Request.Context(), httpapi.Caller(c), Object{Kind: kind, ID: id}, body.Note)
	if abortRefusal(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "request comment access", err)
		return
	}
	c.JSON(http.StatusCreated, statedJSONOf(request))
}

func abortRefusal(c *gin.Context, err error) bool {
	var open *OpenError
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrInvalidNote):
		httpapi.AbortError(c, http.StatusBadRequest, CodeInvalidNote, err.Error())
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrCanComment):
		httpapi.AbortError(c, http.StatusConflict, CodeCanComment, CanCommentMessage)
	case errors.As(err, &open):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": open.Message(), "code": CodeRequestOpen, "requested_at": httpapi.FormatTime(open.RequestedAt)})
	case errors.Is(err, folders.ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	default:
		return false
	}
	return true
}

func (h *Handler) list(c *gin.Context) {
	read := h.service.List
	switch c.DefaultQuery("scope", scopeManaged) {
	case scopeManaged:
	case scopeMine:
		read = h.service.Mine
	default:
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "scope must be managed or mine")
		return
	}
	items, err := read(c.Request.Context(), httpapi.Caller(c))
	if errors.Is(err, folders.ErrForbidden) {
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
		return
	}
	if err != nil {
		h.fail(c, "list access requests", err)
		return
	}
	c.JSON(http.StatusOK, statedListJSONOf(items))
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}
