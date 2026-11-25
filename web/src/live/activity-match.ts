import type { components } from "@/api/schema.gen"
import type { Block, Observation } from "@/observations/queries"

export type ActivityEvent = components["schemas"]["ActivityEvent"]

export type ActivityFilter = {
  folderId?: string | null
  types?: readonly string[]
}

export function matchesActivityFilter(
  event: ActivityEvent,
  { folderId, types }: ActivityFilter
): boolean {
  if (folderId && event.folder_id !== folderId) return false
  return types === undefined || types.length === 0 || types.includes(event.type)
}

type Touch = "observation" | "discussion" | "both"

function namesObservation(event: ActivityEvent, id: string): boolean {
  const object = (event.data as { object?: { kind?: unknown; id?: unknown } })
    .object
  return object?.kind === "observation" && object.id === id
}

function discussionTouch(event: ActivityEvent): Touch {
  return event.actor.run_id === null ? "discussion" : "both"
}

export function touchesObservation(
  event: ActivityEvent,
  observation: Pick<Observation, "id" | "thread_id"> & {
    blocks?: readonly Pick<Block, "run_id">[]
  }
): Touch | null {
  const { kind, id } = event.target
  if (kind === "observation" && id === observation.id) return "observation"
  if (
    kind === "run" &&
    (observation.blocks ?? []).some((block) => block.run_id === id)
  ) {
    return "observation"
  }
  if (
    kind === "thread" &&
    (id === observation.thread_id || namesObservation(event, observation.id))
  ) {
    return discussionTouch(event)
  }
  return null
}
