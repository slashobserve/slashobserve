import type { components } from "@/api/schema.gen"
import {
  chatPath,
  dashboardPath,
  findingPath,
  observationPath,
  proposalPath,
} from "@/lib/links"

export type InboxItem = components["schemas"]["InboxItem"]
export type InboxReason = NonNullable<InboxItem["reason"]>
type TargetKind = InboxItem["target"]["kind"]

export type ItemText = { actor: string; verb: string; object: string }
export type ItemGroup = {
  label: "Needs you" | "Workspace activity"
  items: InboxItem[]
}

const kindWords: Record<TargetKind, string> = {
  dashboard: "a dashboard",
  annotation: "an annotation",
  finding: "an observation",
  saved_search: "a saved search",
  observation: "an observation",
  proposal: "a proposal",
  thread: "a thread",
  run: "a request",
}

const reasonLabels: Record<InboxReason, string> = {
  proposal: "Proposal",
  requested_block: "Suggested block",
  mention: "Mention",
  reply: "Reply",
  run_failed: "Run failed",
}

function field(item: InboxItem, key: string): unknown {
  return (item.data as Record<string, unknown>)[key]
}

function textField(item: InboxItem, key: string): string | null {
  const value = field(item, key)
  return typeof value === "string" && value !== "" ? value : null
}

type ObjectRef = { kind: TargetKind; id: string }

function objectRef(item: InboxItem): ObjectRef | null {
  const value = field(item, "object")
  if (typeof value !== "object" || value === null) return null
  const { kind, id } = value as Record<string, unknown>
  if (typeof kind !== "string" || typeof id !== "string") return null
  return Object.hasOwn(kindWords, kind)
    ? { kind: kind as TargetKind, id }
    : null
}

function threadObject(item: InboxItem): string {
  return kindWords[objectRef(item)?.kind ?? item.target.kind]
}

function fallbackVerb(type: string): string {
  const action = type.slice(type.indexOf(".") + 1)
  return action.replace(/_/g, " ")
}

export function itemText(item: InboxItem): ItemText {
  const actor = item.actor.label
  switch (item.type) {
    case "proposal.opened":
      return { actor, verb: "proposed a change to", object: "a dashboard" }
    case "observation.block_added":
      return {
        actor,
        verb:
          field(item, "state") === "requested"
            ? "suggested a block on"
            : "added a block to",
        object: textField(item, "title") ?? "an observation",
      }
    case "thread.mentioned":
      return { actor, verb: "mentioned you on", object: threadObject(item) }
    case "thread.replied":
      return { actor, verb: "replied on", object: threadObject(item) }
    case "run.failed":
      return {
        actor: "",
        verb: "Your run failed on",
        object: kindWords[item.target.kind],
      }
    default:
      return {
        actor,
        verb: fallbackVerb(item.type),
        object: textField(item, "title") ?? kindWords[item.target.kind],
      }
  }
}

function threadHref(item: InboxItem): string | null {
  const ref = objectRef(item)
  if (ref === null) return null
  switch (ref.kind) {
    case "dashboard":
      return dashboardPath(ref.id)
    case "observation":
      return observationPath(ref.id)
    case "proposal":
      return proposalPath(ref.id)
    case "finding":
      return findingPath(ref.id)
    case "annotation":
      return item.dashboard_id === null
        ? null
        : dashboardPath(item.dashboard_id)
    default:
      return null
  }
}

export function itemHref(item: InboxItem): string | null {
  const { kind, id } = item.target
  switch (kind) {
    case "proposal":
      return proposalPath(id)
    case "observation":
      return observationPath(id)
    case "run":
      return chatPath(id)
    case "finding":
      return findingPath(id)
    case "dashboard":
      return dashboardPath(id, textField(item, "title") ?? undefined)
    case "thread":
      return threadHref(item)
    case "annotation":
      return item.dashboard_id === null
        ? null
        : dashboardPath(item.dashboard_id)
    default:
      return null
  }
}

export function reasonLabel(reason: InboxReason): string {
  return reasonLabels[reason]
}

export function groupItems(items: readonly InboxItem[]): ItemGroup[] {
  const needs = items.filter((item) => item.reason !== null)
  const rest = items.filter((item) => item.reason === null)
  const groups: ItemGroup[] = [
    { label: "Needs you", items: needs },
    { label: "Workspace activity", items: rest },
  ]
  return groups.filter((group) => group.items.length > 0)
}

function observationOf(item: InboxItem): string | null {
  if (item.target.kind === "observation") return item.target.id
  const ref = objectRef(item)
  return item.target.kind === "thread" && ref?.kind === "observation"
    ? ref.id
    : null
}

export function unreadObservationIds(items: readonly InboxItem[]): Set<string> {
  const ids = new Set<string>()
  for (const item of items) {
    const id = item.unread ? observationOf(item) : null
    if (id !== null) ids.add(id)
  }
  return ids
}
