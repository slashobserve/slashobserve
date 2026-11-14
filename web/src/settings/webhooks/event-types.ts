import type { components } from "@/api/schema.gen"

export type ActivityType = components["schemas"]["ActivityType"]

export const eventTypeLabels = {
  "dashboard.created": "Dashboard created",
  "dashboard.version_created": "Version saved",
  "dashboard.restored": "Version restored",
  "dashboard.moved": "Dashboard moved",
  "dashboard.deleted": "Dashboard deleted",
  "annotation.created": "Annotation added",
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
  "observation.resolved": "Observation resolved",
  "observation.reopened": "Observation reopened",
  "observation.published": "RCA published",
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
