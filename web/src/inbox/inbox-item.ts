import type { components } from "@/api/schema.gen"
import { versionChangesSearch } from "@/history/search"
import {
  chatPath,
  dashboardPath,
  findingPath,
  folderPath,
  observationPath,
  proposalPath,
} from "@/lib/links"
import { plainTitle } from "@/comments/plain-title"

export type InboxItem = components["schemas"]["InboxItem"]
export type InboxReason = NonNullable<InboxItem["reason"]>
type TargetKind = InboxItem["target"]["kind"]

export type ItemText = { actor: string; verb: string; object: string }
export type ItemGroup = {
  key: string
  label: string
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
  user: "a person",
  service_account: "a service account",
  webhook: "a webhook",
  mcp_client: "an agent connection",
  outbound_domain: "an outbound domain",
  grafana_connection: "a Grafana connection",
  datasource: "a datasource",
  public_link: "a public link",
  access_request: "an access request",
}

const reasonLabels: Record<InboxReason, string> = {
  proposal: "Proposal",
  requested_block: "Suggested block",
  mention: "Mention",
  reply: "Reply",
  run_failed: "Run failed",
  repaired: "Repaired",
  proposal_closed: "Proposal closed",
  access_request: "Access request",
}

function field(item: InboxItem, key: string): unknown {
  return (item.data as Record<string, unknown>)[key]
}

function textField(item: InboxItem, key: string): string | null {
  const value = field(item, key)
  return typeof value === "string" && value !== "" ? value : null
}

function countField(item: InboxItem, key: string): number | null {
  const value = field(item, key)
  return typeof value === "number" && Number.isInteger(value) && value > 0
    ? value
    : null
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

function titleOf(item: InboxItem): string | null {
  return item.target_title === null || item.target_title === ""
    ? null
    : plainTitle(item.target_title)
}

const commentable = new Set<TargetKind>([
  "dashboard",
  "annotation",
  "observation",
  "proposal",
])

export function threadTarget(item: InboxItem): string | null {
  if (item.target.kind !== "thread") return null
  const ref = objectRef(item)
  return ref !== null && commentable.has(ref.kind)
    ? `${ref.kind}:${ref.id}`
    : null
}

const builtInAgentName = "Observe Agent"

function answeringActor(item: InboxItem): string {
  return item.actor.run_id === null ? item.actor.label : builtInAgentName
}

function repliedText(item: InboxItem): ItemText {
  if (item.question !== null) {
    return {
      actor: answeringActor(item),
      verb: "answered",
      object: item.question,
    }
  }
  if (item.actor.run_id !== null) {
    return {
      actor: builtInAgentName,
      verb: "answered on",
      object: threadObject(item),
    }
  }
  return {
    actor: item.actor.label,
    verb: "replied on",
    object: threadObject(item),
  }
}

function threadObject(item: InboxItem): string {
  return titleOf(item) ?? kindWords[objectRef(item)?.kind ?? item.target.kind]
}

export function itemHint(item: InboxItem): string | null {
  if (!inGeneral(item)) return null
  const where = item.actor.kind === "service_account" ? "API keys" : "People"
  return `General is open to everyone by role and has no list to change. Raise ${item.actor.name}'s role on ${where}, or move it to a folder first.`
}

export function itemExcerpt(item: InboxItem): string | null {
  return item.excerpt === null || item.excerpt === "" ? null : item.excerpt
}

function fallbackVerb(type: string): string {
  const action = type.slice(type.indexOf(".") + 1)
  return action.replace(/_/g, " ")
}

function repairedText(item: InboxItem): ItemText {
  const title = textField(item, "dashboard_title")
  const panels = countField(item, "panels_repaired")
  if (title !== null && panels !== null) {
    return {
      actor: item.actor.label,
      verb: `repaired ${panels} ${panels === 1 ? "panel" : "panels"} on`,
      object: title,
    }
  }
  const version = countField(item, "result_version")
  return {
    actor: "",
    verb: "Your repair saved",
    object: version === null ? "a version" : `version ${version}`,
  }
}

function closedProposalText(item: InboxItem): ItemText {
  const merged = item.type === "proposal.merged"
  const title = textField(item, "title") ?? titleOf(item) ?? "a proposal"
  const version = countField(item, "version")
  return {
    actor: item.actor.label,
    verb: merged ? "merged your proposal" : "discarded your proposal",
    object:
      merged && version !== null ? `${title} as version ${version}` : title,
  }
}

export function itemText(item: InboxItem): ItemText {
  const actor = item.actor.label
  if (item.type === "run.succeeded" && item.reason === "repaired") {
    return repairedText(item)
  }
  switch (item.type) {
    case "proposal.opened": {
      const title = textField(item, "title") ?? titleOf(item)
      return title === null
        ? { actor, verb: "proposed a change to", object: "a dashboard" }
        : { actor, verb: "proposed", object: title }
    }
    case "observation.block_added":
      return {
        actor,
        verb:
          field(item, "state") === "requested"
            ? "suggested a block on"
            : "added a block to",
        object: textField(item, "title") ?? titleOf(item) ?? "an observation",
      }
    case "proposal.merged":
    case "proposal.discarded":
      return closedProposalText(item)
    case "thread.mentioned":
      return { actor, verb: "mentioned you on", object: threadObject(item) }
    case "thread.replied":
      return repliedText(item)
    case "access_request.opened":
      return {
        actor,
        verb: "asked for comment access to",
        object:
          textField(item, "title") ??
          titleOf(item) ??
          kindWords[objectRef(item)?.kind ?? "dashboard"],
      }
    case "run.failed":
      return {
        actor: "",
        verb: "Your run failed on",
        object: titleOf(item) ?? kindWords[item.target.kind],
      }
    default:
      return {
        actor,
        verb: fallbackVerb(item.type),
        object:
          textField(item, "title") ??
          titleOf(item) ??
          kindWords[item.target.kind],
      }
  }
}

export type ItemLink = {
  to: string
  search?:
    | { thread: string }
    | { from?: number; to: number }
    | { share: "1" }
    | { settings: "1" }
}

function page(to: string): ItemLink {
  return { to }
}

function threadLink(item: InboxItem): ItemLink | null {
  const ref = objectRef(item)
  if (ref === null) return null
  switch (ref.kind) {
    case "dashboard":
      return { to: dashboardPath(ref.id), search: { thread: item.target.id } }
    case "observation":
      return page(observationPath(ref.id))
    case "proposal":
      return page(proposalPath(ref.id))
    case "finding":
      return page(findingPath(ref.id))
    case "annotation":
      return item.dashboard_id === null
        ? null
        : page(dashboardPath(item.dashboard_id))
    default:
      return null
  }
}

export function inGeneral(item: InboxItem): boolean {
  return field(item, "general") === true
}

function accessRequestLink(item: InboxItem): ItemLink | null {
  if (inGeneral(item)) {
    return { to: item.actor.kind === "service_account" ? "/keys" : "/people" }
  }
  const ref = objectRef(item)
  if (ref?.kind === "dashboard") {
    return { to: dashboardPath(ref.id), search: { share: "1" } }
  }
  return item.folder_id === null
    ? null
    : { to: folderPath(item.folder_id), search: { settings: "1" } }
}

function repairedLink(item: InboxItem): ItemLink | null {
  if (item.dashboard_id === null) return null
  const version = countField(item, "result_version")
  if (version === null) return page(dashboardPath(item.dashboard_id))
  return {
    to: `${dashboardPath(item.dashboard_id)}/versions`,
    search: versionChangesSearch(version),
  }
}

export function itemLink(item: InboxItem): ItemLink | null {
  if (item.type === "run.succeeded" && item.reason === "repaired") {
    return repairedLink(item)
  }
  const { kind, id } = item.target
  switch (kind) {
    case "proposal":
      return page(proposalPath(id))
    case "observation":
      return page(observationPath(id))
    case "run":
      return page(chatPath(id))
    case "finding":
      return page(findingPath(id))
    case "dashboard":
      return page(dashboardPath(id, textField(item, "title") ?? undefined))
    case "thread":
      return threadLink(item)
    case "access_request":
      return accessRequestLink(item)
    case "annotation":
      return item.dashboard_id === null
        ? null
        : page(dashboardPath(item.dashboard_id))
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
    { key: "needs", label: "Needs you", items: needs },
    { key: "activity", label: "Workspace activity", items: rest },
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
