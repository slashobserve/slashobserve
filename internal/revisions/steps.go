package revisions

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
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
		s.logger.WarnContext(ctx, "record agent request step", logattrs.Failure("agent_step_encode_failed", err, slog.String("agent_request_id", req.ID), slog.String("tool", st.Tool))...)
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, stepWriteTimeout)
	defer cancel()
	_, err = s.db.Queries().AppendAgentRequestStep(writeCtx, dbsqlc.AppendAgentRequestStepParams{
		ID: id, Attempts: httpapi.Int32(req.Attempt), Step: payload, MaxSteps: MaxSteps,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "record agent request step", logattrs.Failure("agent_step_record_failed", err, slog.String("agent_request_id", req.ID), slog.String("tool", st.Tool))...)
	}
}

func validPhase(phase string) bool {
	switch phase {
	case "discover", "lint", "query", "save", "write", "repair", "ledger", "stage":
		return true
	}
	return false
}

func cleanStepText(s string, maxRunes int) string {
	s = strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "")
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	const ellipsis = "..."
	cut := string(r[:maxRunes-len(ellipsis)])
	if r[maxRunes-len(ellipsis)] != ' ' {
		if i := strings.LastIndexByte(cut, ' '); i > len(cut)/2 {
			cut = cut[:i]
		}
	}
	return strings.TrimRight(cut, " ,;:.") + ellipsis
}
