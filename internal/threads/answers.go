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

type RunGate interface {
	ShowsRunAnswer(ctx context.Context, run pgtype.UUID, by auth.Principal) (bool, error)
}

type AnswerReader interface {
	RunAnswer(ctx context.Context, by auth.Principal, run pgtype.UUID) (Answer, bool, error)
}

type answerReads struct {
	runs    RunGate
	answers AnswerReader
}

func (s *Service) ReadAnswersWith(runs RunGate, answers AnswerReader) {
	s.answers = &answerReads{runs: runs, answers: answers}
}

func (s *Service) answersOf(ctx context.Context, by auth.Principal, rows []dbsqlc.ListThreadMessagesRow) (map[pgtype.UUID]Answer, error) {
	if s.answers == nil {
		return nil, nil
	}
	out := map[pgtype.UUID]Answer{}
	seen := map[pgtype.UUID]bool{}
	for _, r := range rows {
		run := r.ThreadMessage.RunID
		if !run.Valid || seen[run] {
			continue
		}
		seen[run] = true
		a, found, err := s.answers.read(ctx, by, run)
		if err != nil {
			return nil, err
		}
		if found {
			out[run] = a
		}
	}
	return out, nil
}

func (r *answerReads) read(ctx context.Context, by auth.Principal, run pgtype.UUID) (Answer, bool, error) {
	shown, err := r.runs.ShowsRunAnswer(ctx, run, by)
	if err != nil || !shown {
		return Answer{}, false, err
	}
	return r.answers.RunAnswer(ctx, by, run)
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
