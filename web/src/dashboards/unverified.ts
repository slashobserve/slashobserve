import type { components } from "@/api/schema.gen"

export function unverifiedReasons(
  unverified: components["schemas"]["Unverified"],
  listedVersion: number,
  drawnVersion: number
): Map<string, string> {
  const reasons = new Map<string, string>()
  if (listedVersion !== drawnVersion) return reasons
  for (const { panel_id, issues } of unverified.panels) {
    if (issues[0]) reasons.set(panel_id, issues[0].message)
  }
  return reasons
}
