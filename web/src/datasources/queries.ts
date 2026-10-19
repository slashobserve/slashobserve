import { queryOptions, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { datasourceKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type DatasourceStatus = components["schemas"]["DatasourceStatus"]

export function datasourcesQuery() {
  return queryOptions({
    queryKey: datasourceKeys.list(),
    queryFn: ({ signal }) => api.GET("/datasources", { signal }).then(unwrap),
    select: (data) => data.datasources,
  })
}

export function useDatasources() {
  return useQuery({
    ...datasourcesQuery(),
    staleTime: 60_000,
  })
}

export function datasourceTypeOf(
  list: DatasourceStatus[] | undefined,
  uid: string
): string | undefined {
  return list?.find((datasource) => datasource.uid === uid)?.type
}
