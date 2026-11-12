import { queryOptions, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type Revision = components["schemas"]["Revision"]
export type RevisionStatusValue = Revision["status"]

export const maxQueuedRevisions = 5
export const maxPromptLength = 4000

const pageSize = 50
const safetyPollMs = 5000

export function isActive(status: RevisionStatusValue): boolean {
  return status === "queued" || status === "running"
}

export function revisionsQuery(id: string) {
  return queryOptions({
    queryKey: dashboardKeys.revisions(id),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards/{id}/revisions", {
          params: { path: { id }, query: { limit: pageSize } },
          signal,
        })
        .then(unwrap)
        .then((body) => body.revisions),
    refetchInterval: (query) =>
      query.state.data?.some((revision) => isActive(revision.status))
        ? safetyPollMs
        : false,
  })
}

export function useRevisions(id: string) {
  return useQuery(revisionsQuery(id))
}

export function isAgentUnavailable(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 503 &&
    error.code === "agent_unavailable"
  )
}
