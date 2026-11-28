import { useMemo } from "react"
import {
  infiniteQueryOptions,
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"

import { uniqueById } from "@/lib/unique-by-id"
import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { chatKeys, observeKeys, type ChatsFilters } from "@/api/keys"
import type { components } from "@/api/schema.gen"

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
              ...(filters.q === undefined ? {} : { q: filters.q }),
              ...(filters.dashboardId === undefined
                ? {}
                : { dashboard_id: filters.dashboardId }),
              ...(filters.scope === undefined ? {} : { scope: filters.scope }),
              limit,
              ...(pageParam === undefined ? {} : { cursor: pageParam }),
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
  const chats = useMemo(
    () => uniqueById(query.data?.pages, (page) => page.chats),
    [query.data]
  )
  return { chats, query }
}

function chatQuery(id: string) {
  return queryOptions({
    queryKey: chatKeys.detail(id),
    queryFn: ({ signal }) =>
      api.GET("/chats/{id}", { params: { path: { id } }, signal }).then(unwrap),
    staleTime: 2_000,
    refetchInterval: (query) =>
      query.state.data !== undefined && isLive(query.state.data.chat.last_turn)
        ? activePollMs
        : false,
  })
}

export async function refreshChat(
  queryClient: QueryClient,
  id: string
): Promise<void> {
  await queryClient.fetchQuery({ ...chatQuery(id), staleTime: 0 })
}

export function useChat(id: string | undefined) {
  return useQuery({ ...chatQuery(id ?? ""), enabled: id !== undefined })
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
