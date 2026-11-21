package activity

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type auditMapping struct {
	action  audit.Action
	result  string
	outcome audit.Outcome
}

var auditActions = map[string]auditMapping{
	TypeDashboardCreated:        {action: "dashboard.create"},
	TypeDashboardVersionCreated: {action: "dashboard.update"},
	TypeDashboardRestored:       {action: "dashboard.restore"},
	TypeDashboardMoved:          {action: "dashboard.move"},
	TypeDashboardDeleted:        {action: "dashboard.delete"},

	TypeAnnotationCreated: {action: "annotation.create"},
	TypeAnnotationUpdated: {action: "annotation.update"},
	TypeAnnotationDeleted: {action: "annotation.delete"},

	TypeFindingCreated: {action: "finding.create"},
	TypeFindingPinned:  {action: "finding.pin"},
	TypeFindingMoved:   {action: "finding.move"},

	TypeSavedSearchCreated: {action: "saved_search.create"},
	TypeSavedSearchUpdated: {action: "saved_search.update"},
	TypeSavedSearchMoved:   {action: "saved_search.move"},
	TypeSavedSearchDeleted: {action: "saved_search.delete"},

	TypeObservationCreated:        {action: "observation.create"},
	TypeObservationUpdated:        {action: "observation.update"},
	TypeObservationMoved:          {action: "observation.move"},
	TypeObservationDeleted:        {action: "observation.delete"},
	TypeObservationResolved:       {action: "observation.resolve"},
	TypeObservationReopened:       {action: "observation.reopen"},
	TypeObservationPublished:      {action: "observation.publish"},
	TypeObservationRCAWithdrawn:   {action: "observation.rca_withdraw"},
	TypeObservationBlockAdded:     {action: "observation.block_add"},
	TypeObservationBlockUpdated:   {action: "observation.block_update"},
	TypeObservationBlockKept:      {action: "observation.block_keep"},
	TypeObservationBlockDiscarded: {action: "observation.block_discard"},
	TypeObservationBlockRemoved:   {action: "observation.block_remove"},

	TypeProposalOpened:    {action: "proposal.open"},
	TypeProposalUpdated:   {action: "proposal.update"},
	TypeProposalMerged:    {action: "proposal.merge"},
	TypeProposalDiscarded: {action: "proposal.discard"},

	TypeThreadOpened:    {action: "thread.open"},
	TypeThreadReplied:   {action: "thread.message"},
	TypeThreadEdited:    {action: "thread.message_edit"},
	TypeThreadDeleted:   {action: "thread.message_delete"},
	TypeThreadResolved:  {action: "thread.resolve"},
	TypeThreadReopened:  {action: "thread.reopen"},
	TypeThreadMentioned: {action: "thread.mention"},

	TypeRunQueued:    {action: "run.queue"},
	TypeRunStarted:   {action: "run.start"},
	TypeRunSucceeded: {action: "run.finish", result: "succeeded"},
	TypeRunFailed:    {action: "run.finish", result: "failed", outcome: audit.OutcomeFailed},
	TypeRunCanceled:  {action: "run.finish", result: "canceled", outcome: audit.OutcomeFailed},
	TypeRunCanceling: {action: "run.cancel"},
	TypeRunReleased:  {action: "run.release"},
	TypeRunKept:      {action: "run.keep"},

	TypePublicLinkCreated: {action: "public_link.create"},
	TypePublicLinkRevoked: {action: "public_link.revoke"},

	TypeAccessRequestOpened:  {action: "access_request.open"},
	TypeAccessRequestGranted: {action: "access_request.grant"},

	TypeUserRoleChanged:           {action: "user.role_change"},
	TypeUserSignedOutEverywhere:   {action: "user.sign_out_everywhere"},
	TypeUserInvited:               {action: "user.invite"},
	TypeUserInviteUpdated:         {action: "user.invite_update"},
	TypeUserInviteWithdrawn:       {action: "user.invite_withdraw"},
	TypeUserDeactivated:           {action: "user.deactivate"},
	TypeUserReactivated:           {action: "user.reactivate"},
	TypeUserRemoved:               {action: "user.remove"},
	TypeServiceAccountCreated:     {action: "service_account.create"},
	TypeServiceAccountUpdated:     {action: "service_account.update"},
	TypeServiceAccountDeleted:     {action: "service_account.delete"},
	TypeServiceAccountKeyCreated:  {action: "service_account.key_create"},
	TypeServiceAccountKeyRevoked:  {action: "service_account.key_revoke"},
	TypeWebhookCreated:            {action: "webhook.create"},
	TypeWebhookUpdated:            {action: "webhook.update"},
	TypeWebhookSecretRotated:      {action: "webhook.secret_rotate"},
	TypeWebhookDeleted:            {action: "webhook.delete"},
	TypeMCPClientConnected:        {action: "mcp_client.connect"},
	TypeMCPClientDisconnected:     {action: "mcp_client.disconnect"},
	TypeOutboundDomainAdded:       {action: "outbound_domain.add"},
	TypeOutboundDomainRemoved:     {action: "outbound_domain.remove"},
	TypeGrafanaConnectionCreated:  {action: "grafana_connection.create"},
	TypeGrafanaConnectionUpdated:  {action: "grafana_connection.update"},
	TypeGrafanaConnectionDeleted:  {action: "grafana_connection.delete"},
	TypeDatasourceCreated:         {action: "datasource.create"},
	TypeDatasourceUpdated:         {action: "datasource.update"},
	TypeDatasourceAudienceChanged: {action: "datasource.audience_change"},
	TypeDatasourceDeleted:         {action: "datasource.delete"},
}

func AuditAction(typ string) (audit.Action, bool) {
	m, ok := auditActions[typ]
	return m.action, ok
}

func recordAudit(ctx context.Context, q *dbsqlc.Queries, seq int64, e Entry, columns actor.Columns, raw []byte) error {
	mapping, ok := auditActions[e.Type]
	if !ok {
		return fmt.Errorf("activity type %q has no audit action", e.Type)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("read activity data for audit: %w", err)
	}
	if mapping.result != "" {
		data["result"] = mapping.result
	}
	maps.Copy(data, e.AuditOnly)
	target := audit.Target{Kind: targetKind(e.Type), ID: e.TargetID.String(), Name: displayName(data)}
	switch e.Owner.kind {
	case ownerDashboard:
		target.DashboardID = nullUUID(e.Owner.id)
	case ownerObservation:
		target.ObservationID = nullUUID(e.Owner.id)
	}
	by := columns.AuditBy()
	by.Agent = e.Agent
	return audit.Record(ctx, q, audit.Event{Action: cmp.Or(e.AuditAction, mapping.action), Outcome: mapping.outcome, Target: target, Data: data, By: by, Activity: seq})
}

func displayName(data map[string]any) string {
	for _, key := range []string{"title", "name"} {
		if s, ok := data[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func nullUUID(id pgtype.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id.Bytes, Valid: id.Valid}
}
