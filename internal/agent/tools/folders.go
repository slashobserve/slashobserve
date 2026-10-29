package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
)

const maxListFolders = 500

var errFoldersNotConfigured = errors.New("folders are not configured")

func (r *Registry) folderEntries() []entry {
	return []entry{
		{Tool{Name: nameListFolders, Description: descListFolders, InputSchema: json.RawMessage(schemaEmpty), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.listFolders},
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

func (r *Registry) viewableDashboard(ctx context.Context, id pgtype.UUID, notFoundMessage string) (Result, bool, error) {
	if r.deps.Folders == nil {
		return Result{}, false, errFoldersNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	_, err := r.deps.Folders.Dashboard(ctx, principal, id)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden), errors.Is(err, dashboards.ErrNotFound):
		return toolError(codeNotFound, notFoundMessage), false, nil
	case err != nil:
		return Result{}, false, err
	}
	return Result{}, true, nil
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
