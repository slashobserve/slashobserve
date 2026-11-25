import { useEffect, useState } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
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

type InboxPages = InfiniteData<
  components["schemas"]["InboxList"],
  string | undefined
>

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

function isForYou(view: InboxView, filter: InboxFilter): boolean {
  return view === "for_you" && filter === "open"
}

function inboxInfiniteQuery(view: InboxView, filter: InboxFilter = "open") {
  return infiniteQueryOptions({
    queryKey: inboxKeys.list(view, filter),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/inbox", {
          params: {
            query: { view, filter, cursor: pageParam, limit: pageSize },
          },
          signal,
        })
        .then(unwrap),
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
  const interval = useInboxPollInterval()
  return useInfiniteQuery({
    ...inboxInfiniteQuery(view, filter),
    refetchInterval: isForYou(view, filter) ? false : interval,
  })
}

export function useInboxForYou() {
  return useInfiniteQuery({
    ...inboxInfiniteQuery("for_you"),
    staleTime: countStaleTime,
    refetchInterval: useInboxPollInterval(),
  })
}

export function useInboxCount(): number | null {
  const first = useInboxForYou().data?.pages[0]
  return first === undefined ? null : first.unread_count
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
        queryKey: inboxKeys.list("for_you", "open"),
        exact: true,
      })
      if (open && !isForYou(view, filter)) {
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
