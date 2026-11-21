package threads

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type Answer struct {
	RunID           string
	FindingID       string
	Title           string
	Markdown        string
	EvidencePanels  int
	SuggestedChange string
	ObservationID   string
	CanSave         bool
	Finding         json.RawMessage
}

type Answers interface {
	ForRuns(ctx context.Context, by auth.Principal, repliedIn map[pgtype.UUID]pgtype.UUID) (map[pgtype.UUID]Answer, error)
}

func (s *Service) ReadAnswersWith(answers Answers) {
	s.answers = answers
}

func (s *Service) answersOf(ctx context.Context, by auth.Principal, rows []dbsqlc.ListThreadMessagesRow) (map[pgtype.UUID]Answer, error) {
	repliedIn := map[pgtype.UUID]pgtype.UUID{}
	for _, r := range rows {
		if r.ThreadMessage.RunID.Valid {
			repliedIn[r.ThreadMessage.RunID] = r.ThreadMessage.ThreadID
		}
	}
	if s.answers == nil || len(repliedIn) == 0 {
		return nil, nil
	}
	return s.answers.ForRuns(ctx, by, repliedIn)
}

func answerOf(answers map[pgtype.UUID]Answer, row dbsqlc.ThreadMessage) *Answer {
	if !row.RunID.Valid {
		return nil
	}
	if a, ok := answers[row.RunID]; ok {
		return &a
	}
	return nil
}
