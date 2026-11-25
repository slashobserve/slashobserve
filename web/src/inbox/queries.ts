import { useEffect, useRef } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { inboxKeys, type InboxFilter, type InboxView } from "@/api/keys"
import {
  useActivitySubscription,
  useHasActivityStream,
} from "@/live/activity-stream"
import { useDocumentVisible } from "@/live/use-activity-stream"

const pageSize = 50
const countStaleTime = 30_000
const liveDebounceMs = 1000
const pollMs = 15_000

export function inboxPollInterval(
  streamed: boolean,
  visible: boolean
): number | false {
  return !streamed && visible ? pollMs : false
}

export function useInboxPollInterval(): number | false {
  return inboxPollInterval(useHasActivityStream(), useDocumentVisible())
}

export function inboxInfiniteQuery(
  view: InboxView,
  filter: InboxFilter = "open"
) {
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
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  function refresh() {
    clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      void queryClient.invalidateQueries({ queryKey: inboxKeys.all })
    }, liveDebounceMs)
  }

  useEffect(() => () => clearTimeout(timer.current), [])

  useActivitySubscription({
    onEvent: refresh,
    onGap: refresh,
  })
}
