import type { components } from "@/api/schema.gen"
import type { Variable } from "@/spec"

import type { PublicMode } from "./supported"

export type PublicLink = components["schemas"]["PublicLink"]

export type ExpiryChoice = "none" | "1d" | "7d" | "30d" | "date"

export const expiryChoices: { value: ExpiryChoice; label: string }[] = [
  { value: "none", label: "No expiry" },
  { value: "1d", label: "1 day" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
  { value: "date", label: "On a date" },
]

export const modeChoices: { value: PublicMode; label: string }[] = [
  { value: "frozen", label: "Frozen: the values selected now" },
  { value: "free", label: "Free: viewers choose" },
]

const dayMs = 86_400_000
const days: Record<Exclude<ExpiryChoice, "none" | "date">, number> = {
  "1d": 1,
  "7d": 7,
  "30d": 30,
}
const datePattern = /^(\d{4})-(\d{2})-(\d{2})$/

export type Expiry =
  { ok: true; expiresAt: string | null } | { ok: false; problem: string }

export function expiryFor(
  choice: ExpiryChoice,
  date: string,
  now = Date.now()
): Expiry {
  if (choice === "none") return { ok: true, expiresAt: null }
  if (choice !== "date") {
    return {
      ok: true,
      expiresAt: new Date(now + days[choice] * dayMs).toISOString(),
    }
  }
  const match = datePattern.exec(date)
  if (match === null) return { ok: false, problem: "Choose a date." }
  const [year, month, day] = match.slice(1).map(Number)
  const chosen = new Date(year, month - 1, day)
  if (chosen.getMonth() !== month - 1 || chosen.getDate() !== day) {
    return { ok: false, problem: "Choose a date." }
  }
  const endOfDay = new Date(year, month - 1, day + 1)
  if (endOfDay.getTime() <= now) {
    return { ok: false, problem: "Choose today or a later date." }
  }
  return { ok: true, expiresAt: endOfDay.toISOString() }
}

export function linkStatus(
  link: Pick<PublicLink, "expired" | "usable">
): string | null {
  if (link.expired) return "Expired"
  if (!link.usable) return "Not usable: uses the viewer's identity"
  return null
}

export function modeLabel(mode: PublicMode): string {
  return mode === "frozen" ? "Frozen variables" : "Free variables"
}

export function frozenSelections(
  variables: readonly Variable[] | undefined,
  vars: Record<string, string[]>
): Record<string, string[]> {
  const out: Record<string, string[]> = {}
  for (const variable of variables ?? []) {
    const values = vars[variable.name]
    if (values !== undefined && values.length > 0) out[variable.name] = values
  }
  return out
}
