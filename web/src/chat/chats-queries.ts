import { useMemo } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { chatKeys, observeKeys, type ChatsFilters } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { dashboardTitlesQuery } from "@/shell/palette-queries"

export type Chat = components["schemas"]["Chat"]

const activePollMs = 5000

function isLive(state: Chat["last_turn"]): boolean {
  return (
    state !== null &&
    (state.state === "queued" ||
      state.state === "running" ||
      state.state === "canceling")
  )
}

function chatsQuery(filters: ChatsFilters, limit: number) {
  return infiniteQueryOptions({
    queryKey: chatKeys.list(filters, limit),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/chats", {
          params: {
            query: {
              q: filters.q,
              dashboard_id: filters.dashboardId,
              scope: filters.scope,
              limit,
              cursor: pageParam,
            },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 10_000,
    refetchInterval: (query) =>
      query.state.data?.pages.some((page) =>
        page.chats.some((chat) => isLive(chat.last_turn))
      )
        ? activePollMs
        : false,
  })
}

export function useChats(filters: ChatsFilters, limit = 20, enabled = true) {
  const query = useInfiniteQuery({ ...chatsQuery(filters, limit), enabled })
  const chats = useMemo(() => {
    const seen = new Set<string>()
    const unique: Chat[] = []
    for (const page of query.data?.pages ?? []) {
      for (const chat of page.chats) {
        if (seen.has(chat.id)) continue
        seen.add(chat.id)
        unique.push(chat)
      }
    }
    return unique
  }, [query.data])
  return { chats, query }
}

export function useArchiveChat(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api.DELETE("/chats/{id}", { params: { path: { id } } }).then(unwrap),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: observeKeys.mineAll() }),
        queryClient.invalidateQueries({ queryKey: observeKeys.request(id) }),
      ]),
  })
}

export function useDashboardTitles(): ReadonlyMap<string, string> | undefined {
  const titles = useQuery(dashboardTitlesQuery())
  return useMemo(
    () =>
      titles.data === undefined
        ? undefined
        : new Map(titles.data.map((entry) => [entry.id, entry.title])),
    [titles.data]
  )
}
