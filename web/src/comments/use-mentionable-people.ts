import { useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { threadKeys } from "@/api/keys"

import type { MentionPerson } from "./mentions"

export type MentionablePeople = {
  people: MentionPerson[]
  searching: boolean
}

export function useMentionablePeople(
  target: string | null,
  term: string,
  enabled: boolean
): MentionablePeople {
  const query = useQuery({
    queryKey: [...threadKeys.all, "mentionable", target, term] as const,
    queryFn: ({ signal }) =>
      api
        .GET("/threads/mentionable", {
          params: { query: { target: target ?? "", q: term } },
          signal,
        })
        .then(unwrap),
    enabled: enabled && target !== null,
    staleTime: 30_000,
    placeholderData: (previous) => previous,
  })
  return {
    people: query.data?.people ?? [],
    searching: query.isFetching,
  }
}
