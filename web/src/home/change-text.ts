import { dayLabel } from "@/activity/day-label"
import type { components } from "@/api/schema.gen"

type ActivityEvent = components["schemas"]["ActivityEvent"]

const changeWords: Partial<Record<ActivityEvent["type"], string>> = {
  "observation.block_added": "added a block",
  "observation.block_removed": "removed a block",
  "observation.block_updated": "changed a block",
  "observation.updated": "renamed it",
  "observation.resolved": "resolved it",
  "observation.reopened": "reopened it",
  "thread.opened": "wrote in the discussion",
  "thread.replied": "wrote in the discussion",
}

function dayWords(iso: string, now: number): string {
  const label = dayLabel(iso, now)
  if (label === "Today" || label === "Yesterday") return label.toLowerCase()
  return label === "" ? "" : `on ${label}`
}

export function changeText(
  event: ActivityEvent,
  me: string,
  now: number
): string | null {
  const words = changeWords[event.type]
  if (words === undefined) return null
  const who = event.actor.id === me ? "you" : event.actor.label
  return [who, words, dayWords(event.created_at, now)]
    .filter((part) => part !== "")
    .join(" ")
}
