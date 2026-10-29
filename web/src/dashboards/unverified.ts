import type { components } from "@/api/schema.gen"

type Issues = { issues: components["schemas"]["Issue"][] }

export type UnverifiedReasons = {
  panels: Map<string, string>
  variables: Map<string, string>
}

function firstMessages<T extends Issues>(
  items: readonly T[],
  key: (item: T) => string
): Map<string, string> {
  const reasons = new Map<string, string>()
  for (const item of items) {
    if (item.issues[0]) reasons.set(key(item), item.issues[0].message)
  }
  return reasons
}

export function unverifiedReasons(
  unverified: components["schemas"]["Unverified"],
  listedVersion: number,
  drawnVersion: number
): UnverifiedReasons {
  if (listedVersion !== drawnVersion) {
    return { panels: new Map(), variables: new Map() }
  }
  return {
    panels: firstMessages(unverified.panels, (panel) => panel.panel_id),
    variables: firstMessages(unverified.variables, (variable) => variable.name),
  }
}
