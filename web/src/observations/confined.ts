import type { components } from "@/api/schema.gen"

export type Confined = components["schemas"]["Confined"]

const confinedNotes: Record<Confined["reason"], string> = {
  permanent:
    "This observation holds an answer that read data only Admins can query, so it can only move to a folder only Admins can read.",
  run_in_flight:
    "An agent run on this observation has not finished. Until it ends, the observation can only move to a folder only Admins can read.",
}

export function confinedNote(confined: Confined | undefined): string | null {
  if (confined === undefined) return null
  return confinedNotes[confined.reason] ?? null
}
