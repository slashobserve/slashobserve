package threads

import (
	"context"
	"encoding/json"
	"fmt"

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
	runs, err := s.answeredRuns(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := map[pgtype.UUID]Answer{}
	for _, run := range runs {
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

func (s *Service) answeredRuns(ctx context.Context, rows []dbsqlc.ListThreadMessagesRow) ([]pgtype.UUID, error) {
	var replied []pgtype.UUID
	seen := map[pgtype.UUID]bool{}
	for _, r := range rows {
		if run := r.ThreadMessage.RunID; run.Valid && !seen[run] {
			seen[run] = true
			replied = append(replied, run)
		}
	}
	if len(replied) == 0 {
		return nil, nil
	}
	answered, err := s.db.Queries().ListAnsweredRuns(ctx, replied)
	if err != nil {
		return nil, fmt.Errorf("list the page's runs that saved an answer: %w", err)
	}
	return answered, nil
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
