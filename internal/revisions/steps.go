package revisions

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	stepWriteTimeout = 2 * time.Second
	maxStepToolRunes = 64
)

type Steps struct {
	db     *db.DB
	logger *slog.Logger
}

func NewSteps(database *db.DB, logger *slog.Logger) *Steps {
	return &Steps{db: database, logger: logger}
}

func (s *Steps) RecordStep(ctx context.Context, req Request, st Step) {
	if !validPhase(st.Phase) || (st.Status != "ok" && st.Status != "error") || req.Attempt < 1 {
		return
	}
	id, err := db.ParseUUID(req.ID)
	if err != nil {
		return
	}
	st.Tool = cleanStepText(st.Tool, maxStepToolRunes)
	st.Message = cleanStepText(st.Message, MaxStepMessageRunes)
	st.Attempt = req.Attempt
	st.At = st.At.UTC()
	st.DurationMS = max(st.DurationMS, 0)
	payload, err := json.Marshal(st)
	if err != nil {
		s.logger.WarnContext(ctx, "record agent request step", slog.String("revision_id", req.ID), slog.String("tool", st.Tool), slog.Any("error", err))
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, stepWriteTimeout)
	defer cancel()
	_, err = s.db.Queries().AppendAgentRequestStep(writeCtx, dbsqlc.AppendAgentRequestStepParams{
		ID: id, Attempts: toInt32(req.Attempt), Step: payload, MaxSteps: MaxSteps,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "record agent request step", slog.String("revision_id", req.ID), slog.String("tool", st.Tool), slog.Any("error", err))
	}
}

func validPhase(phase string) bool {
	switch phase {
	case "discover", "lint", "query", "save", "repair":
		return true
	}
	return false
}

func cleanStepText(s string, maxRunes int) string {
	s = strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "")
	if r := []rune(s); len(r) > maxRunes {
		return string(r[:maxRunes])
	}
	return s
}
