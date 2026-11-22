package grafanaconn

import (
	"context"
	"net/url"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/audit"
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

type updateRecorder func(ctx context.Context, q *dbsqlc.Queries, before dbsqlc.GrafanaConnection) error

const (
	actionTest           audit.Action = "grafana_connection.test"
	actionFetchDashboard audit.Action = "grafana_connection.fetch_dashboard"
)

func testRecord(id string, tested Connection) audit.Result {
	healthy := tested.LastTest != nil && tested.LastTest.OK
	outcome, reason := audit.OutcomeSuccess, ""
	if !healthy {
		outcome, reason = audit.OutcomeFailed, audit.ReasonUnhealthy
	}
	return audit.Result{
		Action:  actionTest,
		Outcome: outcome,
		Reason:  reason,
		Target:  audit.Target{Kind: "grafana_connection", ID: id, Name: tested.Name},
		Data:    map[string]any{"host": hostOf(tested.URL), "healthy": healthy},
	}
}

func fetchRecord(linked LinkedDashboard) audit.Result {
	return audit.Result{
		Action:  actionFetchDashboard,
		Outcome: audit.OutcomeSuccess,
		Target:  audit.Target{Kind: "grafana_connection", ID: linked.ConnectionID, Name: linked.ConnectionName},
		Data:    map[string]any{"host": linked.Host},
	}
}

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

func recordUpdated(by auth.Principal, id pgtype.UUID, after dbsqlc.UpdateGrafanaConnectionParams, tokenChanged bool) updateRecorder {
	return func(ctx context.Context, q *dbsqlc.Queries, before dbsqlc.GrafanaConnection) error {
		update := recordedUpdate{
			Name: after.Name, Host: hostOf(after.Url), TokenChanged: tokenChanged,
			Changes: recordedChanges{
				Name: activity.Changed(before.Name, after.Name),
				Host: activity.Changed(hostOf(before.Url), hostOf(after.Url)),
			},
		}
		if update.Changes == (recordedChanges{}) && !update.TokenChanged {
			return nil
		}
		return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeGrafanaConnectionUpdated, id, update)
	}
}
