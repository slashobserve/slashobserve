package revisions

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

var (
	ErrNoAnswer        = errors.New("the request has no answer to save")
	ErrNoAnswerToShare = errors.New("the request has not finished with an answer to share")
	ErrNotShareable    = errors.New("only a private question about a dashboard or an observation, asked in a chat, can be shared")
	ErrShareThread     = errors.New("thread_id must be a thread you can comment in, on the dashboard or observation the question was about")
)

type SavedAnswer struct {
	ObservationID string
	Outcome       string
}

const (
	AnswerCreated   = "created"
	AnswerKept      = "kept"
	AnswerUnchanged = "unchanged"
)

type AnswerSaver func(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (SavedAnswer, error)

type savedAnswerJSON struct {
	ObservationID string `json:"observation_id"`
	URL           string `json:"url"`
	Replayed      bool   `json:"replayed"`
	Outcome       string `json:"outcome"`
}

func (h *ObserveHandler) saveAnswer(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx, caller := c.Request.Context(), httpapi.Caller(c)
	idAttr := slog.String("agent_request_id", id.String())
	row, err := h.service.GetObserve(ctx, id, caller)
	if err == nil && !isRequester(row, caller) {
		err = ErrRequestNotFound
	}
	var saved SavedAnswer
	if err == nil {
		saved, err = h.opts.SaveAnswer(ctx, caller, row.ID)
	}
	if errors.Is(err, ErrNoAnswer) {
		httpapi.AbortError(c, http.StatusConflict, "no_answer", ErrNoAnswer.Error())
		return
	}
	if err != nil {
		h.failBuild(c, "save agent request answer as observation", err, idAttr)
		return
	}
	status := http.StatusOK
	if saved.Outcome == AnswerCreated {
		status = http.StatusCreated
	}
	c.JSON(status, savedAnswerJSON{ObservationID: saved.ObservationID, URL: h.opts.ObservationURL(saved.ObservationID), Replayed: saved.Outcome == AnswerUnchanged, Outcome: saved.Outcome})
}

type SharedAnswer struct {
	ThreadID  string
	MessageID string
	Replayed  bool
}

type AnswerSharer func(ctx context.Context, by auth.Principal, requestID, threadID pgtype.UUID) (SharedAnswer, error)

type shareAnswerRequest struct {
	ThreadID *string `json:"thread_id"`
}

type sharedAnswerJSON struct {
	ThreadID  string `json:"thread_id"`
	MessageID string `json:"message_id"`
	Replayed  bool   `json:"replayed"`
}

func (h *ObserveHandler) shareAnswer(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req shareAnswerRequest
	if !decodeOptionalBody(c, &req, maxRequestBytes) {
		return
	}
	var threadID pgtype.UUID
	if req.ThreadID != nil {
		parsed, err := db.ParseUUID(*req.ThreadID)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "thread_id must be a UUID")
			return
		}
		threadID = parsed
	}
	ctx, caller := c.Request.Context(), httpapi.Caller(c)
	row, err := h.service.GetObserve(ctx, id, caller)
	if err == nil && !isRequester(row, caller) {
		err = ErrRequestNotFound
	}
	var shared SharedAnswer
	if err == nil {
		shared, err = h.opts.ShareAnswer(ctx, caller, row.ID, threadID)
	}
	if err != nil {
		if !failShare(c, err) {
			h.failBuild(c, "share agent request answer", err, slog.String("agent_request_id", id.String()))
		}
		return
	}
	status := http.StatusCreated
	if shared.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, sharedAnswerJSON(shared))
}

func failShare(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, ErrNoAnswerToShare):
		httpapi.AbortError(c, http.StatusConflict, "no_answer", err.Error())
	case errors.Is(err, ErrNotShareable):
		httpapi.AbortError(c, http.StatusConflict, "not_shareable", err.Error())
	case errors.Is(err, ErrShareThread):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_thread", err.Error())
	case folders.AbortRestricted(c, err):
	default:
		return false
	}
	return true
}
