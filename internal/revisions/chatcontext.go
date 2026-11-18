package revisions

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	maxChatTurns          = 6
	maxContextPromptRunes = 600
	maxContextAnswerRunes = 1200
	maxThreadContextRunes = 8000
)

type chatTurn struct {
	prompt string
	answer string
}

type observeChat struct {
	id      pgtype.UUID
	context string
	retries pgtype.UUID
}

func clipRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n])) + "..."
}

func chatTranscript(turns []chatTurn) string {
	var kept []string
	used := 0
	for i := len(turns) - 1; i >= 0; i-- {
		answer := turns[i].answer
		if answer == "" {
			answer = "No answer or dashboard was saved."
		}
		entry := "You: " + clipRunes(turns[i].prompt, maxContextPromptRunes) + "\nObserve Agent: " + clipRunes(answer, maxContextAnswerRunes)
		size := utf8.RuneCountInString(entry) + 1
		if used+size > maxThreadContextRunes {
			break
		}
		used += size
		kept = append(kept, entry)
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return strings.Join(kept, "\n")
}

func answerOf(row dbsqlc.ListChatTurnsRow) string {
	switch {
	case row.Answer.Valid:
		return row.Answer.String
	case row.ResultVersion.Valid && row.VersionSummary.Valid:
		return fmt.Sprintf("Saved version %s of the dashboard: %s", strconv.Itoa(int(row.ResultVersion.Int32)), row.VersionSummary.String)
	case row.ResultVersion.Valid:
		return fmt.Sprintf("Saved version %s of the dashboard.", strconv.Itoa(int(row.ResultVersion.Int32)))
	}
	return ""
}

func (s *Service) chatTranscript(ctx context.Context, by auth.Principal, chatID, dashboardID pgtype.UUID) (string, error) {
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return "", fmt.Errorf("requester id: %w", err)
	}
	rows, err := s.db.Queries().ListChatTurns(ctx, dbsqlc.ListChatTurnsParams{
		RequestedByKind: string(by.Kind),
		RequestedByID:   requesterID,
		ChatID:          chatID,
		DashboardID:     dashboardID,
		RowLimit:        maxChatTurns,
	})
	if err != nil {
		return "", fmt.Errorf("list chat turns: %w", err)
	}
	turns := make([]chatTurn, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		turns = append(turns, chatTurn{prompt: rows[i].Prompt, answer: answerOf(rows[i])})
	}
	return chatTranscript(turns), nil
}
