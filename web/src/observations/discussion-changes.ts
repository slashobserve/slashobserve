import type { DiscussionChange } from "@/discussion/discussion-model"
import type { ActivityEvent } from "@/live/activity-match"
import { isDataPanel } from "@/spec"

import { blockTitle } from "./block-rows"
import {
  datasourceUidOf,
  panelBlockBody,
  snapshotIdOf,
} from "./blocks/block-data"
import type { Block, Observation } from "./queries"

type ChangeContext = {
  folderTitle: string
  datasources: readonly { uid: string; name: string }[]
  events: readonly ActivityEvent[]
  snapshotTitles?: ReadonlyMap<string, string>
}

export const changeEventTypes = [
  "observation.created",
  "observation.updated",
  "observation.moved",
  "observation.resolved",
  "observation.reopened",
  "observation.published",
  "observation.rca_withdrawn",
  "observation.block_updated",
  "observation.block_removed",
] as const

export function blockSubject(
  block: Block,
  snapshotTitles?: ReadonlyMap<string, string>
): string {
  const title = blockTitle(block)
  if (block.kind === "panel" && title !== "Panel") return `Panel: ${title}`
  if (block.kind === "snapshot") {
    const frozen = snapshotTitles?.get(snapshotIdOf(block) ?? "")
    if (frozen !== undefined) return `Snapshot: ${frozen}`
  }
  return title
}

function panelLine(
  block: Block,
  datasources: ChangeContext["datasources"]
): string | null {
  if (block.kind !== "panel") return null
  const body = panelBlockBody(block)
  if (body === null) return null
  const uid = isDataPanel(body.panel) ? datasourceUidOf(body.panel) : undefined
  if (uid === undefined) return "live"
  const name = datasources.find((entry) => entry.uid === uid)?.name ?? uid
  return `${name}, live`
}

function eventTitle(event: ActivityEvent): string | null {
  const { title } = event.data as { title?: unknown }
  return typeof title === "string" ? title : null
}

function eventFromTitle(event: ActivityEvent): string | null {
  const { from_title } = event.data as { from_title?: unknown }
  return typeof from_title === "string" ? from_title : null
}

function originalTitle(
  observation: Observation,
  events: readonly ActivityEvent[]
): string {
  const ordered = events.toSorted((a, b) => a.seq - b.seq)
  const created = ordered.find((event) => event.type === "observation.created")
  const first = created === undefined ? null : eventTitle(created)
  if (first !== null) return first
  for (const event of ordered) {
    const from = eventFromTitle(event)
    if (from !== null) return from
  }
  return observation.title
}

function startedDetail(title: string, folderTitle: string) {
  return `${title} · In ${folderTitle}`
}

function eventRevision(event: ActivityEvent): string {
  const { revision } = event.data as { revision?: unknown }
  return typeof revision === "number" ? ` revision ${revision}` : ""
}

function eventBlockId(event: ActivityEvent): string | null {
  const { block_id } = event.data as { block_id?: unknown }
  return typeof block_id === "string" ? block_id : null
}

const plainVerbs: Record<string, (event: ActivityEvent) => string> = {
  "observation.moved": () => "moved this observation to another folder",
  "observation.resolved": () => "resolved this observation",
  "observation.reopened": () => "reopened this observation",
  "observation.published": (event) =>
    `published${eventRevision(event) || " a revision"} as an RCA`,
  "observation.rca_withdrawn": (event) =>
    `withdrew${eventRevision(event) || " a revision"} of the RCA`,
  "observation.block_removed": () => "removed a block",
}

function eventChanges(
  events: readonly ActivityEvent[],
  blocks: readonly Block[],
  snapshotTitles?: ReadonlyMap<string, string>
): DiscussionChange[] {
  const changes: DiscussionChange[] = []
  let title: string | null = null
  for (const event of events.toSorted((a, b) => a.seq - b.seq)) {
    const base = {
      key: `event:${event.seq}`,
      at: event.created_at,
      actor: event.actor,
      subject: null,
      detail: null,
    }
    const plain = plainVerbs[event.type]
    if (plain !== undefined) {
      changes.push({ ...base, verb: plain(event) })
      continue
    }
    if (event.type === "observation.block_updated") {
      const block = blocks.find((entry) => entry.id === eventBlockId(event))
      changes.push({
        ...base,
        verb: "changed a block",
        subject:
          block === undefined
            ? null
            : { id: block.id, title: blockSubject(block, snapshotTitles) },
      })
      continue
    }
    const next = eventTitle(event)
    const from = eventFromTitle(event)
    if (
      event.type === "observation.updated" &&
      next !== null &&
      from !== null &&
      next !== from
    ) {
      changes.push({
        ...base,
        verb: `renamed this observation from ${from} to ${next}`,
      })
    } else if (
      event.type === "observation.updated" &&
      next !== null &&
      title !== null &&
      next !== title
    ) {
      changes.push({ ...base, verb: `renamed this observation to ${next}` })
    }
    if (next !== null) title = next
  }
  return changes
}

export function observationChanges(
  observation: Observation,
  { folderTitle, datasources, events, snapshotTitles }: ChangeContext
): DiscussionChange[] {
  const started: DiscussionChange = {
    key: "started",
    at: observation.created_at,
    actor: observation.created_by,
    verb: "started this observation",
    subject: null,
    detail: startedDetail(originalTitle(observation, events), folderTitle),
  }
  const added = (observation.blocks ?? []).map((block): DiscussionChange => ({
    key: block.id,
    at: block.created_at,
    actor: block.created_by,
    verb: block.state === "requested" ? "suggested a block" : "added a block",
    subject: { id: block.id, title: blockSubject(block, snapshotTitles) },
    detail: panelLine(block, datasources),
  }))
  return [
    started,
    ...added,
    ...eventChanges(events, observation.blocks ?? [], snapshotTitles),
  ]
}

export function discussionToggleLabel(count: number): string {
  if (count === 0) return "Discussion"
  return `Discussion, ${count} ${count === 1 ? "message" : "messages"}`
}
