package revisions

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

var ErrNoAnswer = errors.New("the request has no answer to save")

type SavedAnswer struct {
	ObservationID string
	Replayed      bool
}

type AnswerSaver func(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (SavedAnswer, error)

type savedAnswerJSON struct {
	ObservationID string `json:"observation_id"`
	URL           string `json:"url"`
	Replayed      bool   `json:"replayed"`
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
	status := http.StatusCreated
	if saved.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, savedAnswerJSON{ObservationID: saved.ObservationID, URL: h.opts.ObservationURL(saved.ObservationID), Replayed: saved.Replayed})
}
