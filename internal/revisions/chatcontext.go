package revisions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	actionChatCreate  audit.Action = "chat.create"
	actionChatTurn    audit.Action = "chat.turn"
	actionChatArchive audit.Action = "chat.archive"
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

type chatMode int

const (
	noChat chatMode = iota
	newChat
	givenChat
	sameChat
	dashboardChat
)

type chatChoice struct {
	mode chatMode
	id   pgtype.UUID
}

func chatNamed(id pgtype.UUID) chatChoice {
	if !id.Valid {
		return chatChoice{mode: newChat}
	}
	return chatChoice{mode: givenChat, id: id}
}

type chatSubject struct {
	dashboard, observation pgtype.UUID
}

func (c chatSubject) of(row dbsqlc.Chat) bool {
	return row.DashboardID == c.dashboard && row.ObservationID == c.observation
}

type chatSlot struct {
	chat, turn pgtype.UUID
	created    bool
	subject    chatSubject
}

func lockChatSubject(ctx context.Context, q *dbsqlc.Queries, subject chatSubject) error {
	var err error
	switch {
	case subject.observation.Valid:
		_, err = q.LockObservation(ctx, subject.observation)
	case subject.dashboard.Valid:
		_, err = q.LockDashboardKeyShare(ctx, subject.dashboard)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock the chat subject: %w", err)
	}
	return nil
}

func takeChat(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, choice chatChoice, subject chatSubject) (chatSlot, error) {
	if choice.mode == noChat {
		return chatSlot{}, nil
	}
	ownerID, err := db.ParseUUID(by.ID)
	if err != nil {
		return chatSlot{}, fmt.Errorf("chat owner id: %w", err)
	}
	if err := lockChatSubject(ctx, q, subject); err != nil {
		return chatSlot{}, err
	}
	owner := string(by.Kind)
	switch choice.mode {
	case givenChat, sameChat:
		row, err := q.TouchChat(ctx, dbsqlc.TouchChatParams{ID: choice.id, OwnerKind: owner, OwnerID: ownerID})
		switch {
		case err == nil && (choice.mode == sameChat || subject.of(row)):
			return chatSlot{chat: row.ID, subject: subject}, nil
		case err == nil:
			return chatSlot{}, refused("chat_id names a chat about another dashboard or observation")
		case !errors.Is(err, pgx.ErrNoRows):
			return chatSlot{}, fmt.Errorf("touch chat: %w", err)
		case choice.mode == givenChat:
			return chatSlot{}, chatRefusal(ctx, q, by, choice.id)
		}
	case dashboardChat:
		row, err := q.TouchLatestDashboardChat(ctx, dbsqlc.TouchLatestDashboardChatParams{OwnerKind: owner, OwnerID: ownerID, DashboardID: subject.dashboard})
		switch {
		case err == nil:
			return chatSlot{chat: row.ID, subject: subject}, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return chatSlot{}, fmt.Errorf("touch the latest chat on the dashboard: %w", err)
		}
	}
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	if _, err := q.CreateChat(ctx, dbsqlc.CreateChatParams{ID: id, OwnerKind: owner, OwnerID: ownerID, DashboardID: subject.dashboard, ObservationID: subject.observation}); err != nil {
		return chatSlot{}, fmt.Errorf("create chat: %w", err)
	}
	return chatSlot{chat: id, turn: id, created: true, subject: subject}, nil
}

func recordChatTurn(ctx context.Context, q *dbsqlc.Queries, slot chatSlot, row dbsqlc.AgentRequest) error {
	if !slot.chat.Valid {
		return nil
	}
	chat := uuid.NullUUID{UUID: slot.chat.Bytes, Valid: true}
	target := audit.Target{Kind: "chat", ID: slot.chat.String(), DashboardID: nullUUID(slot.subject.dashboard), ObservationID: nullUUID(slot.subject.observation)}
	by := requesterColumns(row).AuditBy()
	if slot.created {
		if err := audit.Record(ctx, q, audit.Event{Action: actionChatCreate, Target: target, Chat: chat, By: by}); err != nil {
			return err
		}
	}
	sent := audit.MessageData(row.Prompt, string(anchor.KindOf(row.Anchor)))
	return audit.Record(ctx, q, audit.Event{Action: actionChatTurn, Target: target, Chat: chat, By: by, Data: sent})
}

func nullUUID(id pgtype.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id.Bytes, Valid: id.Valid}
}

func againChat(row dbsqlc.AgentRequest, by auth.Principal) chatChoice {
	switch {
	case !row.ChatID.Valid:
		return chatChoice{}
	case sameActor(requesterOf(row), by):
		return chatChoice{mode: sameChat, id: row.ChatID}
	case SourceOf(row) == SourcePrompt:
		return chatChoice{mode: dashboardChat}
	}
	return chatChoice{mode: newChat}
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

func answerOf(row dbsqlc.ChatTurnsBeforeRow) string {
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

func withChatTranscript(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) (dbsqlc.AgentRequest, error) {
	if !row.ChatID.Valid || row.ThreadMessageID.Valid || row.ThreadContext.Valid {
		return row, nil
	}
	rows, err := q.ChatTurnsBefore(ctx, dbsqlc.ChatTurnsBeforeParams{ChatID: row.ChatID, BeforeID: row.ID, RowLimit: maxChatTurns})
	if err != nil {
		return row, fmt.Errorf("read the earlier turns of the chat: %w", err)
	}
	turns := make([]chatTurn, 0, len(rows))
	ids := make([]pgtype.UUID, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		turns = append(turns, chatTurn{prompt: rows[i].Prompt, answer: answerOf(rows[i])})
		ids = append(ids, rows[i].ID)
	}
	transcript := chatTranscript(turns)
	if transcript == "" {
		return row, nil
	}
	if row, err = markIfTurnsKeptWithAdmins(ctx, q, row, ids); err != nil {
		return row, err
	}
	stored := pgtype.Text{String: transcript, Valid: true}
	if err := q.SetRequestThreadContext(ctx, dbsqlc.SetRequestThreadContextParams{ID: row.ID, ThreadContext: stored}); err != nil {
		return row, fmt.Errorf("store the chat transcript: %w", err)
	}
	row.ThreadContext = stored
	return row, nil
}

func markIfTurnsKeptWithAdmins(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest, turns []pgtype.UUID) (dbsqlc.AgentRequest, error) {
	if row.ReadRestricted {
		return row, nil
	}
	kept, err := q.TurnsKeptWithAdminsFrom(ctx, dbsqlc.TurnsKeptWithAdminsFromParams{ID: row.ID, TurnIds: turns})
	if err != nil {
		return row, fmt.Errorf("read whether an earlier turn's answer stays where only Admins read: %w", err)
	}
	if !kept {
		return row, nil
	}
	if err := q.MarkRunReadRestricted(ctx, row.ID); err != nil {
		return row, fmt.Errorf("mark a turn that carries an answer kept where only Admins read: %w", err)
	}
	row.ReadRestricted = true
	return row, nil
}
