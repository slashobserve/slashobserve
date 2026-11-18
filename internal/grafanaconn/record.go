package grafanaconn

import (
	"context"
	"net/url"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type recordedConnection struct {
	Name string `json:"name"`
	Host string `json:"host"`
}

type recordedChanges struct {
	Name *activity.Change[string] `json:"name,omitempty"`
	Host *activity.Change[string] `json:"host,omitempty"`
}

type recordedUpdate struct {
	Name         string          `json:"name"`
	Host         string          `json:"host"`
	Changes      recordedChanges `json:"changes"`
	TokenChanged bool            `json:"token_changed,omitempty"`
}

type recorder func(ctx context.Context, q *dbsqlc.Queries) error

func hostOf(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return u.Host
}

func recordCreated(by auth.Principal, id pgtype.UUID, name, address string) recorder {
	return func(ctx context.Context, q *dbsqlc.Queries) error {
		return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeGrafanaConnectionCreated, id, recordedConnection{Name: name, Host: hostOf(address)})
	}
}

func recordDeleted(by auth.Principal, id pgtype.UUID, name, address string) recorder {
	return func(ctx context.Context, q *dbsqlc.Queries) error {
		return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeGrafanaConnectionDeleted, id, recordedConnection{Name: name, Host: hostOf(address)})
	}
}

func recordUpdated(by auth.Principal, id pgtype.UUID, before Connection, after dbsqlc.UpdateGrafanaConnectionParams, tokenChanged bool) recorder {
	update := recordedUpdate{
		Name: after.Name, Host: hostOf(after.Url), TokenChanged: tokenChanged,
		Changes: recordedChanges{
			Name: activity.Changed(before.Name, after.Name),
			Host: activity.Changed(hostOf(before.URL), hostOf(after.Url)),
		},
	}
	return func(ctx context.Context, q *dbsqlc.Queries) error {
		if update.Changes == (recordedChanges{}) && !update.TokenChanged {
			return nil
		}
		return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeGrafanaConnectionUpdated, id, update)
	}
}
