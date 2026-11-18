package webhooks

import (
	"context"
	"net/url"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type recordedWebhook struct {
	Name       string   `json:"name"`
	Host       string   `json:"host"`
	EventTypes []string `json:"event_types,omitempty"`
}

type recordedChanges struct {
	Name       *activity.Change[string]   `json:"name,omitempty"`
	Host       *activity.Change[string]   `json:"host,omitempty"`
	Enabled    *activity.Change[bool]     `json:"enabled,omitempty"`
	EventTypes *activity.Change[[]string] `json:"event_types,omitempty"`
}

type recordedUpdate struct {
	Name       string          `json:"name"`
	Host       string          `json:"host"`
	Changes    recordedChanges `json:"changes"`
	URLChanged bool            `json:"url_changed,omitempty"`
}

func hostOf(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func record(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, typ string, id pgtype.UUID, data any) error {
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), typ, id, data)
}

func recordUpdate(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, before, after dbsqlc.Webhook) error {
	update := recordedUpdate{
		Name: after.Name,
		Host: hostOf(after.Url),
		Changes: recordedChanges{
			Name:    activity.Changed(before.Name, after.Name),
			Host:    activity.Changed(hostOf(before.Url), hostOf(after.Url)),
			Enabled: activity.Changed(!before.DisabledAt.Valid, !after.DisabledAt.Valid),
		},
		URLChanged: before.Url != after.Url,
	}
	if !slices.Equal(before.EventTypes, after.EventTypes) {
		update.Changes.EventTypes = &activity.Change[[]string]{From: before.EventTypes, To: after.EventTypes}
	}
	if update.Changes == (recordedChanges{}) && !update.URLChanged {
		return nil
	}
	return record(ctx, q, by, activity.TypeWebhookUpdated, after.ID, update)
}
