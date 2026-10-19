package dashboards

import (
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type authorJSON struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type dashboardJSON struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Tags          []string   `json:"tags"`
	LatestVersion int        `json:"latest_version"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
	UpdatedBy     authorJSON `json:"updated_by"`
}

type versionMetaJSON struct {
	Version        int        `json:"version"`
	ParentVersion  *int       `json:"parent_version"`
	RestoredFrom   *int       `json:"restored_from"`
	Summary        string     `json:"summary"`
	Author         authorJSON `json:"author"`
	CreatedAt      string     `json:"created_at"`
	Origin         *string    `json:"origin"`
	OriginClient   *string    `json:"origin_client"`
	AgentRequestID *string    `json:"agent_request_id"`
}

type versionJSON struct {
	versionMetaJSON
	Spec *spec.Dashboard `json:"spec"`
}

type dashboardVersionJSON struct {
	Dashboard dashboardJSON `json:"dashboard"`
	Version   versionJSON   `json:"version"`
}

type dashboardListJSON struct {
	Dashboards []dashboardJSON `json:"dashboards"`
	NextCursor *string         `json:"next_cursor"`
}

type versionListJSON struct {
	Versions   []versionMetaJSON `json:"versions"`
	NextCursor *string           `json:"next_cursor"`
}

type diffJSON struct {
	From    int           `json:"from"`
	To      int           `json:"to"`
	Changes []spec.Change `json:"changes"`
}

type conflictJSON struct {
	Error         string `json:"error"`
	Code          string `json:"code"`
	LatestVersion int    `json:"latest_version"`
}

type invalidSpecJSON struct {
	Error  string       `json:"error"`
	Code   string       `json:"code"`
	Issues []spec.Issue `json:"issues"`
}

func newAuthorJSON(a Author) authorJSON {
	return authorJSON(a)
}

func newDashboardJSON(d Dashboard) dashboardJSON {
	return dashboardJSON{
		ID:            d.ID,
		Title:         d.Title,
		Tags:          d.Tags,
		LatestVersion: d.LatestVersion,
		CreatedAt:     httpapi.FormatTime(d.CreatedAt),
		UpdatedAt:     httpapi.FormatTime(d.UpdatedAt),
		UpdatedBy:     newAuthorJSON(d.UpdatedBy),
	}
}

func newVersionMetaJSON(v VersionMeta) versionMetaJSON {
	return versionMetaJSON{
		Version:        v.Number,
		ParentVersion:  v.ParentVersion,
		RestoredFrom:   v.RestoredFrom,
		Summary:        v.Summary,
		Author:         newAuthorJSON(v.Author),
		CreatedAt:      httpapi.FormatTime(v.CreatedAt),
		Origin:         httpapi.OptionalString(v.Origin),
		OriginClient:   httpapi.OptionalString(v.OriginClient),
		AgentRequestID: httpapi.OptionalString(v.AgentRequestID),
	}
}

func newDashboardVersionJSON(d Dashboard, v Version) dashboardVersionJSON {
	return dashboardVersionJSON{
		Dashboard: newDashboardJSON(d),
		Version:   versionJSON{versionMetaJSON: newVersionMetaJSON(v.VersionMeta), Spec: v.Spec},
	}
}
