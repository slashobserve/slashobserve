package revisions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	defaultChatLimit = 20
	maxChatLimit     = 50
)

type chatTurnJSON struct {
	ID                   string         `json:"id"`
	State                State          `json:"state"`
	Prompt               string         `json:"prompt"`
	Investigation        bool           `json:"investigation"`
	SubjectObservationID *string        `json:"subject_observation_id"`
	Anchor               *anchor.Anchor `json:"anchor"`
	BlockID              *string        `json:"block_id"`
}

type chatJSON struct {
	ID             string        `json:"id"`
	Title          string        `json:"title"`
	DashboardID    *string       `json:"dashboard_id"`
	DashboardTitle *string       `json:"dashboard_title"`
	ObservationID  *string       `json:"observation_id"`
	PinnedAt       *string       `json:"pinned_at"`
	CreatedAt      string        `json:"created_at"`
	LastTurnAt     string        `json:"last_turn_at"`
	Turns          int           `json:"turns"`
	LastTurn       *chatTurnJSON `json:"last_turn"`
	Owner          chatOwnerJSON `json:"owner"`
	ReadOnly       bool          `json:"read_only"`
}

type chatOwnerJSON struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type chatListJSON struct {
	Chats      []chatJSON `json:"chats"`
	NextCursor *string    `json:"next_cursor"`
}

type chatDetailJSON struct {
	Chat  chatJSON           `json:"chat"`
	Turns []agentRequestJSON `json:"turns"`
}

func newChatJSON(c Chat) chatJSON {
	out := chatJSON{
		ID:             c.ID.String(),
		Title:          c.Title,
		DashboardID:    optionalUUID(c.DashboardID),
		DashboardTitle: c.DashboardTitle,
		ObservationID:  optionalUUID(c.ObservationID),
		PinnedAt:       httpapi.FormatOptionalTime(c.PinnedAt),
		CreatedAt:      httpapi.FormatTime(c.CreatedAt),
		LastTurnAt:     httpapi.FormatTime(c.LastTurnAt),
		Turns:          c.Turns,
		Owner:          chatOwnerJSON{Kind: c.Owner.Kind, Name: c.Owner.Name},
		ReadOnly:       c.ReadOnly,
	}
	if c.LastTurn != nil {
		out.LastTurn = &chatTurnJSON{ID: c.LastTurn.ID.String(), State: c.LastTurn.State, Prompt: c.LastTurn.Prompt, Investigation: c.LastTurn.Investigation, SubjectObservationID: optionalUUID(c.LastTurn.Subject), Anchor: c.LastTurn.Anchor, BlockID: c.LastTurn.BlockID}
	}
	return out
}

func (r *RunDesk) shownChat(ctx context.Context, caller auth.Principal, c Chat) (chatJSON, error) {
	out := newChatJSON(c)
	var err error
	if out.ObservationID, err = r.shownObservation(ctx, caller, out.ObservationID, c.ObservationID); err != nil {
		return chatJSON{}, err
	}
	if out.LastTurn != nil {
		if out.LastTurn.SubjectObservationID, err = r.shownObservation(ctx, caller, out.LastTurn.SubjectObservationID, c.LastTurn.Subject); err != nil {
			return chatJSON{}, err
		}
	}
	return out, nil
}

func (r *RunDesk) Chats(ctx context.Context, caller auth.Principal, filter ChatFilter, limit int, cursor string) (json.RawMessage, error) {
	chats, next, err := r.service.Chats(ctx, caller, filter, limit, cursor)
	if err != nil {
		return nil, err
	}
	body := chatListJSON{Chats: make([]chatJSON, len(chats))}
	for i, c := range chats {
		if body.Chats[i], err = r.shownChat(ctx, caller, c); err != nil {
			return nil, err
		}
	}
	if next != "" {
		body.NextCursor = &next
	}
	return marshalChat(body)
}

func (r *RunDesk) Chat(ctx context.Context, caller auth.Principal, id pgtype.UUID) (json.RawMessage, error) {
	chat, turns, err := r.service.Chat(ctx, caller, id)
	if err != nil {
		return nil, err
	}
	shown, err := r.shownChat(ctx, caller, chat)
	if err != nil {
		return nil, err
	}
	body := chatDetailJSON{Chat: shown, Turns: make([]agentRequestJSON, len(turns))}
	for i, turn := range turns {
		if body.Turns[i], err = r.render(ctx, turn, caller, chat.ReadOnly); err != nil {
			return nil, err
		}
	}
	return marshalChat(body)
}

func (r *RunDesk) ArchiveChat(ctx context.Context, caller auth.Principal, id pgtype.UUID) error {
	return r.service.ArchiveChat(ctx, caller, id)
}

func marshalChat(body any) (json.RawMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode chat: %w", err)
	}
	return raw, nil
}

func (h *ObserveHandler) listChats(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultChatLimit, maxChatLimit)
	if !ok {
		return
	}
	filter, ok := chatFilter(c)
	if !ok {
		return
	}
	raw, err := h.runs.Chats(c.Request.Context(), httpapi.Caller(c), filter, limit, c.Query("cursor"))
	if err != nil {
		h.service.fail(c, "list chats", err, principalAttrs(c)...)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func chatFilter(c *gin.Context) (ChatFilter, bool) {
	var filter ChatFilter
	if raw, present := c.GetQuery("scope"); present {
		switch ListScope(raw) {
		case ScopeMine, ScopeAll:
			filter.Everyone = ListScope(raw) == ScopeAll
		default:
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "scope must be mine or all")
			return filter, false
		}
	}
	if raw, present := c.GetQuery("q"); present {
		q, err := CheckQuery(raw)
		if err != nil {
			httpapi.AbortText(c, http.StatusBadRequest, "invalid_request", err)
			return filter, false
		}
		filter.Query = q
	}
	if raw, present := c.GetQuery("pinned"); present {
		switch raw {
		case "true", "false":
			pinned := raw == "true"
			filter.Pinned = &pinned
		default:
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "pinned must be true or false")
			return filter, false
		}
	}
	for name, into := range map[string]*pgtype.UUID{"dashboard_id": &filter.DashboardID, "observation_id": &filter.ObservationID} {
		raw, present := c.GetQuery(name)
		if !present {
			continue
		}
		id, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be a UUID")
			return filter, false
		}
		*into = id
	}
	return filter, true
}

func (h *ObserveHandler) getChat(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	raw, err := h.runs.Chat(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.service.fail(c, "get chat", err, slog.String("chat_id", id.String()))
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *ObserveHandler) archiveChat(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.runs.ArchiveChat(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		h.service.fail(c, "archive chat", err, slog.String("chat_id", id.String()))
		return
	}
	c.Status(http.StatusNoContent)
}
