package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	nameListInbox        = "list_inbox"
	nameSetInboxItemDone = "set_inbox_item_done"

	defaultListInbox = 50
	maxListInbox     = 100

	codeUserSessionRequired = "user_session_required"

	descListInbox        = "List what is asked of the person you work for, newest first: proposals to review, observation blocks waiting for them, mentions, replies in their threads, their agent runs that failed and health repairs of theirs that saved a version and their proposals that someone merged or discarded. Each item is an activity event as list_activity returns it, plus target_title (what it is about), excerpt (the start of the comment for a mention or reply, or the reason a proposal was discarded), question (on a reply an agent run wrote, the first line of the thread message it answered, and on a failed run the first line of the question it was asked, cut to a title of at most 60 characters, or null), the reason it needs them (proposal, requested_block, mention, reply, run_failed, repaired, proposal_closed; null in the everything view) and whether it is unread. Views: for_you (default, everything that needs them), proposals (review requests and closed proposals of theirs), mentions (mentions and replies) and everything (all activity by others). Filter: open (default, items not marked done), unread (open items that are unread) or done. unread_count counts unread for_you items and stops at 100. It works only for a signed-in person, never for a service-account key, and it does not mark anything read; set_inbox_item_done marks an item done. Page with next_cursor."
	descSetInboxItemDone = "Mark one inbox item done for the person you work for, or bring a done item back with done false; seq is the item's seq from list_inbox. A done item leaves the default list and the unread filter, counts as read, and stays done when the inbox is read. Only for a signed-in person, never for a service-account key."
)

var schemaListInbox = `{"type":"object","properties":{` +
	`"view":{"type":"string","enum":["` + strings.Join(inboxViewNames(), `","`) + `"],"description":"Which items to list. Default for_you."},` +
	`"filter":{"type":"string","enum":["` + strings.Join(inboxFilterNames(), `","`) + `"],"description":"open (default) lists items not marked done, unread lists the open items that are unread, done lists the items marked done."},` +
	`"limit":{"type":"integer","minimum":1,"maximum":` + strconv.Itoa(maxListInbox) + `,"description":"Items per page. Default ` + strconv.Itoa(defaultListInbox) + `."},` +
	`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

func inboxViewNames() []string {
	names := make([]string, len(activity.Views))
	for i, v := range activity.Views {
		names[i] = string(v)
	}
	return names
}

const schemaSetInboxItemDone = `{"type":"object","properties":{"seq":{"type":"integer","minimum":1,"description":"The seq of the item, from list_inbox."},"done":{"type":"boolean","description":"true marks it done (default), false brings it back."}},"required":["seq"],"additionalProperties":false}`

func inboxFilterNames() []string {
	names := make([]string, len(activity.Triages))
	for i, t := range activity.Triages {
		names[i] = string(t)
	}
	return names
}

func init() { registerEntries((*Registry).inboxEntries) }

func (r *Registry) inboxEntries() []entry {
	return []entry{
		{Tool{Name: nameListInbox, Description: descListInbox, InputSchema: json.RawMessage(schemaListInbox), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer, PersonOnly: true}, r.listInbox},
		{Tool{Name: nameSetInboxItemDone, Description: descSetInboxItemDone, InputSchema: json.RawMessage(schemaSetInboxItemDone), MCPOnly: true, MinRole: auth.RoleViewer, PersonOnly: true}, r.setInboxItemDone},
	}
}

type listInboxInput struct {
	View   string `json:"view"`
	Filter string `json:"filter"`
	Limit  *int   `json:"limit"`
	Cursor string `json:"cursor"`
}

func (r *Registry) listInbox(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Activity == nil {
		return Result{}, errActivityNotConfigured
	}
	var in listInboxInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultListInbox, 1, maxListInbox, "limit")
	if !ok {
		return res, nil
	}
	view := activity.ViewForYou
	if in.View != "" {
		parsed, ok := activity.ParseView(in.View)
		if !ok {
			return invalid("view must be one of %s", strings.Join(inboxViewNames(), ", ")), nil
		}
		view = parsed
	}
	triage := activity.TriageOpen
	if in.Filter != "" {
		parsed, ok := activity.ParseTriage(in.Filter)
		if !ok {
			return invalid("filter must be one of %s", strings.Join(inboxFilterNames(), ", ")), nil
		}
		triage = parsed
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		page, err := r.deps.Activity.Inbox(ctx, principal, view, triage, in.Cursor, limit)
		if err != nil {
			return inboxFailure(err)
		}
		out := activity.InboxJSONOf(page)
		empty := activity.InboxJSON{Items: []activity.InboxItemJSON{}, NextCursor: out.NextCursor, UnreadCount: out.UnreadCount}
		kept, cut := fit(out.Items, listBudget-jsonSize(empty))
		if !cut || len(out.Items) == 1 {
			return success("Listed "+plural(len(out.Items), "inbox item", "inbox items"), out), nil
		}
		limit = max(len(kept), 1)
	}
}

type inboxItemDoneOut struct {
	Seq  int64 `json:"seq"`
	Done bool  `json:"done"`
}

func (r *Registry) setInboxItemDone(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Activity == nil {
		return Result{}, errActivityNotConfigured
	}
	var in struct {
		Seq  int64 `json:"seq"`
		Done *bool `json:"done"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if in.Seq < 1 {
		return invalid("seq must be a positive integer from list_inbox"), nil
	}
	done := in.Done == nil || *in.Done
	principal, _ := auth.FromContext(ctx)
	if err := r.deps.Activity.SetItemDone(ctx, principal, in.Seq, done); err != nil {
		if errors.Is(err, activity.ErrUnknownSeq) {
			return toolError(codeNotFound, "no activity has that seq; use a seq from list_inbox"), nil
		}
		return inboxFailure(err)
	}
	step := "Marked inbox item " + strconv.FormatInt(in.Seq, 10) + " done"
	if !done {
		step = "Brought inbox item " + strconv.FormatInt(in.Seq, 10) + " back"
	}
	return success(step, inboxItemDoneOut{Seq: in.Seq, Done: done}), nil
}

func inboxFailure(err error) (Result, error) {
	switch {
	case errors.Is(err, activity.ErrInvalidCursor):
		return invalid("cursor is not valid"), nil
	case errors.Is(err, activity.ErrPersonOnly):
		return toolError(codeUserSessionRequired, "the inbox belongs to a signed-in person; service-account keys have none"), nil
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	}
	return Result{}, err
}
