import type { QueryClient } from "@tanstack/react-query"

import { datasourcesQuery } from "@/datasources/queries"
import { agentInfoQuery } from "@/observe/queries"

export async function preloadLogsSource(queryClient: QueryClient) {
  await Promise.allSettled([
    queryClient.ensureQueryData(datasourcesQuery()),
    queryClient.ensureQueryData(agentInfoQuery()),
  ])
}
