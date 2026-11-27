import { queryOptions, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { followCursor } from "@/lib/follow-cursor"

export type Revision = components["schemas"]["Revision"]
export type RevisionStatusValue = Revision["status"]

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
      followCursor((cursor) =>
        api
          .GET("/dashboards/{id}/revisions", {
            params: { path: { id }, query: { limit: pageSize, cursor } },
            signal,
          })
          .then(unwrap)
          .then((body) => ({
            items: body.revisions,
            next: body.next_cursor ?? null,
          }))
      ),
    refetchInterval: (query) =>
      query.state.data?.some((revision) => isActive(revision.status))
        ? safetyPollMs
        : false,
  })
}

export function useRevisions(id: string) {
  return useQuery(revisionsQuery(id))
}
