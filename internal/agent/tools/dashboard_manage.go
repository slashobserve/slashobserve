package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grantmode"
)

const (
	nameMoveDashboard      = "move_dashboard"
	nameDuplicateDashboard = "duplicate_dashboard"

	descMoveDashboard      = "Move a dashboard to another folder from list_folders. Needs edit access to both folders. Its versions, comments and links stay with it."
	descDuplicateDashboard = "Copy the latest version of a dashboard into a new dashboard with its own title, in folder_id or your default folder. Returns the new dashboard's id and url."

	schemaMoveDashboard = `{"type":"object","properties":{` + propVersionDashboardID + `,` +
		`"folder_id":{"type":"string","format":"uuid","description":"The target folder, from list_folders."}},"required":["dashboard_id","folder_id"],"additionalProperties":false}`

	schemaDuplicateDashboard = `{"type":"object","properties":{` + propVersionDashboardID + `,` +
		`"title":{"type":"string","minLength":1,"maxLength":200,"description":"The title of the copy."},` +
		`"folder_id":{"type":"string","format":"uuid","description":"The folder for the copy, from list_folders. Default your My Dashboards, else General."}},"required":["dashboard_id","title"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).dashboardManageEntries) }

func (r *Registry) dashboardManageEntries() []entry {
	return []entry{
		{Tool{Name: nameMoveDashboard, Description: descMoveDashboard, InputSchema: json.RawMessage(schemaMoveDashboard), MCPOnly: true, MinRole: auth.RoleViewer}, r.moveDashboard},
		{Tool{Name: nameDuplicateDashboard, Description: descDuplicateDashboard, InputSchema: json.RawMessage(schemaDuplicateDashboard), MCPOnly: true, MinRole: auth.RoleViewer}, r.duplicateDashboard},
	}
}

func proposeModeRefusal(ctx context.Context, action string) (Result, bool) {
	if grantmode.From(ctx) != grantmode.Propose {
		return Result{}, false
	}
	return toolError(codeProposalRequired, "this connection is in propose mode and cannot "+action+" directly; save the change with "+nameUpdateDashboard+", which becomes a proposal, or ask your person to do it"), true
}

func requiredFolderID(raw string) (pgtype.UUID, Result, bool) {
	if raw == "" {
		return pgtype.UUID{}, invalid("folder_id is required; list folders with list_folders"), false
	}
	return parseFolderID(raw)
}

func (r *Registry) moveDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		DashboardID string `json:"dashboard_id"`
		FolderID    string `json:"folder_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return res, nil
	}
	folderID, res, ok := requiredFolderID(in.FolderID)
	if !ok {
		return res, nil
	}
	if res, refused := proposeModeRefusal(ctx, "move a dashboard"); refused {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	dash, ver, err := r.deps.Dashboards.Move(ctx, principal, id, folderID)
	switch {
	case errors.Is(err, dashboards.ErrFolderNotFound):
		return toolError(codeNotFound, "folder not found"), nil
	case errors.Is(err, folders.ErrMoveIntoPersonal):
		return toolError(codePersonalFolder, err.Error()), nil
	case err != nil:
		return saveFailure(err)
	}
	return success("Moved dashboard "+dash.ID, r.savedOut(dash, ver, nil)), nil
}

func (r *Registry) duplicateDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		DashboardID string `json:"dashboard_id"`
		Title       string `json:"title"`
		FolderID    string `json:"folder_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return res, nil
	}
	folderID, res, ok := parseFolderID(in.FolderID)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	dash, ver, err := r.deps.Dashboards.Duplicate(ctx, principal, id, folderID, in.Title)
	if err != nil {
		if res, handled := createFolderFailure(err); handled {
			return res, nil
		}
		return saveFailure(err)
	}
	return success("Duplicated as dashboard "+dash.ID, r.savedOut(dash, ver, nil)), nil
}
