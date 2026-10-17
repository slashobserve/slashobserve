package events

const Channel = "slashobserve_events"

const TypeVersionCreated = "version.created"

type Author struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Event struct {
	Type          string  `json:"type"`
	DashboardID   string  `json:"dashboard_id"`
	Version       int     `json:"version,omitempty"`
	ParentVersion *int    `json:"parent_version,omitempty"`
	RestoredFrom  *int    `json:"restored_from,omitempty"`
	Summary       string  `json:"summary,omitempty"`
	Author        *Author `json:"author,omitempty"`
	CreatedAt     string  `json:"created_at,omitempty"`
	CommentID     string  `json:"comment_id,omitempty"`
	PanelID       string  `json:"panel_id,omitempty"`
	Status        string  `json:"status,omitempty"`
}
