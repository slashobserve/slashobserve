import { queryOptions, useQueries } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { datasourceKeys } from "@/api/keys"
import { panelRequestLimiter } from "@/dashboards/limiter"

import type { DatasourceStatus } from "./queries"
import {
  servicesOfResponse,
  type ServiceSignal,
  type ServicesState,
} from "./services"

function servicesQuery(uid: string, signal: ServiceSignal) {
  return queryOptions({
    queryKey: datasourceKeys.services(uid, signal),
    queryFn: ({ signal: aborted }) =>
      panelRequestLimiter.run(
        () =>
          api
            .GET("/datasources/{uid}/services", {
              params: { path: { uid }, query: { signal } },
              signal: aborted,
            })
            .then(unwrap),
        aborted
      ),
    staleTime: 60_000,
    retry: false,
  })
}

export function useSourceServices(
  sources: readonly DatasourceStatus[],
  signal: ServiceSignal | null
): ReadonlyMap<string, ServicesState> {
  const results = useQueries({
    queries: sources.map((source) => ({
      ...servicesQuery(source.uid, signal ?? "logs"),
      enabled: signal !== null,
    })),
  })
  const states = new Map<string, ServicesState>()
  sources.forEach((source, index) => {
    const result = results[index]
    if (result === undefined) return
    states.set(
      source.uid,
      result.isError
        ? { status: "unavailable" }
        : result.data === undefined
          ? { status: "loading" }
          : servicesOfResponse(result.data)
    )
  })
  return states
}
