import type { Selections } from "@/api/keys"
import { withVariables, type DashboardSearchParams } from "@/dashboards/search"
import { displayWithDefaults } from "@/spec"

const maxLimit = 100

export type DashlistOptions = {
  query: string
  tags: string[]
  limit: number
  keepTime: boolean
  includeVars: boolean
}

export function dashlistOptions(display: unknown): DashlistOptions {
  const given = displayWithDefaults("dashlist", display)
  const limit = typeof given.limit === "number" ? Math.trunc(given.limit) : 10
  return {
    query: typeof given.query === "string" ? given.query.trim() : "",
    tags: Array.isArray(given.tags)
      ? given.tags.filter((tag): tag is string => typeof tag === "string")
      : [],
    limit: Math.min(maxLimit, Math.max(1, limit)),
    keepTime: given.keepTime === true,
    includeVars: given.includeVars === true,
  }
}

export function dashlistSearch(
  options: Pick<DashlistOptions, "keepTime" | "includeVars">,
  range: { from: string; to: string },
  vars: Selections
): DashboardSearchParams {
  const base: DashboardSearchParams = options.keepTime
    ? { from: range.from, to: range.to }
    : {}
  return options.includeVars ? withVariables(base, vars) : base
}
