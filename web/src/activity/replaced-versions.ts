import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

type SavedEvent = Pick<components["schemas"]["ActivityEvent"], "data">

const newestSaves = 100

function versionNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isInteger(value) && value >= 1
    ? value
    : null
}

export function replacedVersions(
  events: readonly SavedEvent[]
): Map<number, number> {
  const replaced = new Map<number, number>()
  for (const event of events) {
    const data = event.data as Record<string, unknown>
    const version = versionNumber(data.version)
    const before = versionNumber(data.replaced_version)
    if (version !== null && before !== null) replaced.set(version, before)
  }
  return replaced
}

export function replacedVersionsQuery(dashboardId: string) {
  return queryOptions({
    queryKey: dashboardKeys.replacedVersions(dashboardId),
    queryFn: ({ signal }) =>
      api
        .GET("/activity", {
          params: {
            query: {
              dashboard_id: dashboardId,
              type: ["dashboard.version_created"],
              limit: newestSaves,
            },
          },
          signal,
        })
        .then(unwrap)
        .then((body) => replacedVersions(body.activity)),
    staleTime: 15_000,
  })
}
