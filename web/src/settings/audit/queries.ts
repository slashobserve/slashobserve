import { infiniteQueryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { auditKeys } from "@/api/keys"

import { rangeSince, type ActivityType, type AuditRange } from "./audit-events"

const pageSize = 50

export type AuditFilters = {
  actor: string | null
  type: ActivityType | null
  range: AuditRange
}

export function auditInfiniteQuery(filters: AuditFilters) {
  return infiniteQueryOptions({
    queryKey: auditKeys.list(filters.actor, filters.type, filters.range),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/activity", {
          params: {
            query: {
              scope: "admin",
              actor_id: filters.actor ?? undefined,
              type: filters.type === null ? undefined : [filters.type],
              since: rangeSince(filters.range, Date.now()),
              cursor: pageParam,
              limit: pageSize,
            },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  })
}
