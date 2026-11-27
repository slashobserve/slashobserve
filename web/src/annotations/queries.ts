import { useMemo, useSyncExternalStore } from "react"
import {
  queryOptions,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { annotationKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import type { PanelSource } from "@/dashboards/panel-source"
import { followCursor } from "@/lib/follow-cursor"
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

function cachedTags(queryClient: QueryClient, dashboardId: string): string {
  const tags = queryClient
    .getQueriesData<readonly Annotation[]>({
      queryKey: annotationKeys.ranges(dashboardId),
    })
    .flatMap(([, list]) => list?.flatMap((a) => a.tags) ?? [])
  return JSON.stringify([...new Set(tags)].sort())
}

const activityLimit = 50

export function annotationActivityQuery(dashboardId: string) {
  return queryOptions({
    queryKey: annotationKeys.activity(dashboardId),
    queryFn: ({ signal }) =>
      followCursor((cursor) =>
        api
          .GET("/annotations", {
            params: {
              query: { dashboard_id: dashboardId, limit: activityLimit, cursor },
            },
            signal,
          })
          .then(unwrap)
          .then((body) => ({
            items: body.annotations.filter(
              (annotation) => annotation.dashboard_id === dashboardId
            ),
            next: body.next_cursor ?? null,
          }))
      ),
    staleTime: 15_000,
  })
}

export function useKnownAnnotationTags(dashboardId: string): string[] {
  const queryClient = useQueryClient()
  const tags = useSyncExternalStore(
    (notify) => queryClient.getQueryCache().subscribe(notify),
    () => cachedTags(queryClient, dashboardId)
  )
  return useMemo(() => JSON.parse(tags) as string[], [tags])
}
