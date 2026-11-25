package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	nameListDashboardViewers = "list_dashboard_viewers"

	descListDashboardViewers = "List the people who have a dashboard open in the web app right now, by name, other than you. A person counts while their open page has reported in during the last 45 seconds, so the list is empty when nobody is looking. Use it before you change a dashboard people are watching. You need to be able to view the dashboard."

	schemaListDashboardViewers = `{"type":"object","properties":{"dashboard_id":{"type":"string","format":"uuid","description":"Dashboard id from list_dashboards or a dashboard URL."}},"required":["dashboard_id"],"additionalProperties":false}`
)

var errViewersNotConfigured = errors.New("dashboard viewers are not configured")

func init() { registerEntries((*Registry).viewerEntries) }

func (r *Registry) viewerEntries() []entry {
	return []entry{
		{Tool{Name: nameListDashboardViewers, Description: descListDashboardViewers, InputSchema: json.RawMessage(schemaListDashboardViewers), ReadOnly: true, MinRole: auth.RoleViewer}, r.listDashboardViewers},
	}
}

type viewerOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type viewerListOut struct {
	Viewers []viewerOut `json:"viewers"`
}

func (r *Registry) listDashboardViewers(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Viewers == nil {
		return Result{}, errViewersNotConfigured
	}
	var in struct {
		DashboardID string `json:"dashboard_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, err := db.ParseUUID(in.DashboardID)
	if err != nil {
		return invalid("dashboard_id must be a UUID from list_dashboards"), nil
	}
	if res, ok, err := r.viewableDashboard(ctx, id, "dashboard not found"); err != nil || !ok {
		return res, err
	}
	principal, _ := auth.FromContext(ctx)
	others, err := r.deps.Viewers.Others(ctx, principal, id)
	if err != nil {
		return Result{}, err
	}
	out := viewerListOut{Viewers: make([]viewerOut, len(others))}
	for i, v := range others {
		out.Viewers[i] = viewerOut(v)
	}
	return success("Listed "+plural(len(out.Viewers), "viewer", "viewers"), out), nil
}
