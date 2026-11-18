package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	maxListFolders = 500

	nameCreateFolder = "create_folder"
	nameRenameFolder = "rename_folder"

	descCreateFolder = "Create a folder and get its id, so create_dashboard, create_observation and move_dashboard can put work there. The title is trimmed, 1 to 100 characters, and unique without regard to case. You manage the folder you create. The folder rules refuse a Viewer; no tool deletes a folder or changes who can see one, which people do in the web app."
	descRenameFolder = "Rename a folder from list_folders. Needs the Manage permission on it, which its creator and Admins have. The General folder cannot be renamed."

	propFolderTitle = `"title":{"type":"string","minLength":1,"maxLength":100,"description":"The folder's name, 1 to 100 characters with no control characters."}`

	schemaCreateFolder = `{"type":"object","properties":{` + propFolderTitle + `},"required":["title"],"additionalProperties":false}`
	schemaRenameFolder = `{"type":"object","properties":{"folder_id":{"type":"string","format":"uuid","description":"The folder to rename, from list_folders."},` + propFolderTitle + `},"required":["folder_id","title"],"additionalProperties":false}`
)

var errFoldersNotConfigured = errors.New("folders are not configured")

func init() { registerEntries((*Registry).folderEntries) }

func (r *Registry) folderEntries() []entry {
	return []entry{
		{Tool{Name: nameListFolders, Description: descListFolders, InputSchema: json.RawMessage(schemaEmpty), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.listFolders},
		{Tool{Name: nameCreateFolder, Description: descCreateFolder, InputSchema: json.RawMessage(schemaCreateFolder), MCPOnly: true, MinRole: auth.RoleViewer}, r.createFolder},
		{Tool{Name: nameRenameFolder, Description: descRenameFolder, InputSchema: json.RawMessage(schemaRenameFolder), MCPOnly: true, MinRole: auth.RoleViewer}, r.renameFolder},
	}
}

type folderItemOut struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	General   bool   `json:"general"`
	CanCreate bool   `json:"can_create"`
}

type folderListOut struct {
	Folders   []folderItemOut `json:"folders"`
	Truncated bool            `json:"truncated,omitempty"`
}

func (r *Registry) listFolders(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if r.deps.Folders == nil {
		return Result{}, errFoldersNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	levels, err := r.deps.Folders.Levels(ctx, principal)
	if err != nil {
		return Result{}, err
	}
	all := levels.Folders()
	visible := all[:min(len(all), maxListFolders)]
	items := make([]folderItemOut, len(visible))
	for i, f := range visible {
		items[i] = folderItemOut{ID: f.ID.String(), Title: f.Title, General: f.General, CanCreate: levels.Of(f.ID).Create}
	}
	kept, cut := fit(items, listBudget)
	return success("Listed "+plural(len(kept), "folder", "folders"), folderListOut{Folders: kept, Truncated: cut || len(all) > len(visible)}), nil
}

func (r *Registry) createFolder(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Title string `json:"title"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if r.deps.Folders == nil {
		return Result{}, errFoldersNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	folder, abilities, err := r.deps.Folders.CreateFolder(ctx, principal, in.Title)
	if res, ok := folderWriteFailure(err); ok {
		return res, nil
	}
	if err != nil {
		return Result{}, err
	}
	return success("Created folder "+clip(folder.Title, maxStepNameBytes), folderItemOut{ID: folder.ID.String(), Title: folder.Title, General: folder.General, CanCreate: abilities.Create}), nil
}

func (r *Registry) renameFolder(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		FolderID string `json:"folder_id"`
		Title    string `json:"title"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, err := db.ParseUUID(in.FolderID)
	if err != nil {
		return invalid("folder_id must be a UUID from list_folders"), nil
	}
	if r.deps.Folders == nil {
		return Result{}, errFoldersNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	folder, abilities, err := r.deps.Folders.RenameFolder(ctx, principal, id, in.Title)
	if res, ok := folderWriteFailure(err); ok {
		return res, nil
	}
	if err != nil {
		return Result{}, err
	}
	return success("Renamed a folder to "+clip(folder.Title, maxStepNameBytes), folderItemOut{ID: folder.ID.String(), Title: folder.Title, General: folder.General, CanCreate: abilities.Create}), nil
}

func folderWriteFailure(err error) (Result, bool) {
	var invalidTitle *folders.InvalidTitleError
	switch {
	case errors.As(err, &invalidTitle):
		return invalid("%s", invalidTitle.Message), true
	case errors.Is(err, folders.ErrFolderExists):
		return toolError("folder_exists", "a folder with that name already exists"), true
	case errors.Is(err, folders.ErrGeneralFolder):
		return toolError("general_folder", "the General folder cannot be renamed"), true
	case errors.Is(err, folders.ErrNotFound):
		return toolError(codeNotFound, "folder not found"), true
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, "your role or your access to this folder does not allow this"), true
	}
	return Result{}, false
}

func (r *Registry) viewableDashboard(ctx context.Context, id pgtype.UUID, notFoundMessage string) (Result, bool, error) {
	_, res, ok, err := r.dashboardAbilities(ctx, id, notFoundMessage)
	return res, ok, err
}

func (r *Registry) commentableDashboard(ctx context.Context, id pgtype.UUID, notFoundMessage string) (Result, bool, error) {
	abilities, res, ok, err := r.dashboardAbilities(ctx, id, notFoundMessage)
	if err != nil || !ok {
		return res, ok, err
	}
	if !abilities.Comment {
		return toolError(codeForbiddenRole, roleDeniedMessage), false, nil
	}
	return Result{}, true, nil
}

func (r *Registry) dashboardAbilities(ctx context.Context, id pgtype.UUID, notFoundMessage string) (folders.Abilities, Result, bool, error) {
	if r.deps.Folders == nil {
		return folders.Abilities{}, Result{}, false, errFoldersNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	access, err := r.deps.Folders.Dashboard(ctx, principal, id)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden), errors.Is(err, dashboards.ErrNotFound):
		return folders.Abilities{}, toolError(codeNotFound, notFoundMessage), false, nil
	case err != nil:
		return folders.Abilities{}, Result{}, false, err
	}
	return access.Abilities, Result{}, true, nil
}

func createFolderFailure(err error) (Result, bool) {
	switch {
	case errors.Is(err, dashboards.ErrFolderNotFound):
		return toolError(codeNotFound, "folder not found"), true
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, "you cannot create dashboards in this folder"), true
	}
	return Result{}, false
}
