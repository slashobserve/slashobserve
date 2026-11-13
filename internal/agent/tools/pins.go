package tools

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/stars"
)

const (
	namePinDashboard         = "pin_dashboard"
	nameUnpinDashboard       = "unpin_dashboard"
	nameListPinnedDashboards = "list_pinned_dashboards"

	descPinDashboard         = "Pin a dashboard for the person you work for; pinned dashboards show in their sidebar. Only for a signed-in person, never a service-account key."
	descUnpinDashboard       = "Unpin a dashboard for the person you work for."
	descListPinnedDashboards = "List the dashboards the person you work for has pinned, most recently pinned first."

	schemaPinDashboard = `{"type":"object","properties":{` + propVersionDashboardID + `},"required":["dashboard_id"],"additionalProperties":false}`
)

var errStarsNotConfigured = errors.New("pins are not configured")

func init() { registerEntries((*Registry).pinEntries) }

func (r *Registry) pinEntries() []entry {
	return []entry{
		{Tool{Name: namePinDashboard, Description: descPinDashboard, InputSchema: json.RawMessage(schemaPinDashboard), MCPOnly: true, MinRole: auth.RoleViewer}, r.pinDashboard},
		{Tool{Name: nameUnpinDashboard, Description: descUnpinDashboard, InputSchema: json.RawMessage(schemaPinDashboard), MCPOnly: true, MinRole: auth.RoleViewer}, r.unpinDashboard},
		{Tool{Name: nameListPinnedDashboards, Description: descListPinnedDashboards, InputSchema: json.RawMessage(schemaEmpty), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.listPinnedDashboards},
	}
}

type pinnedOut struct {
	DashboardID string `json:"dashboard_id"`
	Pinned      bool   `json:"pinned"`
}

type pinnedItemOut struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	FolderID    string `json:"folder_id"`
	FolderTitle string `json:"folder_title"`
	PinnedAt    string `json:"pinned_at"`
}

type pinnedListOut struct {
	Dashboards []pinnedItemOut `json:"dashboards"`
	Truncated  bool            `json:"truncated,omitempty"`
}

func (r *Registry) pinner(ctx context.Context) (auth.Principal, Result, bool, error) {
	if r.deps.Stars == nil {
		return auth.Principal{}, Result{}, false, errStarsNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	if principal.Kind != auth.KindUser {
		return auth.Principal{}, toolError(codeUserSessionRequired, "pins belong to a signed-in person; service-account keys have none"), false, nil
	}
	return principal, Result{}, true, nil
}

func (r *Registry) setPin(ctx context.Context, input json.RawMessage, pinned bool) (Result, error) {
	var in struct {
		DashboardID string `json:"dashboard_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return res, nil
	}
	principal, res, ok, err := r.pinner(ctx)
	if err != nil || !ok {
		return res, err
	}
	if res, viewable, err := r.viewableDashboard(ctx, id, "dashboard not found"); err != nil || !viewable {
		return res, err
	}
	step := "Unpinned dashboard "
	call := r.deps.Stars.Unstar
	if pinned {
		step, call = "Pinned dashboard ", r.deps.Stars.Star
	}
	switch err := call(ctx, principal, id); {
	case errors.Is(err, stars.ErrNotFound):
		return toolError(codeNotFound, "dashboard not found"), nil
	case err != nil:
		return Result{}, err
	}
	return success(step+id.String(), pinnedOut{DashboardID: id.String(), Pinned: pinned}), nil
}

func (r *Registry) pinDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	return r.setPin(ctx, input, true)
}

func (r *Registry) unpinDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	return r.setPin(ctx, input, false)
}

func (r *Registry) listPinnedDashboards(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	principal, res, ok, err := r.pinner(ctx)
	if err != nil || !ok {
		return res, err
	}
	list, err := r.deps.Stars.List(ctx, principal)
	if err != nil {
		return Result{}, err
	}
	items := make([]pinnedItemOut, len(list))
	for i, s := range list {
		items[i] = pinnedItemOut{ID: s.DashboardID, Title: s.Title, FolderID: s.FolderID, FolderTitle: s.FolderTitle, PinnedAt: s.StarredAt.UTC().Format(time.RFC3339)}
	}
	kept, cut := fit(items, listBudget-jsonSize(pinnedListOut{Dashboards: []pinnedItemOut{}, Truncated: true}))
	return success("Listed "+plural(len(kept), "pinned dashboard", "pinned dashboards"), pinnedListOut{Dashboards: kept, Truncated: cut}), nil
}
