import type { components } from "@/api/schema.gen"

export type ActivityType = components["schemas"]["ActivityType"]

export const eventTypeLabels = {
  "dashboard.created": "Dashboard created",
  "dashboard.version_created": "Version saved",
  "dashboard.restored": "Version restored",
  "dashboard.moved": "Dashboard moved",
  "dashboard.deleted": "Dashboard deleted",
  "annotation.created": "Annotation added",
  "annotation.updated": "Annotation edited",
  "annotation.deleted": "Annotation removed",
  "finding.created": "Finding saved",
  "finding.pinned": "Finding pinned",
  "finding.moved": "Finding moved",
  "saved_search.created": "Saved search created",
  "saved_search.updated": "Saved search updated",
  "saved_search.moved": "Saved search moved",
  "saved_search.deleted": "Saved search deleted",
  "observation.created": "Observation created",
  "observation.updated": "Observation updated",
  "observation.moved": "Observation moved",
  "observation.deleted": "Observation deleted",
  "observation.resolved": "Observation resolved",
  "observation.reopened": "Observation reopened",
  "observation.published": "RCA published",
  "observation.rca_withdrawn": "RCA revision withdrawn",
  "observation.block_added": "Block added",
  "observation.block_updated": "Block updated",
  "observation.block_kept": "Block kept",
  "observation.block_discarded": "Block discarded",
  "observation.block_removed": "Block removed",
  "proposal.opened": "Proposal opened",
  "proposal.updated": "Proposal updated",
  "proposal.merged": "Proposal merged",
  "proposal.discarded": "Proposal discarded",
  "thread.opened": "Thread opened",
  "thread.replied": "Thread replied to",
  "thread.edited": "Message edited",
  "thread.deleted": "Message deleted",
  "thread.resolved": "Thread resolved",
  "thread.reopened": "Thread reopened",
  "thread.mentioned": "Mention",
  "run.queued": "Run queued",
  "run.started": "Run started",
  "run.succeeded": "Run succeeded",
  "run.failed": "Run failed",
  "run.canceled": "Run canceled",
  "run.canceling": "Run canceling",
  "run.released": "Run released",
  "run.kept": "Build kept",
  "public_link.created": "Public link created",
  "public_link.revoked": "Public link revoked",
  "access_request.opened": "Comment access requested",
  "access_request.granted": "Comment access granted",
  "user.role_changed": "Role changed",
  "user.signed_out_everywhere": "Signed out everywhere",
  "user.invited": "Person invited",
  "user.invite_updated": "Invite changed",
  "user.invite_withdrawn": "Invite withdrawn",
  "user.deactivated": "Person deactivated",
  "user.reactivated": "Person reactivated",
  "user.removed": "Person removed",
  "service_account.created": "Service account created",
  "service_account.updated": "Service account updated",
  "service_account.deleted": "Service account deleted",
  "service_account.key_created": "Service account key created",
  "service_account.key_revoked": "Service account key revoked",
  "webhook.created": "Webhook created",
  "webhook.updated": "Webhook updated",
  "webhook.secret_rotated": "Webhook secret rotated",
  "webhook.deleted": "Webhook deleted",
  "mcp_client.connected": "Agent connected",
  "mcp_client.disconnected": "Agent disconnected",
  "outbound_domain.added": "Outbound domain added",
  "outbound_domain.removed": "Outbound domain removed",
  "grafana_connection.created": "Grafana connection created",
  "grafana_connection.updated": "Grafana connection updated",
  "grafana_connection.deleted": "Grafana connection deleted",
  "datasource.created": "Datasource added",
  "datasource.updated": "Datasource changed",
  "datasource.audience_changed": "Datasource audience changed",
  "datasource.deleted": "Datasource deleted",
} satisfies Record<ActivityType, string>

const groupLabels: Record<string, string> = {
  dashboard: "Dashboards",
  annotation: "Annotations",
  finding: "Findings",
  saved_search: "Saved searches",
  observation: "Observations",
  proposal: "Proposals",
  thread: "Comment threads",
  run: "Agent runs",
  public_link: "Public links",
  access_request: "Access requests",
  user: "People",
  service_account: "Service accounts",
  webhook: "Webhooks",
  mcp_client: "Connected agents",
  outbound_domain: "Outbound access",
  grafana_connection: "Grafana connections",
  datasource: "Datasources",
}

export const allEventTypes = Object.keys(eventTypeLabels) as ActivityType[]

export type EventTypeGroup = {
  prefix: string
  label: string
  types: ActivityType[]
}

function groupByPrefix(types: readonly ActivityType[]): EventTypeGroup[] {
  const groups: EventTypeGroup[] = []
  for (const type of types) {
    const prefix = type.slice(0, type.indexOf("."))
    const group = groups.find((existing) => existing.prefix === prefix)
    if (group === undefined) {
      groups.push({
        prefix,
        label: groupLabels[prefix] ?? prefix,
        types: [type],
      })
    } else {
      group.types.push(type)
    }
  }
  return groups
}

export const eventTypeGroups = groupByPrefix(allEventTypes)

export function toggleEventType(
  selected: readonly ActivityType[],
  type: ActivityType,
  on: boolean
): ActivityType[] {
  const next = new Set(selected)
  if (on) next.add(type)
  else next.delete(type)
  return allEventTypes.filter((candidate) => next.has(candidate))
}

export function toggleGroup(
  selected: readonly ActivityType[],
  group: EventTypeGroup,
  on: boolean
): ActivityType[] {
  const next = new Set(selected)
  for (const type of group.types) {
    if (on) next.add(type)
    else next.delete(type)
  }
  return allEventTypes.filter((candidate) => next.has(candidate))
}

export function groupCountText(
  selected: readonly ActivityType[],
  group: EventTypeGroup
): string {
  const chosen = group.types.filter((type) => selected.includes(type)).length
  return `${chosen} of ${group.types.length}`
}

export function openGroupPrefixes(selected: readonly ActivityType[]): string[] {
  return eventTypeGroups
    .filter((group) => group.types.some((type) => selected.includes(type)))
    .map((group) => group.prefix)
}

export function eventTypeSummary(types: readonly ActivityType[]): string {
  if (types.length === allEventTypes.length) return "All events"
  if (types.length <= 2) {
    return types.map((type) => eventTypeLabels[type] ?? type).join(", ")
  }
  return `${types.length} event types`
}
