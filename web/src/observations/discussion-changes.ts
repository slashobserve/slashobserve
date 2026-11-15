import type { DiscussionChange } from "@/discussion/discussion-model"
import type { ActivityEvent } from "@/live/activity-match"
import { isDataPanel } from "@/spec"

import { blockTitle } from "./block-rows"
import { datasourceUidOf, panelBlockBody } from "./blocks/block-data"
import type { Block, Observation } from "./queries"

type ChangeContext = {
  folderTitle: string
  datasources: readonly { uid: string; name: string }[]
  events: readonly ActivityEvent[]
}

export const changeEventTypes = [
  "observation.created",
  "observation.updated",
  "observation.block_removed",
] as const

export function blockSubject(block: Block): string {
  const title = blockTitle(block)
  return block.kind === "panel" && title !== "Panel" ? `Panel: ${title}` : title
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

function startedDetail(observation: Observation, folderTitle: string): string {
  const state = observation.state === "open" ? "Open" : "Resolved"
  return `${observation.title} · In ${folderTitle} · ${state}`
}

function eventChanges(events: readonly ActivityEvent[]): DiscussionChange[] {
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
    if (event.type === "observation.block_removed") {
      changes.push({ ...base, verb: "removed a block" })
      continue
    }
    const next = eventTitle(event)
    if (
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
  { folderTitle, datasources, events }: ChangeContext
): DiscussionChange[] {
  const started: DiscussionChange = {
    key: "started",
    at: observation.created_at,
    actor: observation.created_by,
    verb: "started this observation",
    subject: null,
    detail: startedDetail(observation, folderTitle),
  }
  const added = (observation.blocks ?? []).map((block): DiscussionChange => ({
    key: block.id,
    at: block.created_at,
    actor: block.created_by,
    verb: block.state === "requested" ? "suggested a block" : "added a block",
    subject: { id: block.id, title: blockSubject(block) },
    detail: panelLine(block, datasources),
  }))
  return [started, ...added, ...eventChanges(events)]
}

export function discussionToggleLabel(count: number): string {
  if (count === 0) return "Discussion"
  return `Discussion, ${count} ${count === 1 ? "message" : "messages"}`
}
