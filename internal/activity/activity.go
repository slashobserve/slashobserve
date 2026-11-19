package activity

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/actor"
)

const (
	TypeDashboardCreated        = "dashboard.created"
	TypeDashboardVersionCreated = "dashboard.version_created"
	TypeDashboardRestored       = "dashboard.restored"
	TypeDashboardMoved          = "dashboard.moved"
	TypeDashboardDeleted        = "dashboard.deleted"

	TypeAnnotationCreated = "annotation.created"
	TypeAnnotationUpdated = "annotation.updated"
	TypeAnnotationDeleted = "annotation.deleted"

	TypeFindingCreated = "finding.created"
	TypeFindingPinned  = "finding.pinned"
	TypeFindingMoved   = "finding.moved"

	TypeSavedSearchCreated = "saved_search.created"
	TypeSavedSearchUpdated = "saved_search.updated"
	TypeSavedSearchMoved   = "saved_search.moved"
	TypeSavedSearchDeleted = "saved_search.deleted"

	TypeObservationCreated        = "observation.created"
	TypeObservationUpdated        = "observation.updated"
	TypeObservationMoved          = "observation.moved"
	TypeObservationDeleted        = "observation.deleted"
	TypeObservationResolved       = "observation.resolved"
	TypeObservationReopened       = "observation.reopened"
	TypeObservationPublished      = "observation.published"
	TypeObservationRCAWithdrawn   = "observation.rca_withdrawn"
	TypeObservationBlockAdded     = "observation.block_added"
	TypeObservationBlockUpdated   = "observation.block_updated"
	TypeObservationBlockKept      = "observation.block_kept"
	TypeObservationBlockDiscarded = "observation.block_discarded"
	TypeObservationBlockRemoved   = "observation.block_removed"

	TypeProposalOpened    = "proposal.opened"
	TypeProposalUpdated   = "proposal.updated"
	TypeProposalMerged    = "proposal.merged"
	TypeProposalDiscarded = "proposal.discarded"

	TypeThreadOpened    = "thread.opened"
	TypeThreadReplied   = "thread.replied"
	TypeThreadEdited    = "thread.edited"
	TypeThreadDeleted   = "thread.deleted"
	TypeThreadResolved  = "thread.resolved"
	TypeThreadReopened  = "thread.reopened"
	TypeThreadMentioned = "thread.mentioned"

	TypeRunQueued    = "run.queued"
	TypeRunStarted   = "run.started"
	TypeRunSucceeded = "run.succeeded"
	TypeRunFailed    = "run.failed"
	TypeRunCanceled  = "run.canceled"

	TypeRunCanceling = "run.canceling"
	TypeRunReleased  = "run.released"
	TypeRunKept      = "run.kept"

	TypePublicLinkCreated = "public_link.created"
	TypePublicLinkRevoked = "public_link.revoked"

	TypeUserRoleChanged          = "user.role_changed"
	TypeUserSignedOutEverywhere  = "user.signed_out_everywhere"
	TypeUserInvited              = "user.invited"
	TypeUserInviteUpdated        = "user.invite_updated"
	TypeUserInviteWithdrawn      = "user.invite_withdrawn"
	TypeUserDeactivated          = "user.deactivated"
	TypeUserReactivated          = "user.reactivated"
	TypeUserRemoved              = "user.removed"
	TypeServiceAccountCreated    = "service_account.created"
	TypeServiceAccountUpdated    = "service_account.updated"
	TypeServiceAccountDeleted    = "service_account.deleted"
	TypeServiceAccountKeyCreated = "service_account.key_created"
	TypeServiceAccountKeyRevoked = "service_account.key_revoked"
	TypeWebhookCreated           = "webhook.created"
	TypeWebhookUpdated           = "webhook.updated"
	TypeWebhookSecretRotated     = "webhook.secret_rotated"
	TypeWebhookDeleted           = "webhook.deleted"
	TypeMCPClientConnected       = "mcp_client.connected"
	TypeMCPClientDisconnected    = "mcp_client.disconnected"
	TypeOutboundDomainAdded      = "outbound_domain.added"
	TypeOutboundDomainRemoved    = "outbound_domain.removed"
	TypeGrafanaConnectionCreated = "grafana_connection.created"
	TypeGrafanaConnectionUpdated = "grafana_connection.updated"
	TypeGrafanaConnectionDeleted = "grafana_connection.deleted"
)

var publicLinkTypes = []string{
	TypePublicLinkCreated,
	TypePublicLinkRevoked,
}

var adminTypes = []string{
	TypeUserRoleChanged,
	TypeUserSignedOutEverywhere,
	TypeUserInvited,
	TypeUserInviteUpdated,
	TypeUserInviteWithdrawn,
	TypeUserDeactivated,
	TypeUserReactivated,
	TypeUserRemoved,
	TypeServiceAccountCreated,
	TypeServiceAccountUpdated,
	TypeServiceAccountDeleted,
	TypeServiceAccountKeyCreated,
	TypeServiceAccountKeyRevoked,
	TypeWebhookCreated,
	TypeWebhookUpdated,
	TypeWebhookSecretRotated,
	TypeWebhookDeleted,
	TypeMCPClientConnected,
	TypeMCPClientDisconnected,
	TypeOutboundDomainAdded,
	TypeOutboundDomainRemoved,
	TypeGrafanaConnectionCreated,
	TypeGrafanaConnectionUpdated,
	TypeGrafanaConnectionDeleted,
}

var workspaceTypes = []string{
	TypeDashboardCreated,
	TypeDashboardVersionCreated,
	TypeDashboardRestored,
	TypeDashboardMoved,
	TypeDashboardDeleted,
	TypeAnnotationCreated,
	TypeAnnotationUpdated,
	TypeAnnotationDeleted,
	TypeFindingCreated,
	TypeFindingPinned,
	TypeFindingMoved,
	TypeSavedSearchCreated,
	TypeSavedSearchUpdated,
	TypeSavedSearchMoved,
	TypeSavedSearchDeleted,
	TypeObservationCreated,
	TypeObservationUpdated,
	TypeObservationMoved,
	TypeObservationDeleted,
	TypeObservationResolved,
	TypeObservationReopened,
	TypeObservationPublished,
	TypeObservationRCAWithdrawn,
	TypeObservationBlockAdded,
	TypeObservationBlockUpdated,
	TypeObservationBlockKept,
	TypeObservationBlockDiscarded,
	TypeObservationBlockRemoved,
	TypeProposalOpened,
	TypeProposalUpdated,
	TypeProposalMerged,
	TypeProposalDiscarded,
	TypeThreadOpened,
	TypeThreadReplied,
	TypeThreadEdited,
	TypeThreadDeleted,
	TypeThreadResolved,
	TypeThreadReopened,
	TypeThreadMentioned,
	TypeRunQueued,
	TypeRunStarted,
	TypeRunSucceeded,
	TypeRunFailed,
	TypeRunCanceled,
	TypeRunCanceling,
	TypeRunReleased,
	TypeRunKept,
}

var knownTypes = slices.Concat(workspaceTypes, publicLinkTypes, adminTypes)

var auditTypes = slices.Concat(adminTypes, publicLinkTypes)

func Types() []string { return slices.Clone(knownTypes) }

func AdminTypes() []string { return slices.Clone(adminTypes) }

func IsAdminType(t string) bool { return slices.Contains(adminTypes, t) }

func Known(t string) bool { return slices.Contains(knownTypes, t) }

func targetKind(t string) string {
	kind, _, _ := strings.Cut(t, ".")
	return kind
}

type Event struct {
	Seq         int64
	Type        string
	TargetKind  string
	TargetID    string
	DashboardID string
	FolderID    string
	Actor       actor.Actor
	Data        json.RawMessage
	CreatedAt   time.Time
	Position    Position
}

type Position struct {
	xid uint64
	seq int64
}

func At(xid uint64, seq int64) Position { return Position{xid: xid, seq: seq} }

func (p Position) Xid() uint64 { return p.xid }

func (p Position) Seq() int64 { return p.seq }

func later(a, b Position) Position {
	if b.xid > a.xid || (b.xid == a.xid && b.seq > a.seq) {
		return b
	}
	return a
}
