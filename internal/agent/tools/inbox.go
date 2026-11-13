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
	nameListInbox = "list_inbox"

	defaultListInbox = 50
	maxListInbox     = 100

	codeUserSessionRequired = "user_session_required"

	descListInbox = "List what is asked of the person you work for, newest first: proposals to review, observation blocks waiting for them, mentions, replies in their threads, their agent runs that failed and health repairs of theirs that saved a version. Each item is an activity event as list_activity returns it, plus the reason it needs them (proposal, requested_block, mention, reply, run_failed, repaired; null in the everything view) and whether it is unread. Views: for_you (default, everything that needs them), proposals, mentions (mentions and replies) and everything (all activity by others). unread_count counts unread for_you items and stops at 100. It works only for a signed-in person, never for a service-account key, and it does not mark anything read. Page with next_cursor."
)

var schemaListInbox = `{"type":"object","properties":{` +
	`"view":{"type":"string","enum":["` + strings.Join(inboxViewNames(), `","`) + `"],"description":"Which items to list. Default for_you."},` +
	`"limit":{"type":"integer","minimum":1,"maximum":` + strconv.Itoa(maxListInbox) + `,"description":"Items per page. Default ` + strconv.Itoa(defaultListInbox) + `."},` +
	`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

func inboxViewNames() []string {
	names := make([]string, len(activity.Views))
	for i, v := range activity.Views {
		names[i] = string(v)
	}
	return names
}

func init() { registerEntries((*Registry).inboxEntries) }

func (r *Registry) inboxEntries() []entry {
	return []entry{
		{Tool{Name: nameListInbox, Description: descListInbox, InputSchema: json.RawMessage(schemaListInbox), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.listInbox},
	}
}

type listInboxInput struct {
	View   string `json:"view"`
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
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		page, err := r.deps.Activity.Inbox(ctx, principal, view, in.Cursor, limit)
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
