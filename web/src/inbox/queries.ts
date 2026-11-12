import { useEffect, useRef } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { inboxKeys, type InboxView } from "@/api/keys"
import {
  useActivityStream,
  useDocumentVisible,
} from "@/live/use-activity-stream"

const pageSize = 50
const countStaleTime = 30_000
const liveDebounceMs = 1000

export function inboxInfiniteQuery(view: InboxView) {
  return infiniteQueryOptions({
    queryKey: inboxKeys.list(view),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/inbox", {
          params: { query: { view, cursor: pageParam, limit: pageSize } },
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

export function useInboxLive() {
  const queryClient = useQueryClient()
  const visible = useDocumentVisible()
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  function refresh() {
    clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      void queryClient.invalidateQueries({ queryKey: inboxKeys.all })
    }, liveDebounceMs)
  }

  useEffect(() => () => clearTimeout(timer.current), [])

  useActivityStream({
    enabled: visible,
    onEvent: refresh,
    onGap: refresh,
  })
}
