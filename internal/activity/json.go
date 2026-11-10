package activity

import (
	"encoding/json"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

type TargetJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type EventJSON struct {
	Seq         int64           `json:"seq"`
	Type        string          `json:"type"`
	Target      TargetJSON      `json:"target"`
	DashboardID *string         `json:"dashboard_id"`
	FolderID    *string         `json:"folder_id"`
	Actor       actor.JSON      `json:"actor"`
	Data        json.RawMessage `json:"data"`
	CreatedAt   string          `json:"created_at"`
}

func JSONOf(e Event) EventJSON {
	return EventJSON{
		Seq:         e.Seq,
		Type:        e.Type,
		Target:      TargetJSON{Kind: e.TargetKind, ID: e.TargetID},
		DashboardID: httpapi.OptionalString(e.DashboardID),
		FolderID:    httpapi.OptionalString(e.FolderID),
		Actor:       e.Actor.JSON(),
		Data:        e.Data,
		CreatedAt:   httpapi.FormatTime(e.CreatedAt),
	}
}
