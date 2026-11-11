package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	nameListActivity = "list_activity"

	defaultListActivity = 50
	maxListActivity     = 100

	descListActivity = "List what changed in the workspace, newest first: dashboard saves, restores, moves and deletes, annotations, findings, comment threads (opened, replied, edited, resolved, reopened, mentioned) and agent runs, each with the dashboard, the folder and the actor, whose label says who or what made the change. It lists only activity in folders this key may see, optionally narrowed to one dashboard or folder, to some types, or to events since a time. The data of an event holds ids, numbers, titles and a version summary, never text; read the object for more. Page with next_cursor."
)

var schemaListActivity = `{"type":"object","properties":{` +
	`"dashboard_id":{"type":"string","format":"uuid","description":"Only this dashboard's activity, from list_dashboards."},` +
	`"folder_id":{"type":"string","format":"uuid","description":"Only activity in this folder, from list_folders."},` +
	`"types":{"type":"array","maxItems":` + strconv.Itoa(len(activity.Types())) + `,"items":{"type":"string","enum":["` + strings.Join(activity.Types(), `","`) + `"]},"description":"Only events of any of these types."},` +
	`"since":{"type":"string","format":"date-time","description":"Only events at or after this RFC 3339 time."},` +
	`"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Events per page. Default 50."},` +
	`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

var errActivityNotConfigured = errors.New("activity is not configured")

func init() { registerEntries((*Registry).activityEntries) }

func (r *Registry) activityEntries() []entry {
	return []entry{
		{Tool{Name: nameListActivity, Description: descListActivity, InputSchema: json.RawMessage(schemaListActivity), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.listActivity},
	}
}

type listActivityInput struct {
	DashboardID string   `json:"dashboard_id"`
	FolderID    string   `json:"folder_id"`
	Types       []string `json:"types"`
	Since       string   `json:"since"`
	Limit       *int     `json:"limit"`
	Cursor      string   `json:"cursor"`
}

type activityListOut struct {
	Activity   []activity.EventJSON `json:"activity"`
	NextCursor *string              `json:"next_cursor"`
}

func (r *Registry) listActivity(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Activity == nil {
		return Result{}, errActivityNotConfigured
	}
	var in listActivityInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultListActivity, 1, maxListActivity, "limit")
	if !ok {
		return res, nil
	}
	filter, err := activity.ParseFilter(activityValues(in))
	if err != nil {
		return invalid("%s", err.Error()), nil
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	audience, err := r.deps.Activity.Audience(ctx, principal)
	if err != nil {
		return activityFailure(err)
	}
	for {
		page, next, err := r.deps.Activity.List(ctx, audience, filter, in.Cursor, limit)
		if err != nil {
			return activityFailure(err)
		}
		out := activityListOut{Activity: make([]activity.EventJSON, 0, len(page))}
		for _, e := range page {
			out.Activity = append(out.Activity, activity.JSONOf(e))
		}
		if next != "" {
			out.NextCursor = &next
		}
		kept, cut := fit(out.Activity, listBudget-jsonSize(activityListOut{Activity: []activity.EventJSON{}, NextCursor: out.NextCursor}))
		if !cut || len(out.Activity) == 1 {
			return success("Listed "+plural(len(out.Activity), "event", "events"), out), nil
		}
		limit = max(len(kept), 1)
	}
}

func activityValues(in listActivityInput) url.Values {
	v := url.Values{}
	for key, value := range map[string]string{"dashboard_id": in.DashboardID, "folder_id": in.FolderID, "since": in.Since} {
		if value != "" {
			v.Set(key, value)
		}
	}
	v["type"] = in.Types
	return v
}

func activityFailure(err error) (Result, error) {
	switch {
	case errors.Is(err, activity.ErrInvalidCursor):
		return invalid("cursor is not valid"), nil
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	}
	return Result{}, err
}
