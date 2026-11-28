import { infiniteQueryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { searchKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type SearchKind = components["schemas"]["SearchResult"]["kind"]

export const searchKinds: readonly { kind: SearchKind; label: string }[] = [
  { kind: "dashboard", label: "Dashboards" },
  { kind: "observation", label: "Observations" },
  { kind: "comment", label: "Comments" },
  { kind: "annotation", label: "Annotations" },
  { kind: "saved_search", label: "Saved searches" },
]

export function parseSearchKind(value: unknown): SearchKind | undefined {
  return searchKinds.find((entry) => entry.kind === value)?.kind
}

export function searchPageQuery(q: string, kind: SearchKind | null) {
  return infiniteQueryOptions({
    queryKey: searchKeys.page(q, kind),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/search", {
          params: {
            query: {
              q,
              ...(kind === null ? {} : { kind: [kind] }),
              ...(pageParam === undefined ? {} : { cursor: pageParam }),
              limit: 20,
            },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    retry: false,
    staleTime: 15_000,
  })
}
