package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameListDashboardVersions   = "list_dashboard_versions"
	nameDiffDashboardVersions   = "diff_dashboard_versions"
	nameRestoreDashboardVersion = "restore_dashboard_version"

	defaultListVersions = 20
	maxListVersions     = 100

	descListDashboardVersions   = "List a dashboard's saved versions, newest first, with who saved each, when, from where and its summary. Page with next_cursor."
	descDiffDashboardVersions   = "Show what changed between two versions of a dashboard as a list of changes, each with op, path and the from and to values. Without from and to it compares the latest version with the one before."
	descRestoreDashboardVersion = "Bring back an earlier version of a dashboard by saving it as a new version; nothing is deleted. Send the latest version you read as parent_version; someone saving since answers version_conflict. Needs edit access to the dashboard's folder."

	propVersionDashboardID = `"dashboard_id":{"type":"string","format":"uuid","description":"The dashboard, from list_dashboards."}`

	schemaListDashboardVersions = `{"type":"object","properties":{` + propVersionDashboardID + `,` +
		`"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Versions per page. Default 20."},` +
		`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"required":["dashboard_id"],"additionalProperties":false}`

	schemaDiffDashboardVersions = `{"type":"object","properties":{` + propVersionDashboardID + `,` +
		`"from":{"type":"integer","minimum":0,"description":"The older version; 0 compares against an empty dashboard. Default one before to."},` +
		`"to":{"type":"integer","minimum":1,"description":"The newer version. Default the latest."}},"required":["dashboard_id"],"additionalProperties":false}`

	schemaRestoreDashboardVersion = `{"type":"object","properties":{` + propVersionDashboardID + `,` +
		`"version":{"type":"integer","minimum":1,"description":"The version to bring back, from list_dashboard_versions."},` +
		`"parent_version":{"type":"integer","minimum":1,"description":"The latest version you read; the restore is saved on top of it."},` +
		`"summary":{"type":"string","maxLength":200,"description":"What the restore is for. Default Restore version N."}},"required":["dashboard_id","version","parent_version"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).versionEntries) }

func (r *Registry) versionEntries() []entry {
	return []entry{
		{Tool{Name: nameListDashboardVersions, Description: descListDashboardVersions, InputSchema: json.RawMessage(schemaListDashboardVersions), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.listDashboardVersions},
		{Tool{Name: nameDiffDashboardVersions, Description: descDiffDashboardVersions, InputSchema: json.RawMessage(schemaDiffDashboardVersions), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.diffDashboardVersions},
		{Tool{Name: nameRestoreDashboardVersion, Description: descRestoreDashboardVersion, InputSchema: json.RawMessage(schemaRestoreDashboardVersion), MCPOnly: true, MinRole: auth.RoleViewer}, r.restoreDashboardVersion},
	}
}

type versionItemOut struct {
	Version       int    `json:"version"`
	ParentVersion *int   `json:"parent_version"`
	RestoredFrom  *int   `json:"restored_from"`
	Summary       string `json:"summary"`
	Author        string `json:"author"`
	Origin        string `json:"origin"`
	OriginClient  string `json:"origin_client,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type versionListOut struct {
	Versions   []versionItemOut `json:"versions"`
	NextCursor *string          `json:"next_cursor"`
}

type versionDiffOut struct {
	From      int           `json:"from"`
	To        int           `json:"to"`
	Changes   []spec.Change `json:"changes"`
	Truncated bool          `json:"truncated,omitempty"`
}

func (r *Registry) listDashboardVersions(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		DashboardID string `json:"dashboard_id"`
		Limit       *int   `json:"limit"`
		Cursor      string `json:"cursor"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultListVersions, 1, maxListVersions, "limit")
	if !ok {
		return res, nil
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	if res, viewable, err := r.viewableDashboard(ctx, id, "dashboard not found"); err != nil || !viewable {
		return res, err
	}
	for {
		items, next, err := r.deps.Dashboards.ListVersions(ctx, id, limit, in.Cursor)
		switch {
		case errors.Is(err, dashboards.ErrInvalidCursor):
			return invalid("cursor is not valid"), nil
		case errors.Is(err, dashboards.ErrNotFound):
			return toolError(codeNotFound, "dashboard not found"), nil
		case err != nil:
			return Result{}, err
		}
		out := versionListOut{Versions: make([]versionItemOut, len(items))}
		for i, v := range items {
			out.Versions[i] = versionItem(v)
		}
		if next != "" {
			out.NextCursor = &next
		}
		kept, cut := fit(out.Versions, listBudget-jsonSize(versionListOut{Versions: []versionItemOut{}, NextCursor: out.NextCursor}))
		if !cut || len(out.Versions) == 1 {
			return success("Listed "+plural(len(out.Versions), "version", "versions"), out), nil
		}
		limit = max(len(kept), 1)
	}
}

func versionItem(v dashboards.VersionMeta) versionItemOut {
	return versionItemOut{
		Version:       v.Number,
		ParentVersion: v.ParentVersion,
		RestoredFrom:  v.RestoredFrom,
		Summary:       v.Summary,
		Author:        v.Author.Name,
		Origin:        v.Origin,
		OriginClient:  v.OriginClient,
		CreatedAt:     v.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (r *Registry) diffDashboardVersions(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		DashboardID string `json:"dashboard_id"`
		From        *int   `json:"from"`
		To          *int   `json:"to"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return res, nil
	}
	if res, viewable, err := r.viewableDashboard(ctx, id, "dashboard not found"); err != nil || !viewable {
		return res, err
	}
	changes, from, to, err := r.deps.Dashboards.Diff(ctx, id, in.From, in.To)
	switch {
	case errors.Is(err, dashboards.ErrInvalidRange):
		return invalid("from must be less than to"), nil
	case errors.Is(err, dashboards.ErrNotFound):
		return toolError(codeNotFound, "dashboard or version not found"), nil
	case err != nil:
		return Result{}, err
	}
	kept, cut := fit(changes, listBudget-jsonSize(versionDiffOut{Changes: []spec.Change{}}))
	return success(fmt.Sprintf("Compared version %d with version %d: %s", from, to, plural(len(kept), "change", "changes")), versionDiffOut{From: from, To: to, Changes: nonNilChanges(kept), Truncated: cut}), nil
}

func nonNilChanges(changes []spec.Change) []spec.Change {
	if changes == nil {
		return []spec.Change{}
	}
	return changes
}

func (r *Registry) restoreDashboardVersion(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		DashboardID   string `json:"dashboard_id"`
		Version       *int   `json:"version"`
		ParentVersion *int   `json:"parent_version"`
		Summary       string `json:"summary"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return res, nil
	}
	if in.Version == nil || *in.Version < 1 {
		return invalid("version is required and must be at least 1; list them with %s", nameListDashboardVersions), nil
	}
	if in.ParentVersion == nil || *in.ParentVersion < 1 {
		return invalid("parent_version is required and must be at least 1: the latest version you read"), nil
	}
	if res, refused := proposeModeRefusal(ctx, "restore a dashboard version"); refused {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	dash, ver, err := r.deps.Dashboards.Restore(ctx, principal, id, *in.Version, *in.ParentVersion, in.Summary)
	if err != nil {
		return saveFailure(err)
	}
	return success(fmt.Sprintf("Restored version %d as version %d", *in.Version, ver.Number), r.savedOut(dash, ver, nil)), nil
}
