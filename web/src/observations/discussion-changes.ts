import type { DiscussionChange } from "@/discussion/discussion-model"

import { blockTitle } from "./block-rows"
import type { Block, Observation } from "./queries"

export function blockSubject(block: Block): string {
  const title = blockTitle(block)
  return block.kind === "panel" && title !== "Panel" ? `Panel: ${title}` : title
}

export function observationChanges(
  observation: Observation
): DiscussionChange[] {
  const started: DiscussionChange = {
    key: "started",
    at: observation.created_at,
    actor: observation.created_by,
    verb: "started this observation",
    subject: null,
  }
  const added = (observation.blocks ?? []).map((block): DiscussionChange => ({
    key: block.id,
    at: block.created_at,
    actor: block.created_by,
    verb: block.state === "requested" ? "suggested a block" : "added a block",
    subject: { id: block.id, title: blockSubject(block) },
  }))
  return [started, ...added]
}

export function discussionToggleLabel(count: number): string {
  if (count === 0) return "Discussion"
  return `Discussion, ${count} ${count === 1 ? "message" : "messages"}`
}
