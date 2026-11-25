package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	nameListActivity = "list_activity"

	defaultListActivity = 50
	maxListActivity     = 100

	descListActivity = "List what changed in the workspace, newest first: dashboard saves, restores, moves and deletes, annotations, findings, public links, comment access requests, comment threads and agent runs, each with the dashboard, the folder and the actor, whose label says who or what made the change. It lists only activity in folders this key may see, optionally narrowed by dashboard, folder, observation, types or time. The data of an event holds ids, numbers, titles and a version summary, never text; read the object for more. Page with next_cursor. An Admin key also sees admin changes (roles, people, keys, webhooks, connected agents, outbound domains, Grafana connections, datasources)."
)

var schemaListActivity = `{"type":"object","properties":{` +
	`"dashboard_id":{"type":"string","format":"uuid","description":"Only this dashboard's activity, from list_dashboards."},` +
	`"folder_id":{"type":"string","format":"uuid","description":"Only activity in this folder, from list_folders."},` +
	`"observation_id":{"type":"string","format":"uuid","description":"Only this observation's activity, from list_observations."},` +
	`"types":{"type":"array","maxItems":` + strconv.Itoa(len(activity.Types())) + `,"items":{"type":"string","enum":["` + strings.Join(activity.Types(), `","`) + `"]},"description":"Only events of any of these types."},` +
	`"since":{"type":"string","format":"date-time","description":"Only events at or after this RFC 3339 time."},` +
	`"until":{"type":"string","format":"date-time","description":"Only events at or before this RFC 3339 time."},` +
	`"actor_id":{"type":"string","format":"uuid","description":"Only events made by this person or service account."},` +
	`"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Events per page. Default 50."},` +
	`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

var errActivityNotConfigured = errors.New("activity is not configured")

func init() { registerEntries((*Registry).activityEntries) }

func (r *Registry) activityEntries() []entry {
	return []entry{
		{Tool{Name: nameListActivity, Description: descListActivity, InputSchema: json.RawMessage(schemaListActivity), ReadOnly: true, MinRole: auth.RoleViewer}, r.listActivity},
	}
}

type listActivityInput struct {
	DashboardID   string   `json:"dashboard_id"`
	FolderID      string   `json:"folder_id"`
	ObservationID string   `json:"observation_id"`
	Types         []string `json:"types"`
	Since         string   `json:"since"`
	Until         string   `json:"until"`
	ActorID       string   `json:"actor_id"`
	Limit         *int     `json:"limit"`
	Cursor        string   `json:"cursor"`
}

type activityListOut struct {
	Activity   []activity.EventJSON `json:"activity"`
	NextCursor *string              `json:"next_cursor"`
	leftOutOut
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
	audience, err := r.activityAudience(ctx, principal)
	if err != nil {
		return activityFailure(err)
	}
	titles, res, ok, err := r.restrictedTitles(ctx, false)
	if err != nil || !ok {
		return res, err
	}
	for {
		page, next, err := r.deps.Activity.List(ctx, audience, filter, in.Cursor, limit)
		if err != nil {
			return activityFailure(err)
		}
		events := keepActivity(titles, activityEvents(page))
		out := activityListOut{Activity: events, leftOutOut: titles.report("event", "events")}
		if next != "" {
			out.NextCursor = &next
		}
		kept, cut := fit(out.Activity, listBudget-jsonSize(activityListOut{Activity: []activity.EventJSON{}, NextCursor: out.NextCursor}))
		if !cut || len(out.Activity) == 1 {
			titles.done(ctx)
			return success("Listed "+plural(len(out.Activity), "event", "events"), out), nil
		}
		limit = max(len(kept), 1)
	}
}

func (r *Registry) activityAudience(ctx context.Context, principal auth.Principal) (folders.Audience, error) {
	audience, err := r.deps.Activity.Audience(ctx, principal)
	if a, ok := datasource.AccessFrom(ctx); ok && !a.Reads(datasource.AudienceAdmins) {
		audience.Admin = false
	}
	return audience, err
}

func activityEvents(page []activity.Event) []activity.EventJSON {
	events := make([]activity.EventJSON, 0, len(page))
	for _, e := range page {
		events = append(events, activity.JSONOf(e))
	}
	return events
}

func keepActivity(titles *restrictedTitles, events []activity.EventJSON) []activity.EventJSON {
	titles.restart()
	return slices.DeleteFunc(events, func(e activity.EventJSON) bool {
		if activity.IsAdminType(e.Type) {
			return !titles.keepAdminsOnly()
		}
		folderID := ""
		if e.FolderID != nil {
			folderID = *e.FolderID
		}
		return !titles.keepIn(folderID, activitySubject(e))
	})
}

func activitySubject(e activity.EventJSON) string {
	if e.DashboardID != nil {
		return *e.DashboardID
	}
	return e.Target.ID
}

func activityValues(in listActivityInput) url.Values {
	v := url.Values{}
	for key, value := range map[string]string{"dashboard_id": in.DashboardID, "folder_id": in.FolderID, "observation_id": in.ObservationID, "since": in.Since, "until": in.Until, "actor_id": in.ActorID} {
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
