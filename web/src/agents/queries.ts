import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { agentKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type AgentsOverview = components["schemas"]["AgentsOverview"]
export type ConnectedRow = AgentsOverview["connected"][number]
export type ServiceAccountRow = AgentsOverview["service_accounts"][number]

export function agentsQuery() {
  return queryOptions({
    queryKey: agentKeys.list(),
    queryFn: ({ signal }) => api.GET("/agents", { signal }).then(unwrap),
    staleTime: 30_000,
  })
}
