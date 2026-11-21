package datasourceadmin

import (
	"cmp"
	"context"
	"net/url"
	"reflect"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type recordedDatasource struct {
	UID  string `json:"uid"`
	Type string `json:"type"`
	Name string `json:"name"`
	Host string `json:"host"`
}

type recordedChanges struct {
	Name *activity.Change[string] `json:"name,omitempty"`
	Host *activity.Change[string] `json:"host,omitempty"`
}

type recordedAudience struct {
	recordedDatasource
	From string `json:"from"`
	To   string `json:"to"`
}

type recordedUpdate struct {
	recordedDatasource
	Changes            recordedChanges `json:"changes"`
	AddressChanged     bool            `json:"address_changed,omitempty"`
	SettingsChanged    bool            `json:"settings_changed,omitempty"`
	CredentialsChanged bool            `json:"credentials_changed,omitempty"`
}

func hostOf(def Definition) string {
	if def.Type == TypeClickHouse {
		return def.Address
	}
	u, err := url.Parse(def.Address)
	if err != nil {
		return ""
	}
	return u.Host
}

func recordedOf(def Definition) recordedDatasource {
	return recordedDatasource{UID: def.UID, Type: def.Type, Name: def.Name, Host: hostOf(def)}
}

const actionTest audit.Action = "datasource.test"

func testRecord(baseUID string, def Definition, result TestResult) audit.Result {
	outcome, reason := audit.OutcomeSuccess, ""
	if !result.Healthy {
		outcome, reason = audit.OutcomeFailed, audit.ReasonUnhealthy
	}
	return audit.Result{
		Action:  actionTest,
		Outcome: outcome,
		Reason:  reason,
		Target:  audit.Target{Kind: "datasource", ID: cmp.Or(baseUID, def.UID), Name: def.Name},
		Data:    map[string]any{"type": def.Type, "host": hostOf(def), "healthy": result.Healthy},
	}
}

func recordCreated(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, id pgtype.UUID, def Definition) error {
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeDatasourceCreated, id, recordedOf(def))
}

func recordDeleted(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, id pgtype.UUID, def Definition) error {
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeDatasourceDeleted, id, recordedOf(def))
}

func recordUpdated(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, id pgtype.UUID, before, after Definition, credentialsChanged bool) error {
	if before.Audience != after.Audience {
		audience := recordedAudience{recordedDatasource: recordedOf(after), From: string(before.Audience), To: string(after.Audience)}
		if err := activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeDatasourceAudienceChanged, id, audience); err != nil {
			return err
		}
	}
	rest := before
	rest.Name, rest.Address, rest.Audience = after.Name, after.Address, after.Audience
	update := recordedUpdate{
		recordedDatasource: recordedOf(after),
		Changes: recordedChanges{
			Name: activity.Changed(before.Name, after.Name),
			Host: activity.Changed(hostOf(before), hostOf(after)),
		},
		AddressChanged:     before.Address != after.Address,
		SettingsChanged:    !reflect.DeepEqual(rest, after),
		CredentialsChanged: credentialsChanged,
	}
	if update.Changes == (recordedChanges{}) && !update.AddressChanged && !update.SettingsChanged && !update.CredentialsChanged {
		return nil
	}
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeDatasourceUpdated, id, update)
}
