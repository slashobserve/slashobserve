import { createContext, useContext, useMemo } from "react"
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { datasourceKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type DatasourceStatus = components["schemas"]["DatasourceStatus"]

export type DatasourceRef = Pick<DatasourceStatus, "uid" | "type">

const ProvidedDatasources = createContext<readonly DatasourceRef[] | undefined>(
  undefined
)

export const DatasourceListProvider = ProvidedDatasources.Provider

export function datasourcesQuery() {
  return queryOptions({
    queryKey: datasourceKeys.list(),
    queryFn: ({ signal }) => api.GET("/datasources", { signal }).then(unwrap),
    select: (data) => data.datasources,
  })
}

type DatasourceList = components["schemas"]["DatasourceList"]

export function useRecheckDatasource() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (uid: string) =>
      api
        .POST("/datasources/{uid}/recheck", { params: { path: { uid } } })
        .then(unwrap),
    onSuccess: (status) =>
      queryClient.setQueryData<DatasourceList>(datasourceKeys.list(), (list) =>
        list === undefined
          ? list
          : {
              datasources: list.datasources.map((datasource) =>
                datasource.uid === status.uid ? status : datasource
              ),
            }
      ),
  })
}

const unhealthyPollMs = 15_000

export function pollWhileUnhealthy(
  list: DatasourceList | undefined
): number | false {
  return list?.datasources?.some((datasource) => !datasource.healthy)
    ? unhealthyPollMs
    : false
}

export function useDatasources() {
  const provided = useContext(ProvidedDatasources)
  const query = useQuery({
    ...datasourcesQuery(),
    staleTime: 60_000,
    refetchInterval: (current) => pollWhileUnhealthy(current.state.data),
    enabled: provided === undefined,
  })
  const listed = useMemo(() => provided?.map(listedOnly), [provided])
  return listed === undefined ? query : { ...query, data: listed }
}

function listedOnly(ref: DatasourceRef): DatasourceStatus {
  return {
    uid: ref.uid,
    type: ref.type,
    name: ref.uid,
    source: "file",
    healthy: true,
    health_state: "healthy",
    checked_at: "",
    dashboard_count: 0,
  }
}

export function datasourceTypeOf(
  list: readonly DatasourceRef[] | undefined,
  uid: string
): string | undefined {
  return list?.find((datasource) => datasource.uid === uid)?.type
}
