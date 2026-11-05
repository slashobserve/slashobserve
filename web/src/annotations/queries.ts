import { queryOptions, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { annotationKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import type { PanelSource } from "@/dashboards/panel-source"
import { resolveTime, timeOptionsFor } from "@/lib/time"

export type Annotation = components["schemas"]["Annotation"]

const pageLimit = 500

function resolvedRange(from: string, to: string, timezone: string | undefined) {
  const options = timeOptionsFor({ timezone })
  const now = Date.now()
  return {
    fromMs: resolveTime(from, now, false, options),
    toMs: resolveTime(to, now, true, options),
  }
}

function validRange(range: { fromMs: number; toMs: number }): boolean {
  return (
    Number.isFinite(range.fromMs) &&
    Number.isFinite(range.toMs) &&
    range.fromMs <= range.toMs
  )
}

function annotationsQuery(
  dashboardId: string,
  from: string,
  to: string,
  timezone: string | undefined
) {
  return queryOptions({
    queryKey: annotationKeys.range(dashboardId, from, to),
    queryFn: ({ signal }) => {
      const range = resolvedRange(from, to, timezone)
      return api
        .GET("/annotations", {
          params: {
            query: {
              dashboard_id: dashboardId,
              from: new Date(range.fromMs).toISOString(),
              to: new Date(range.toMs).toISOString(),
              limit: pageLimit,
            },
          },
          signal,
        })
        .then(unwrap)
        .then((body) => body.annotations)
    },
    staleTime: 30_000,
  })
}

export function useDashboardAnnotations({
  source,
  from,
  to,
  timezone,
  refreshMs,
}: {
  source: PanelSource
  from: string
  to: string
  timezone: string | undefined
  refreshMs: number | null | undefined
}): Annotation[] | undefined {
  const dashboardId = source.kind === "version" ? source.dashboardId : ""
  const enabled =
    source.kind === "version" && validRange(resolvedRange(from, to, timezone))
  const query = useQuery({
    ...annotationsQuery(dashboardId, from, to, timezone),
    enabled,
    refetchInterval: refreshMs ?? false,
  })
  return enabled ? query.data : undefined
}

function distinctTags(list: readonly Annotation[]): string[] {
  return [...new Set(list.flatMap((a) => a.tags))].sort()
}

export function annotationTagsQuery(dashboardId: string) {
  return queryOptions({
    queryKey: annotationKeys.tags(dashboardId),
    queryFn: ({ signal }) =>
      api
        .GET("/annotations", {
          params: { query: { dashboard_id: dashboardId, limit: pageLimit } },
          signal,
        })
        .then(unwrap)
        .then((body) => distinctTags(body.annotations)),
    staleTime: 60_000,
  })
}
