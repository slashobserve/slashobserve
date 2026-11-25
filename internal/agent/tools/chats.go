package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	nameListChats = "list_chats"
	nameGetChat   = "get_chat"

	defaultListChats = 20
	maxListChats     = 50

	descListChats = "List your chats with the built-in agent, pinned ones first and then the one most recently asked in, 20 per page by default; scope all orders them by last turn only. A chat is the run of requests one person sent about one thing; its owner and Admins read it, nobody else. scope all lists every person's chats, Admin only. Keep chats about a dashboard_id or an observation_id, pinned or unpinned ones, or those whose title or any prompt contains q. Each has its id, title, owner, read_only, dashboard_id, dashboard_title (null when you cannot see that dashboard), observation_id, pinned_at, created_at, last_turn_at, how many turns it has and its last turn. Read one with get_chat, and send its id as chat_id to start_run to ask the next turn. Page with next_cursor."
	descGetChat   = "Read one of your chats, or as an Admin anyone's read only, with its turns, oldest first, each the run it started as get_run returns it without steps. Another person's chat for a non-Admin, an archived chat or an unknown id answers not_found. When the turns do not all fit, truncated is true and next_offset reads the rest."

	schemaListChats = `{"type":"object","properties":{` +
		`"scope":{"type":"string","enum":["mine","all"],"description":"mine, the default, or all for every person's chats, which needs the Admin role."},` +
		`"dashboard_id":{"type":"string","format":"uuid","description":"Only chats about this dashboard, from list_dashboards."},` +
		`"observation_id":{"type":"string","format":"uuid","description":"Only chats about this observation."},` +
		`"pinned":{"type":"boolean","description":"true keeps only pinned chats, false only unpinned ones."},` +
		`"q":{"type":"string","minLength":1,"maxLength":200,"description":"Only chats whose title or any prompt contains this text, ignoring case."},` +
		`"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Chats per page. Default 20."},` +
		`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`
	schemaGetChat = `{"type":"object","properties":{` +
		`"id":{"type":"string","format":"uuid","description":"The chat id, from list_chats or the chat_id of a run."},` +
		`"offset":{"type":"integer","minimum":0,"description":"How many turns to skip, the next_offset of the previous call. Default 0."}},"required":["id"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).chatEntries) }

func (r *Registry) chatEntries() []entry {
	return []entry{
		{Tool{Name: nameListChats, Description: descListChats, InputSchema: json.RawMessage(schemaListChats), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.listChats},
		{Tool{Name: nameGetChat, Description: descGetChat, InputSchema: json.RawMessage(schemaGetChat), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.getChat},
	}
}

type listChatsInput struct {
	Scope         string  `json:"scope"`
	DashboardID   string  `json:"dashboard_id"`
	ObservationID string  `json:"observation_id"`
	Pinned        *bool   `json:"pinned"`
	Q             *string `json:"q"`
	Limit         *int    `json:"limit"`
	Cursor        string  `json:"cursor"`
}

func (in listChatsInput) parse() (revisions.ChatFilter, int, Result, bool) {
	filter := revisions.ChatFilter{Pinned: in.Pinned, Everyone: revisions.ListScope(in.Scope) == revisions.ScopeAll}
	switch revisions.ListScope(in.Scope) {
	case "", revisions.ScopeMine, revisions.ScopeAll:
	default:
		return filter, 0, invalid("scope must be mine or all"), false
	}
	if in.Q != nil {
		q, err := revisions.CheckQuery(*in.Q)
		if err != nil {
			return filter, 0, invalid("%s", err.Error()), false
		}
		filter.Query = q
	}
	if in.DashboardID != "" {
		id, res, ok := parseDashboardID(in.DashboardID)
		if !ok {
			return filter, 0, res, false
		}
		filter.DashboardID = id
	}
	if in.ObservationID != "" {
		id, err := db.ParseUUID(in.ObservationID)
		if err != nil {
			return filter, 0, invalid("observation_id must be a UUID"), false
		}
		filter.ObservationID = id
	}
	limit, res, ok := boundedInt(in.Limit, defaultListChats, 1, maxListChats, "limit")
	if !ok {
		return filter, 0, res, false
	}
	res, ok = checkLength("cursor", in.Cursor, maxNameBytes)
	return filter, limit, res, ok
}

type chatListOut struct {
	Chats      []json.RawMessage `json:"chats"`
	NextCursor *string           `json:"next_cursor"`
}

func (r *Registry) listChats(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	var in listChatsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	filter, limit, res, ok := in.parse()
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		raw, err := r.deps.Runs.Chats(ctx, principal, filter, limit, in.Cursor)
		if errors.Is(err, revisions.ErrInvalidCursor) {
			return invalid("cursor is not valid"), nil
		}
		if err != nil {
			return runFailure(ctx, err)
		}
		var out chatListOut
		if err := json.Unmarshal(raw, &out); err != nil {
			return Result{}, fmt.Errorf("decode chats: %w", err)
		}
		if out.Chats == nil {
			out.Chats = []json.RawMessage{}
		}
		kept, cut := fit(out.Chats, listBudget-jsonSize(chatListOut{Chats: []json.RawMessage{}, NextCursor: out.NextCursor}))
		if !cut || len(out.Chats) == 1 {
			return success("Listed "+plural(len(out.Chats), "chat", "chats"), out), nil
		}
		limit = max(len(kept), 1)
	}
}

type getChatInput struct {
	ID     string `json:"id"`
	Offset *int   `json:"offset"`
}

type chatOut struct {
	Chat  json.RawMessage   `json:"chat"`
	Turns []json.RawMessage `json:"turns"`
	truncation
}

func (r *Registry) getChat(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	var in getChatInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, err := db.ParseUUID(in.ID)
	if err != nil {
		return invalid("id must be a UUID"), nil
	}
	offset := 0
	if in.Offset != nil {
		if *in.Offset < 0 {
			return invalid("offset must be 0 or more"), nil
		}
		offset = *in.Offset
	}
	principal, _ := auth.FromContext(ctx)
	raw, err := r.deps.Runs.Chat(ctx, principal, id)
	if err != nil {
		r.recordRefusedRead(ctx, actionChatView, "chat", id.String(), err)
		return runFailure(ctx, err)
	}
	var out chatOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return Result{}, fmt.Errorf("decode chat: %w", err)
	}
	if out.Turns == nil {
		out.Turns = []json.RawMessage{}
	}
	rest := out.Turns[min(offset, len(out.Turns)):]
	kept, _ := fit(rest, listBudget-len(out.Chat))
	if len(kept) == 0 && len(rest) > 0 {
		kept = rest[:1]
	}
	out.Turns = kept
	if len(kept) < len(rest) {
		next := offset + len(kept)
		out.truncation = truncation{Truncated: true, TruncatedBy: truncatedByBytes, NextOffset: &next}
	}
	return success("Read chat "+in.ID, out), nil
}
