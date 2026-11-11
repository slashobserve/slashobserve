import type { components } from "@/api/schema.gen"
import type { Block, Observation } from "@/observations/queries"

export type ActivityEvent = components["schemas"]["ActivityEvent"]

export function touchesObservation(
  event: ActivityEvent,
  observation: Pick<Observation, "id" | "thread_id"> & {
    blocks?: readonly Pick<Block, "run_id">[]
  }
): "observation" | "discussion" | null {
  const { kind, id } = event.target
  if (kind === "observation" && id === observation.id) return "observation"
  if (
    kind === "run" &&
    (observation.blocks ?? []).some((block) => block.run_id === id)
  ) {
    return "observation"
  }
  if (kind === "thread" && id === observation.thread_id) return "discussion"
  return null
}
