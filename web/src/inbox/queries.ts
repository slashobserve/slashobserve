import { useEffect, useState } from "react"
import {
  infiniteQueryOptions,
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { inboxKeys, type InboxFilter, type InboxView } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { throttle } from "@/lib/throttle"
import {
  useActivitySubscription,
  useHasActivityStream,
} from "@/live/activity-stream"
import { useDocumentVisible } from "@/live/use-activity-stream"

import { useInboxPanelStore } from "./inbox-panel-store"

type InboxList = components["schemas"]["InboxList"]
type InboxPages = InfiniteData<InboxList, string | undefined>

const pageSize = 50
const countStaleTime = 30_000
const liveWindowMs = 1000
const pollMs = 15_000

export function inboxPollInterval(
  streamed: boolean,
  visible: boolean
): number | false {
  return !streamed && visible ? pollMs : false
}

function useInboxPollInterval(): number | false {
  return inboxPollInterval(useHasActivityStream(), useDocumentVisible())
}

function fetchInboxPage(
  view: InboxView,
  filter: InboxFilter,
  cursor: string | undefined,
  limit: number,
  signal?: AbortSignal
): Promise<InboxList> {
  return api
    .GET("/inbox", {
      params: { query: { view, filter, cursor, limit } },
      signal,
    })
    .then(unwrap)
}

function inboxInfiniteQuery(view: InboxView, filter: InboxFilter = "open") {
  return infiniteQueryOptions({
    queryKey: inboxKeys.list(view, filter),
    queryFn: ({ pageParam, signal }) =>
      fetchInboxPage(view, filter, pageParam, pageSize, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  })
}

export function keepInboxHead(
  queryClient: QueryClient,
  view: InboxView,
  filter: InboxFilter
): void {
  queryClient.setQueryData<InboxPages>(
    inboxKeys.list(view, filter),
    (current) =>
      current === undefined || current.pages.length <= 1
        ? current
        : {
            pages: current.pages.slice(0, 1),
            pageParams: current.pageParams.slice(0, 1),
          }
  )
}

export function usePanelInboxList(view: InboxView, filter: InboxFilter) {
  return useInfiniteQuery({
    ...inboxInfiniteQuery(view, filter),
    refetchInterval: useInboxPollInterval(),
  })
}

export function useInboxForYou() {
  return useInfiniteQuery({
    ...inboxInfiniteQuery("for_you"),
    staleTime: countStaleTime,
  })
}

const inboxBadgeQuery = queryOptions({
  queryKey: inboxKeys.badge(),
  queryFn: ({ signal }) =>
    fetchInboxPage("for_you", "open", undefined, 1, signal),
})

export function useInboxCount(): number | null {
  const badge = useQuery({
    ...inboxBadgeQuery,
    staleTime: countStaleTime,
    refetchInterval: useInboxPollInterval(),
  })
  return badge.data?.unread_count ?? null
}

export function useMarkInboxRead() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (seq: number) =>
      api.POST("/inbox/read", { body: { seq } }).then(unwrap),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: inboxKeys.all }),
  })
}

export function useSetInboxItemUnread() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { seq: number; unread: boolean }) =>
      api
        .PUT("/inbox/items/{seq}", {
          params: { path: { seq: input.seq } },
          body: { unread: input.unread },
        })
        .then(unwrap),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: inboxKeys.all }),
  })
}

export function useSetInboxItemDone() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { seq: number; done: boolean }) =>
      api
        .PUT("/inbox/items/{seq}", {
          params: { path: { seq: input.seq } },
          body: { done: input.done },
        })
        .then(unwrap),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: inboxKeys.all }),
  })
}

export function useInboxLive() {
  const queryClient = useQueryClient()
  const [refresh] = useState(() =>
    throttle(() => {
      const { open, view, filter } = useInboxPanelStore.getState()
      void queryClient.invalidateQueries({
        queryKey: inboxKeys.badge(),
        exact: true,
      })
      if (open) {
        void queryClient.invalidateQueries({
          queryKey: inboxKeys.list(view, filter),
          exact: true,
        })
      }
    }, liveWindowMs)
  )
  useEffect(() => () => refresh.cancel(), [refresh])
  useActivitySubscription({ onEvent: refresh.call, onGap: refresh.call })
}
