package tools

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/homedash"
)

const (
	nameGetHomeDashboard   = "get_home_dashboard"
	nameSetHomeDashboard   = "set_home_dashboard"
	nameClearHomeDashboard = "clear_home_dashboard"

	descGetHomeDashboard   = "Get the home dashboard of the person you work for, the dashboard that opens for them after sign-in in place of Home. status is none (not set), ok (dashboard filled in), gone (it was deleted) or unreadable (they can no longer see it); in the last two the web app ignores it and tells them. Only for a signed-in person, never a service-account key."
	descSetHomeDashboard   = "Make a dashboard the home dashboard of the person you work for; it replaces the one they had, and setting the same dashboard again changes nothing. They need to be able to view it, otherwise not_found. Only for a signed-in person, never a service-account key."
	descClearHomeDashboard = "Clear the home dashboard of the person you work for so Home opens after sign-in again. Clearing when none is set changes nothing."

	schemaSetHomeDashboard = `{"type":"object","properties":{` + propVersionDashboardID + `},"required":["dashboard_id"],"additionalProperties":false}`
)

var errHomeNotConfigured = errors.New("home dashboards are not configured")

func init() { registerEntries((*Registry).homeDashboardEntries) }

func (r *Registry) homeDashboardEntries() []entry {
	return []entry{
		{Tool{Name: nameGetHomeDashboard, Description: descGetHomeDashboard, InputSchema: json.RawMessage(schemaEmpty), ReadOnly: true, MinRole: auth.RoleViewer}, r.getHomeDashboard},
		{Tool{Name: nameSetHomeDashboard, Description: descSetHomeDashboard, InputSchema: json.RawMessage(schemaSetHomeDashboard), MinRole: auth.RoleViewer}, r.setHomeDashboard},
		{Tool{Name: nameClearHomeDashboard, Description: descClearHomeDashboard, InputSchema: json.RawMessage(schemaEmpty), MinRole: auth.RoleViewer}, r.clearHomeDashboard},
	}
}

type homeDashboardOut struct {
	Status    homedash.Status      `json:"status"`
	Dashboard *homeDashboardRefOut `json:"dashboard"`
	SetAt     *string              `json:"set_at"`
}

type homeDashboardRefOut struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	FolderID    string `json:"folder_id"`
	FolderTitle string `json:"folder_title"`
}

func homeOut(h homedash.Home) homeDashboardOut {
	out := homeDashboardOut{Status: h.Status}
	if h.Status == homedash.StatusNone {
		return out
	}
	setAt := h.SetAt.UTC().Format(time.RFC3339)
	out.SetAt = &setAt
	if h.Status == homedash.StatusOK {
		out.Dashboard = &homeDashboardRefOut{ID: h.DashboardID, Title: h.Title, FolderID: h.FolderID, FolderTitle: h.FolderTitle}
	}
	return out
}

func (r *Registry) homeOwner(ctx context.Context) (auth.Principal, Result, bool, error) {
	if r.deps.Home == nil {
		return auth.Principal{}, Result{}, false, errHomeNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	if principal.Kind != auth.KindUser {
		return auth.Principal{}, toolError(codeUserSessionRequired, "a home dashboard belongs to a signed-in person; service-account keys have none"), false, nil
	}
	return principal, Result{}, true, nil
}

func homeSummary(h homedash.Home) string {
	switch h.Status {
	case homedash.StatusOK:
		return "Home dashboard is " + h.DashboardID
	case homedash.StatusNone:
		return "No home dashboard is set"
	default:
		return "The home dashboard " + h.DashboardID + " is " + string(h.Status)
	}
}

func (r *Registry) getHomeDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	principal, res, ok, err := r.homeOwner(ctx)
	if err != nil || !ok {
		return res, err
	}
	titles, res, ok, err := r.restrictedTitles(ctx, false)
	if err != nil || !ok {
		return res, err
	}
	home, err := r.deps.Home.Get(ctx, principal)
	if err != nil {
		return Result{}, err
	}
	if home.Status == homedash.StatusOK {
		titles.holds(ctx, home.FolderID)
	}
	return success(homeSummary(home), homeOut(home)), nil
}

func (r *Registry) setHomeDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
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
	principal, res, ok, err := r.homeOwner(ctx)
	if err != nil || !ok {
		return res, err
	}
	switch err := r.readDashboardContent(ctx, principal, id); {
	case errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrNotFound):
		return toolError(codeNotFound, "dashboard not found"), nil
	case err != nil:
		return Result{}, err
	}
	switch err := r.deps.Home.Set(ctx, principal, id); {
	case errors.Is(err, homedash.ErrNotFound):
		return toolError(codeNotFound, "dashboard not found"), nil
	case err != nil:
		return Result{}, err
	}
	home, err := r.deps.Home.Get(ctx, principal)
	if err != nil {
		return Result{}, err
	}
	return success("Set the home dashboard to "+id.String(), homeOut(home)), nil
}

func (r *Registry) clearHomeDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	principal, res, ok, err := r.homeOwner(ctx)
	if err != nil || !ok {
		return res, err
	}
	if err := r.deps.Home.Clear(ctx, principal); err != nil {
		return Result{}, err
	}
	return success("Cleared the home dashboard", homeOut(homedash.Home{Status: homedash.StatusNone})), nil
}
